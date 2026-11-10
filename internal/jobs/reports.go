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

func (r Reports) summarise(ctx context.Context, now time.Time) error {
	if now.In(r.Zone).Hour() != summaryHour {
		return nil
	}
	open, err := core.OpenReports(ctx, r.DB)
	if err != nil || len(open) == 0 {
		return err
	}
	return r.Mail.Send(ctx, r.summary(open))
}

func (r Reports) summary(open []store.OpenReportsRow) mail.Message {
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

	var b strings.Builder
	for _, reports := range lists {
		l := reports[0]
		where := "still in browsing"
		if l.Hidden {
			where = "out of browsing"
		}
		fmt.Fprintf(&b, "“%s” by %s, %s\n%s/@%s/lists/%s\n", l.ListName, l.OwnerUsername, where, r.AppURL, l.OwnerUsername, l.ListSlug)
		for _, o := range reports {
			from := "a deleted profile"
			if o.ReporterUsername != nil {
				from = *o.ReporterUsername
			}
			fmt.Fprintf(&b, "  %s, from %s on %s", o.Reason, from, o.CreatedAt.In(r.Zone).Format("2 Jan 15:04"))
			if o.Note != nil {
				fmt.Fprintf(&b, ": %s", *o.Note)
			}
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "  tethr report keep %d\n  tethr report remove %d <reason the owner will read>\n\n", l.ListID, l.ListID)
	}
	subject := fmt.Sprintf("Reports waiting on %d lists", len(lists))
	if len(lists) == 1 {
		subject = "Reports waiting on 1 list"
	}
	return mail.Message{To: r.To, Subject: subject, Text: b.String()}
}
