package core

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/dbtest"
)

func reportedTitle(t *testing.T, db *pgxpool.Pool, kind string, w WikidataTitle) int64 {
	t.Helper()
	res, err := SeedTitle(context.Background(), db, kind, w, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	return res.Title
}

func titleReports(t *testing.T, db *pgxpool.Pool) []string {
	t.Helper()
	open, err := OpenReview(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range open {
		if e.Kind != ReportedTitle {
			continue
		}
		s := fmt.Sprintf("%s %s|%s>%s", e.TitleName, e.Subject, e.Tethr, e.Wikidata)
		if e.Why != "" {
			s += " note " + e.Why
		}
		if e.OtherName != "" || e.OtherQID != "" {
			s += fmt.Sprintf(" other %s %s", e.OtherName, e.OtherQID)
		}
		out = append(out, fmt.Sprintf("%s x%d", s, e.Reports))
	}
	return out
}

func TestReportTitle(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	acct, ann := household(t, db, "ann")
	_, bob := household(t, db, "bob")
	kid, err := AddProfile(ctx, db, acct, NewProfile{Name: "Ollie", Kind: "kids"})
	if err != nil {
		t.Fatal(err)
	}
	akira := reportedTitle(t, db, "movie", WikidataTitle{QID: "Q1", Name: "Akira", Year: 1988, TMDB: 149, IDs: map[string][]string{"imdb": {"tt0094625"}}, Sitelinks: 80})
	id := strconv.FormatInt(akira, 10)

	isNew, err := ReportTitle(ctx, db, ann, id, TitleReport{Reason: "name", Suggestion: " AKIRA\n", Note: "the poster\nsays\x00 so "})
	if err != nil || !isNew {
		t.Fatalf("report = %v, %v", isNew, err)
	}
	if stored, err := ReportTitle(ctx, db, ann, id, TitleReport{Reason: "name"}); err != nil || stored {
		t.Errorf("reporting again with nothing new = %v, %v, want nothing stored", stored, err)
	}
	if stored, err := ReportTitle(ctx, db, ann, id, TitleReport{Reason: "name", Suggestion: "Akira!"}); err != nil || !stored {
		t.Errorf("reporting again with a new suggestion = %v, %v, want it stored", stored, err)
	}
	if _, err := ReportTitle(ctx, db, kid.ID, id, TitleReport{Reason: "name"}); !errors.Is(err, ErrReportProfile) {
		t.Errorf("kids profile = %v, want ErrReportProfile", err)
	}
	if _, err := ReportTitle(ctx, db, bob, id, TitleReport{Reason: "name"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ReportTitle(ctx, db, bob, id, TitleReport{Reason: "year", Suggestion: "1989"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ReportTitle(ctx, db, bob, id, TitleReport{Reason: "link", Provider: "imdb", Suggestion: "https://www.imdb.com/title/tt0094626/"}); err != nil {
		t.Fatal(err)
	}
	want := "[Akira name|Akira>Akira! note the poster says so x2 Akira name|Akira> x2 Akira link imdb|tt0094625>tt0094626 x1 Akira year|1988>1989 x1]"
	if got := fmt.Sprint(titleReports(t, db)); got != want {
		t.Errorf("reports =\n%s\nwant\n%s", got, want)
	}
}

func TestTitleReportSuggestions(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	_, ann := household(t, db, "ann")
	akira := reportedTitle(t, db, "movie", WikidataTitle{QID: "Q1", Name: "Akira", Year: 1988, TMDB: 149, IDs: map[string][]string{"imdb": {"tt0094625"}}})
	twin := reportedTitle(t, db, "movie", WikidataTitle{QID: "Q2", Name: "Akira", Year: 1988, TMDB: 150, IDs: map[string][]string{"imdb": {"tt0094627"}}})
	show := reportedTitle(t, db, "show", WikidataTitle{QID: "Q3", Name: "Akira", Year: 2020})
	id := strconv.FormatInt(akira, 10)

	for _, c := range []struct {
		r    TitleReport
		want error
	}{
		{TitleReport{Reason: "plot"}, ErrTitleReportReason},
		{TitleReport{Reason: "year", Suggestion: "next year"}, ErrReportSuggestion},
		{TitleReport{Reason: "year", Suggestion: "1700"}, ErrReportSuggestion},
		{TitleReport{Reason: "year", Suggestion: "1988"}, ErrReportSame},
		{TitleReport{Reason: "name", Suggestion: "Akira"}, ErrReportSame},
		{TitleReport{Reason: "kind", Suggestion: "movie"}, ErrReportSame},
		{TitleReport{Reason: "kind", Suggestion: "episode"}, ErrReportSuggestion},
		{TitleReport{Reason: "link", Suggestion: "tt0094626"}, ErrReportProvider},
		{TitleReport{Reason: "link", Provider: "tvmaze"}, ErrReportProvider},
		{TitleReport{Reason: "link", Provider: "imdb", Suggestion: "https://www.themoviedb.org/movie/149"}, ErrReportSuggestion},
		{TitleReport{Reason: "duplicate"}, ErrReportDuplicate},
		{TitleReport{Reason: "duplicate", Suggestion: "https://www.imdb.com/title/tt9999999/"}, ErrReportDuplicate},
		{TitleReport{Reason: "duplicate", Suggestion: "https://tethr.tv/films/t-" + id}, ErrReportSelf},
		{TitleReport{Reason: "duplicate", Suggestion: "Q1"}, ErrReportSelf},
		{TitleReport{Reason: "duplicate", Suggestion: "https://www.wikidata.org/wiki/Q1?uselang=en-gb#P345"}, ErrReportSelf},
		{TitleReport{Reason: "name", Note: strings.Repeat("a", 501)}, ErrReportNote},
	} {
		if _, err := ReportTitle(ctx, db, ann, id, c.r); !errors.Is(err, c.want) {
			t.Errorf("%+v = %v, want %v", c.r, err, c.want)
		}
	}

	for _, r := range []TitleReport{
		{Reason: "kind", Suggestion: "show"},
		{Reason: "link", Provider: "letterboxd", Suggestion: "letterboxd.com/film/akira/"},
		{Reason: "link", Provider: "tmdb"},
		{Reason: "duplicate", Suggestion: "https://www.imdb.com/title/tt0094627/"},
	} {
		if _, err := ReportTitle(ctx, db, ann, id, r); err != nil {
			t.Errorf("%+v = %v", r, err)
		}
	}
	showID := strconv.FormatInt(show, 10)
	if _, err := ReportTitle(ctx, db, ann, showID, TitleReport{Reason: "duplicate", Suggestion: "https://www.wikidata.org/wiki/Q42"}); err != nil {
		t.Fatal(err)
	}
	twinID := strconv.FormatInt(twin, 10)
	if _, err := ReportTitle(ctx, db, ann, twinID, TitleReport{Reason: "duplicate", Suggestion: "https://www.themoviedb.org/movie/149-akira"}); err != nil {
		t.Fatal(err)
	}
	want := "[Akira duplicate|>Q2 other Akira Q2 x1 Akira kind|movie>show x1 Akira link letterboxd|>akira x1 Akira link tmdb|149> x1 Akira duplicate|>Q1 other Akira Q1 x1 Akira duplicate|>Q42 x1]"
	if got := fmt.Sprint(titleReports(t, db)); got != want {
		t.Errorf("reports =\n%s\nwant\n%s", got, want)
	}
}

func TestTitleReportCap(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	acct, ann := household(t, db, "ann")
	sam, err := AddProfile(ctx, db, acct, NewProfile{Name: "Sam", Kind: "person"})
	if err != nil {
		t.Fatal(err)
	}
	_, bob := household(t, db, "bob")
	var ids []string
	for i := range 11 {
		id := reportedTitle(t, db, "movie", WikidataTitle{QID: fmt.Sprintf("Q%d", 10+i), Name: fmt.Sprintf("Film %d", i), Year: 2000 + i})
		ids = append(ids, strconv.FormatInt(id, 10))
	}
	for i := range 10 {
		for _, p := range []int64{ann, sam.ID} {
			reason := "name"
			if p == sam.ID {
				reason = "year"
			}
			if _, err := ReportTitle(ctx, db, p, ids[i], TitleReport{Reason: reason}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := ReportTitle(ctx, db, sam.ID, ids[10], TitleReport{Reason: "name"}); !errors.Is(err, ErrTooManyTitleReports) {
		t.Errorf("21st report from one household = %v, want ErrTooManyTitleReports", err)
	}
	if _, err := ReportTitle(ctx, db, bob, ids[10], TitleReport{Reason: "name"}); err != nil {
		t.Errorf("another household = %v", err)
	}
}

func TestTitleReportsClear(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	_, ann := household(t, db, "ann")
	_, bob := household(t, db, "bob")
	akira := reportedTitle(t, db, "movie", WikidataTitle{QID: "Q1", Name: "Akira", Year: 1988, TMDB: 149})
	id := strconv.FormatInt(akira, 10)
	for _, r := range []TitleReport{
		{Reason: "name", Suggestion: "AKIRA"},
		{Reason: "year"},
		{Reason: "link", Provider: "imdb", Suggestion: "tt0094625"},
		{Reason: "kind", Suggestion: "show"},
	} {
		if _, err := ReportTitle(ctx, db, ann, id, r); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ReportTitle(ctx, db, bob, id, TitleReport{Reason: "year", Suggestion: "1987"}); err != nil {
		t.Fatal(err)
	}
	settle := func(imdb ...string) {
		t.Helper()
		if _, err := SeedTitle(ctx, db, "movie", WikidataTitle{QID: "Q1", Name: "AKIRA", Year: 1988, TMDB: 149, IDs: map[string][]string{"imdb": imdb}}, 0, false); err != nil {
			t.Fatal(err)
		}
		if err := SettleReview(ctx, db, nil, Settle{Titles: []int64{akira}, Kinds: ItemKinds}); err != nil {
			t.Fatal(err)
		}
	}

	settle("tt0094625", "tt0000001")
	if got := fmt.Sprint(titleReports(t, db)); got != "[AKIRA year|1988> x2 AKIRA year|1988>1987 x2 AKIRA kind|movie>show x1 AKIRA link imdb|>tt0094625 x1]" {
		t.Errorf("reports = %s, want the link kept while the wrong ID is still there", got)
	}
	reportedTitle(t, db, "show", WikidataTitle{QID: "Q1", Name: "AKIRA", Year: 1988})
	settle("tt0094625")
	if got := fmt.Sprint(titleReports(t, db)); got != "[AKIRA year|1988> x2 AKIRA year|1988>1987 x2]" {
		t.Errorf("reports = %s, want only the ones Wikidata doesn't match yet", got)
	}

	open, err := OpenReview(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := DismissReview(ctx, db, strconv.FormatInt(open[0].ID, 10), "1988 is right"); err != nil || n != 1 {
		t.Errorf("dismiss = %d, %v, want only the report seen when it was listed, not the newer one", n, err)
	}
	if n, err := DismissReview(ctx, db, strconv.FormatInt(open[1].ID, 10), "1988 is right"); err != nil || n != 1 {
		t.Errorf("dismiss = %d, %v", n, err)
	}
	if got := titleReports(t, db); len(got) != 0 {
		t.Errorf("reports = %v", got)
	}
}
