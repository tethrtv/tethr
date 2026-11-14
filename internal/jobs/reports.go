package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/mail"
	"github.com/tethrtv/tethr/internal/store"
	"github.com/tethrtv/tethr/internal/wikidata"
)

const summaryHour = 9

type Sender interface {
	Send(ctx context.Context, m mail.Message) error
}

type Reports struct {
	DB     *pgxpool.Pool
	Mail   Sender
	To     string
	AppURL string
	Zone   *time.Location
	Log    *slog.Logger
}

func (r Reports) Run(ctx context.Context) {
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	for {
		if err := r.summarise(ctx, time.Now()); err != nil && ctx.Err() == nil {
			r.Log.Error("emailing the reports waiting", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

const summaryJob = "morning summary"

func (r Reports) summarise(ctx context.Context, now time.Time) error {
	local := now.In(r.Zone)
	morning := time.Date(local.Year(), local.Month(), local.Day(), summaryHour, 0, 0, 0, r.Zone)
	if local.Before(morning) {
		return nil
	}
	last, err := core.JobLastRun(ctx, r.DB, summaryJob)
	if err != nil || !last.Before(morning) {
		return err
	}
	if err := core.CheckCatalogue(ctx, r.DB); err != nil {
		r.Log.Error("checking the catalogue for the review list", "err", err)
	}
	open, err := core.OpenReports(ctx, r.DB)
	if err != nil {
		return err
	}
	entries, err := core.OpenReview(ctx, r.DB)
	if err != nil {
		return err
	}
	if m, ok := r.summary(open, entries, last); ok {
		if err := r.Mail.Send(ctx, m); err != nil {
			return err
		}
	}
	return core.MarkJobRanAt(ctx, r.DB, summaryJob, now)
}

func (r Reports) summary(open []store.OpenReportsRow, entries []core.ReviewEntry, since time.Time) (mail.Message, bool) {
	var subject []string
	var b strings.Builder
	if lists := r.listReports(&b, open); lists > 0 {
		if lists == 1 {
			subject = append(subject, "Reports waiting on 1 list")
		} else {
			subject = append(subject, fmt.Sprintf("Reports waiting on %d lists", lists))
		}
	}
	if fresh := reviewSummary(&b, entries, since); fresh > 0 {
		subject = append(subject, fmt.Sprintf("%d new for catalogue review", fresh))
	}
	if len(subject) == 0 {
		return mail.Message{}, false
	}
	return mail.Message{To: r.To, Subject: strings.Join(subject, ", "), Text: b.String()}, true
}

func (r Reports) listReports(b *strings.Builder, open []store.OpenReportsRow) int {
	var lists [][]store.OpenReportsRow
	for _, o := range open {
		if n := len(lists); n > 0 && lists[n-1][0].ListID == o.ListID {
			lists[n-1] = append(lists[n-1], o)
		} else {
			lists = append(lists, []store.OpenReportsRow{o})
		}
	}
	slices.SortStableFunc(lists, func(a, b []store.OpenReportsRow) int {
		switch {
		case a[0].Hidden == b[0].Hidden:
			return 0
		case a[0].Hidden:
			return -1
		}
		return 1
	})

	for _, reports := range lists {
		l := reports[0]
		where := "still in browsing"
		if l.Hidden {
			where = "out of browsing"
		}
		fmt.Fprintf(b, "“%s” by %s, %s\n%s/@%s/lists/%s\n", l.ListName, l.OwnerUsername, where, r.AppURL, l.OwnerUsername, l.ListSlug)
		for _, o := range reports {
			from := "a deleted profile"
			if o.ReporterUsername != nil {
				from = *o.ReporterUsername
			}
			fmt.Fprintf(b, "  %s, from %s on %s", o.Reason, from, o.CreatedAt.In(r.Zone).Format("2 Jan 15:04"))
			if o.Note != nil {
				fmt.Fprintf(b, ": %s", *o.Note)
			}
			b.WriteString("\n")
		}
		fmt.Fprintf(b, "  tethr report keep %d\n  tethr report remove %d <reason the owner will read>\n\n", l.ListID, l.ListID)
	}
	return len(lists)
}

const reviewListed = 50

func reviewSummary(b *strings.Builder, entries []core.ReviewEntry, since time.Time) int {
	type group struct {
		kind, batch string
		entries     []core.ReviewEntry
	}
	var groups []*group
	byBatch := map[[2]string]*group{}
	older := map[string]int{}
	fresh := 0
	for _, e := range entries {
		if !e.CreatedAt.After(since) {
			older[e.Kind]++
			continue
		}
		fresh++
		if e.Batch == "" {
			groups = append(groups, &group{kind: e.Kind, entries: []core.ReviewEntry{e}})
			continue
		}
		k := [2]string{e.Kind, e.Batch}
		if g, ok := byBatch[k]; ok {
			g.entries = append(g.entries, e)
			continue
		}
		byBatch[k] = &group{kind: e.Kind, batch: e.Batch, entries: []core.ReviewEntry{e}}
		groups = append(groups, byBatch[k])
	}
	if fresh == 0 {
		return 0
	}

	b.WriteString("Catalogue review\n\n")
	listed := map[string]int{}
	skipped := map[string]int{}
	for _, g := range groups {
		if g.batch != "" {
			fmt.Fprintf(b, "%s\n  %s\n  %s\n  tethr review batch %s\n\n", core.BatchSummary(g.kind, g.entries), wikidata.BatchURL(g.batch), batchTitles(g.entries), g.batch)
			continue
		}
		e := g.entries[0]
		if listed[e.Kind]++; listed[e.Kind] > reviewListed {
			skipped[e.Kind]++
			continue
		}
		fmt.Fprintf(b, "%s\n  %s\n", e.Describe(), core.WikidataURL(e.QID))
		if e.Kind == core.KeptName {
			fmt.Fprintf(b, "  tethr review accept %d\n", e.ID)
		}
		fmt.Fprintf(b, "  tethr review dismiss %d <reason>\n\n", e.ID)
	}
	var more []string
	for _, k := range core.ReviewKinds {
		if skipped[k] > 0 {
			more = append(more, core.ReviewCount(k, skipped[k]))
		}
	}
	if len(more) > 0 {
		fmt.Fprintf(b, "Also new: %s.\n", strings.Join(more, ", "))
	}
	var rest []string
	for _, k := range core.ReviewKinds {
		if older[k] > 0 {
			rest = append(rest, core.ReviewCount(k, older[k]))
		}
	}
	if len(rest) > 0 {
		fmt.Fprintf(b, "Still open from before: %s.", strings.Join(rest, ", "))
	}
	if len(more) > 0 || len(rest) > 0 {
		b.WriteString(" Run tethr review on the server for the list.\n")
	}
	return fresh
}

func batchTitles(entries []core.ReviewEntry) string {
	var names []string
	for _, e := range entries[:min(len(entries), 5)] {
		names = append(names, e.Name())
	}
	out := strings.Join(names, ", ")
	if len(entries) > 5 {
		out += fmt.Sprintf(" and %d more", len(entries)-5)
	}
	return out
}
