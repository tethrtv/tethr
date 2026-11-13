package jobs

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/wikidata"
)

type Feed struct {
	DB       *pgxpool.Pool
	Wikidata *wikidata.Client
	Log      *slog.Logger
}

func (f Feed) Run(ctx context.Context) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		switch err := f.Follow(ctx); {
		case err == nil || ctx.Err() != nil:
		case errors.Is(err, wikidata.ErrBusy):
			f.Log.Warn("wikidata is busy, following its edits later", "err", err)
		default:
			f.Log.Error("following wikidata's edits", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (f Feed) Follow(ctx context.Context) error {
	cursor, err := core.FeedCursor(ctx, f.DB)
	if err != nil {
		return err
	}
	// Changes can reach the feed a little after their timestamp, so stay a few minutes behind.
	to := time.Now().Add(-5 * time.Minute).UTC().Truncate(time.Second)
	if !cursor.Before(to) {
		return nil
	}
	cont, first := "", true
	for {
		changes, next, err := f.Wikidata.RecentChanges(ctx, cursor, to, cont)
		if err != nil {
			return err
		}
		if first && len(changes) > 0 && changes[0].At.Sub(cursor) > time.Hour {
			queued, err := core.QueueAllWikidata(ctx, f.DB)
			if err != nil {
				return err
			}
			f.Log.Error("wikidata's feed no longer goes back to the cursor, refetching every item", "cursor", cursor, "oldest", changes[0].At, "queued", queued)
		}
		first = false
		edits := map[string]core.Edits{}
		moved := cursor
		for _, c := range changes {
			if c.At.After(moved) {
				moved = c.At
			}
			if !c.Relevant {
				continue
			}
			e, ok := edits[c.QID]
			if !ok || c.At.Before(e.First) {
				e.First = c.At
			}
			if c.At.After(e.Last) {
				e.Last = c.At
			}
			e.LoggedOut = e.LoggedOut || c.LoggedOut
			edits[c.QID] = e
		}
		if next == "" {
			moved = to
		}
		if _, err := core.QueueWikidataEdits(ctx, f.DB, edits, moved); err != nil {
			return err
		}
		if next == "" {
			return nil
		}
		cont = next
	}
}
