package core

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

var (
	ErrReportReason   = errors.New("a report's reason is illegal, harassment, personal, spam or other")
	ErrReportNote     = errors.New("a report's note is up to 500 characters")
	ErrTooManyReports = errors.New("a household can send up to 5 reports a day")
	ErrReportOfficial = errors.New("lists made by tethr can't be reported")
)

const reportsPerDay = 5

type Report struct {
	List     List
	Reason   string
	Note     *string
	Reporter string
	Hid      bool
}

type RemovedList struct {
	Email string
	Name  string
}

func ReportList(ctx context.Context, db *pgxpool.Pool, reporter, listID int64, reason, note string) (Report, bool, error) {
	q := store.New(db)
	l, err := getList(ctx, q, reporter, listID)
	if err != nil {
		return Report{}, false, err
	}
	if l.Access != "public" {
		return Report{}, false, ErrNotFound
	}
	if l.Owner.Username == OfficialUsername {
		return Report{}, false, ErrReportOfficial
	}
	if !slices.Contains([]string{"illegal", "harassment", "personal", "spam", "other"}, reason) {
		return Report{}, false, ErrReportReason
	}
	n := optional(strings.TrimSpace(note))
	if n != nil && utf8.RuneCountInString(*n) > 500 {
		return Report{}, false, ErrReportNote
	}
	p, err := q.ProfileByID(ctx, reporter)
	if err != nil {
		return Report{}, false, err
	}
	sent, err := q.CountReportsSince(ctx, store.CountReportsSinceParams{HouseholdID: &p.HouseholdID, CreatedAt: time.Now().Add(-24 * time.Hour)})
	if err != nil {
		return Report{}, false, err
	}
	if sent >= reportsPerDay {
		return Report{}, false, ErrTooManyReports
	}
	r := Report{List: l, Reason: reason, Note: n, Reporter: p.Username}
	isNew := false
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		if _, err := q.LockList(ctx, listID); err != nil {
			return notFound(err)
		}
		before, err := q.ListHidden(ctx, listID)
		if err != nil {
			return err
		}
		_, err = q.AddListReport(ctx, store.AddListReportParams{ListID: listID, ReporterID: &reporter, HouseholdID: &p.HouseholdID, Reason: reason, Note: n})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		isNew = true
		after, err := q.ListHidden(ctx, listID)
		r.Hid = !before && after
		return err
	})
	if err != nil {
		return Report{}, false, err
	}
	return r, isNew, nil
}

func OpenReports(ctx context.Context, db *pgxpool.Pool) ([]store.OpenReportsRow, error) {
	return store.New(db).OpenReports(ctx)
}

func KeepList(ctx context.Context, db *pgxpool.Pool, listID int64) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		n, err := q.ResolveReports(ctx, store.ResolveReportsParams{ListID: listID, Outcome: optional("kept")})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		return q.MarkListKept(ctx, listID)
	})
}

func RemoveList(ctx context.Context, db *pgxpool.Pool, listID int64) (RemovedList, error) {
	var out RemovedList
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		c, err := q.ListOwnerContact(ctx, listID)
		if err != nil {
			return notFound(err)
		}
		if err := q.MakeListPrivate(ctx, listID); err != nil {
			return err
		}
		if _, err := q.ResolveReports(ctx, store.ResolveReportsParams{ListID: listID, Outcome: optional("removed")}); err != nil {
			return err
		}
		out = RemovedList{Email: c.Email, Name: c.Name}
		return markLists(ctx, q, c.ProfileID)
	})
	return out, err
}

func PruneReports(ctx context.Context, db *pgxpool.Pool) error {
	return store.New(db).PruneReports(ctx)
}
