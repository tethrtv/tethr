package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/wikidata"
)

const (
	sweepJob     = "wikidata sweep"
	sweepAttempt = "wikidata sweep attempt"
)

type Sweep struct {
	DB       *pgxpool.Pool
	Wikidata *wikidata.Client
	Log      *slog.Logger
}

func (s Sweep) Run(ctx context.Context) {
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	for {
		switch err := s.due(ctx); {
		case err == nil || ctx.Err() != nil:
		case errors.Is(err, wikidata.ErrBusy):
			s.Log.Warn("wikidata is busy, sweeping later", "err", err)
		default:
			s.Log.Error("sweeping wikidata for new titles", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (s Sweep) due(ctx context.Context) error {
	now := time.Now()
	ran, err := core.JobRanSince(ctx, s.DB, sweepJob, now.Add(-23*time.Hour))
	if err != nil || ran {
		return err
	}
	tried, err := core.JobRanSince(ctx, s.DB, sweepAttempt, now.Add(-6*time.Hour))
	if err != nil || tried {
		return err
	}
	if err := core.MarkJobRun(ctx, s.DB, sweepAttempt); err != nil {
		return err
	}
	return s.Once(ctx)
}

func (s Sweep) Once(ctx context.Context) error {
	listed, fellBack, err := s.Wikidata.ListTitles(ctx, core.ListedClasses())
	if err != nil {
		return err
	}
	if fellBack != nil {
		s.Log.Warn("wikidata's query service failed, the list came from qlever", "err", fellBack)
	}
	known, err := core.KnownWikidataItems(ctx, s.DB)
	if err != nil {
		return err
	}
	var fresh []string
	for _, q := range listed {
		if !known[q] {
			fresh = append(fresh, q)
		}
	}
	waiting := max(len(fresh)-5000, 0)
	fresh = fresh[:min(len(fresh), 5000)]

	sd := &seeding{db: s.DB, log: s.Log, counts: map[string]int{}}
	report := func(msg string) {
		s.Log.Info(msg, "listed", len(listed), "new", sd.counts["items"], "created", sd.counts["created"], "updated", sd.counts["updated"],
			"skipped", sd.counts["skipped"], "left out", sd.counts["left out"], "kind unknown", sd.counts["kind unknown"],
			"deleted", sd.counts["deleted"], "unreadable", sd.counts["unreadable"], "failed", sd.counts["failed"], "waiting", waiting)
	}
	if err := s.sweep(ctx, sd, fresh); err != nil {
		report("sweep stopped")
		return err
	}
	report("swept wikidata")
	return core.MarkJobRun(ctx, s.DB, sweepJob)
}

func (s Sweep) sweep(ctx context.Context, sd *seeding, fresh []string) error {
	for len(fresh) > 0 {
		n := min(len(fresh), 50)
		items, deleted, unreadable, err := s.Wikidata.FetchItems(ctx, fresh[:n])
		if err != nil {
			return err
		}
		fresh = fresh[n:]
		for _, q := range deleted {
			sd.counts["deleted"]++
			if err := core.LeaveOutWikidata(ctx, s.DB, q, "deleted"); err != nil {
				return err
			}
		}
		for _, q := range unreadable {
			sd.counts["unreadable"]++
			s.Log.Warn("wikidata item unreadable, left out for now", "item", q)
			if err := core.LeaveOutWikidata(ctx, s.DB, q, "unreadable"); err != nil {
				return err
			}
		}
		var classes []string
		for _, it := range items {
			classes = append(classes, it.InstanceOf()...)
		}
		if err := fillClasses(ctx, s.DB, s.Wikidata, s.Log, classes); err != nil {
			return err
		}
		for _, it := range items {
			sd.counts["items"]++
			err := sd.item(ctx, it, nil, false)
			if err == nil {
				continue
			}
			if ctx.Err() != nil {
				return err
			}
			sd.counts["failed"]++
			s.Log.Error("seeding a new wikidata item", "item", it.QID(), "err", err)
			if err := core.LeaveOutWikidata(ctx, s.DB, it.QID(), fmt.Sprintf("failed: %v", err)); err != nil {
				return err
			}
		}
	}
	return nil
}
