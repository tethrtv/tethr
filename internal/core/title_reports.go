package core

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
	"github.com/tethrtv/tethr/internal/wikidata"
)

var (
	ErrTitleReportReason   = errors.New("a title report's reason is name, year, kind, link or duplicate")
	ErrReportProfile       = errors.New("kids and guest profiles can't send reports")
	ErrReportSuggestion    = errors.New("the suggestion doesn't fit the reason")
	ErrReportSame          = errors.New("the suggestion is what tethr already has")
	ErrReportProvider      = errors.New("a link report needs a site that lists this kind of title")
	ErrReportDuplicate     = errors.New("a duplicate report needs a link to another title")
	ErrReportSelf          = errors.New("a duplicate report can't point at the title itself")
	ErrTooManyTitleReports = errors.New("a household can send up to 20 title reports a day")
)

const titleReportsPerDay = 20

type TitleReport struct {
	Reason     string
	Provider   string
	Suggestion string
	Note       string
}

var (
	wikidataLink = regexp.MustCompile(`^(?:(?:https?://)?(?:www\.|m\.)?wikidata\.org/(?:wiki|entity)/)?(Q[1-9][0-9]*)/?(?:[?#].*)?$`)
	tethrLink    = regexp.MustCompile(`^(?:https?://)?[^/\s]+/(films|shows)/([a-z0-9-]+)/?(?:[?#].*)?$`)
)

func cleanText(s string) string {
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }), " ")
}

func ReportTitle(ctx context.Context, db *pgxpool.Pool, reporter int64, id string, r TitleReport) (bool, error) {
	q := store.New(db)
	p, err := q.ProfileByID(ctx, reporter)
	if err != nil {
		return false, err
	}
	if !goesOutside(p.Kind) {
		return false, ErrReportProfile
	}
	t, err := titleByPath(ctx, q, id, "")
	if err != nil {
		return false, err
	}
	f, err := titleProblem(ctx, q, t, r)
	if err != nil {
		return false, err
	}
	if f.Why = cleanText(r.Note); utf8.RuneCountInString(f.Why) > 500 {
		return false, ErrReportNote
	}
	isNew := false
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		if _, err := q.LockHousehold(ctx, p.HouseholdID); err != nil {
			return err
		}
		sent, err := q.CountTitleReportsSince(ctx, store.CountTitleReportsSinceParams{HouseholdID: &p.HouseholdID, CreatedAt: time.Now().Add(-24 * time.Hour)})
		if err != nil {
			return err
		}
		if sent >= titleReportsPerDay {
			return ErrTooManyTitleReports
		}
		var other *int64
		if f.Other != 0 {
			other = &f.Other
		}
		n, err := q.AddTitleReport(ctx, store.AddTitleReportParams{
			Key: fmt.Sprintf("%s %d", f.key(), p.HouseholdID), Qid: f.QID, TitleID: &t.ID, Subject: f.Subject,
			TethrValue: f.Tethr, WikidataValue: f.Wikidata, Why: f.Why, OtherID: other, HouseholdID: &p.HouseholdID, ReporterID: &reporter,
		})
		isNew = n > 0
		return err
	})
	return isNew, err
}

func titleProblem(ctx context.Context, q *store.Queries, t store.Title, r TitleReport) (ReviewFlag, error) {
	s := cleanText(r.Suggestion)
	f := ReviewFlag{Kind: ReportedTitle, QID: deref(t.WikidataID), Title: t.ID, Subject: r.Reason}
	have := []string{}
	switch r.Reason {
	case "name":
		have = append(have, deref(t.Name))
		if utf8.RuneCountInString(s) > 500 {
			return f, ErrReportSuggestion
		}
	case "year":
		if t.Year != nil {
			have = append(have, strconv.Itoa(int(*t.Year)))
		}
		if s != "" {
			y, err := strconv.Atoi(s)
			if err != nil || y < 1870 || y > time.Now().Year()+10 {
				return f, ErrReportSuggestion
			}
			s = strconv.Itoa(y)
		}
	case "kind":
		have = append(have, t.Kind)
		if s != "" && s != "movie" && s != "show" {
			return f, ErrReportSuggestion
		}
	case "link":
		if !wikidata.Lists(t.Kind, r.Provider) {
			return f, ErrReportProvider
		}
		f.Subject = "link " + r.Provider
		values, err := titleLinks(ctx, q, t, r.Provider)
		if err != nil {
			return f, err
		}
		have = values
		if s != "" {
			v, ok := wikidata.OutsideID(t.Kind, r.Provider, s)
			if !ok {
				return f, ErrReportSuggestion
			}
			s = v
		}
	case "duplicate":
		other, qid, err := duplicateOf(ctx, q, s)
		if err != nil {
			return f, err
		}
		if other.ID == t.ID {
			return f, ErrReportSelf
		}
		f.Other, s = other.ID, qid
	default:
		return f, ErrTitleReportReason
	}
	if s != "" && slices.Contains(have, s) {
		return f, ErrReportSame
	}
	f.Tethr, f.Wikidata = strings.Join(have, ", "), s
	return f, nil
}

func titleLinks(ctx context.Context, q *store.Queries, t store.Title, provider string) ([]string, error) {
	if provider == "tmdb" {
		if t.TmdbID == nil {
			return nil, nil
		}
		return []string{strconv.Itoa(int(*t.TmdbID))}, nil
	}
	ids, err := q.TitleExternalIDs(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, id := range ids {
		if id.Provider == provider {
			out = append(out, id.Value)
		}
	}
	slices.Sort(out)
	return out, nil
}

func duplicateOf(ctx context.Context, q *store.Queries, s string) (store.Title, string, error) {
	if m := wikidataLink.FindStringSubmatch(s); m != nil {
		qid := m[1]
		for _, kind := range []string{"movie", "show"} {
			t, err := q.TitleByWikidata(ctx, store.TitleByWikidataParams{Kind: kind, WikidataID: &qid})
			if err == nil {
				return t, qid, nil
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return store.Title{}, "", err
			}
		}
		return store.Title{}, qid, nil
	}
	t, err := store.Title{}, ErrNotFound
	if m := tethrLink.FindStringSubmatch(s); m != nil {
		kind := "show"
		if m[1] == "films" {
			kind = "movie"
		}
		t, err = titleByPath(ctx, q, m[2], kind)
	} else {
		for _, kind := range []string{"movie", "show"} {
			if v, ok := wikidata.OutsideID(kind, "imdb", s); ok {
				t, err = findTitle(ctx, q, kind, IDs{IMDB: v})
			} else if v, ok := wikidata.OutsideID(kind, "tmdb", s); ok && strings.Contains(s, "themoviedb.org") {
				n, _ := strconv.ParseInt(v, 10, 64)
				t, err = findTitle(ctx, q, kind, IDs{TMDB: n})
			}
			if !errors.Is(err, ErrNotFound) {
				break
			}
		}
	}
	if errors.Is(err, ErrNotFound) {
		return store.Title{}, "", ErrReportDuplicate
	}
	if err != nil {
		return store.Title{}, "", err
	}
	return t, deref(t.WikidataID), nil
}
