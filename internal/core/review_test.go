package core

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/dbtest"
)

func openReview(t *testing.T, db *pgxpool.Pool) []ReviewEntry {
	t.Helper()
	open, err := OpenReview(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	return open
}

func reviewTitle(t *testing.T, db *pgxpool.Pool, kind string, w WikidataTitle) int64 {
	t.Helper()
	res, err := SeedTitle(context.Background(), db, kind, w, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	return res.Title
}

func TestReviewFlags(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	akira := reviewTitle(t, db, "movie", WikidataTitle{QID: "Q1", Name: "Akira", Year: 1988, TMDB: 149})
	heat := reviewTitle(t, db, "movie", WikidataTitle{QID: "Q2", Name: "Heat", Year: 1995, TMDB: 949})

	kept := ReviewFlag{Kind: KeptName, QID: "Q1", Title: akira, Tethr: "Akira", Wikidata: "アキラ", Why: "out of latin script", Batch: "QSv2/1"}
	if err := FlagForReview(ctx, db, []ReviewFlag{kept}); err != nil {
		t.Fatal(err)
	}
	kept.Batch = "QSv2/2"
	if err := FlagForReview(ctx, db, []ReviewFlag{kept, {Kind: DeletedItem, QID: "Q2", Title: heat}}); err != nil {
		t.Fatal(err)
	}
	open := openReview(t, db)
	if len(open) != 2 {
		t.Fatalf("open = %+v, want one entry each however often it's flagged", open)
	}
	byKind := map[string]ReviewEntry{}
	for _, e := range open {
		byKind[e.Kind] = e
	}
	if e := byKind[KeptName]; e.Batch != "QSv2/1" || e.Wikidata != "アキラ" || e.TitleName != "Akira" || e.TitleYear != 1988 {
		t.Errorf("kept name = %+v, want the first batch kept and the title's name and year", e)
	}

	if err := SettleReview(ctx, db, nil, Settle{Titles: []int64{akira, heat}, Kinds: ItemKinds}); err != nil {
		t.Fatal(err)
	}
	if open := openReview(t, db); len(open) != 0 {
		t.Errorf("open after Wikidata stopped showing either = %+v", open)
	}
	var fixed int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM review_entries WHERE outcome = 'fixed' AND cleared_at IS NOT NULL`).Scan(&fixed); err != nil || fixed != 2 {
		t.Errorf("%d fixed, %v, want both kept as cleared", fixed, err)
	}

	if err := FlagForReview(ctx, db, []ReviewFlag{{Kind: KindChange, QID: "Q2", Title: heat, Tethr: "movie", Wikidata: "show"}, {Kind: SharedID, QID: "Q2", Title: heat, Subject: "imdb tt0113277", Other: akira}}); err != nil {
		t.Fatal(err)
	}
	if err := SettleReview(ctx, db, nil, Settle{Titles: []int64{heat}, Kinds: []string{SharedID}}); err != nil {
		t.Fatal(err)
	}
	open = openReview(t, db)
	if len(open) != 1 || open[0].Kind != KindChange {
		t.Errorf("open = %+v, want settling only the kinds it was told about", open)
	}
}

func TestHeldChangeClearsWhenWikidataMovesOn(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	saw := reviewTitle(t, db, "movie", WikidataTitle{QID: "Q3", Name: "Saw", Year: 2007, TMDB: 176})
	if err := FlagForReview(ctx, db, []ReviewFlag{{Kind: HeldChange, QID: "Q3", Title: saw, Subject: "year", Tethr: "2004", Wikidata: "2007"}}); err != nil {
		t.Fatal(err)
	}
	if err := SettleReview(ctx, db, nil, Settle{Titles: []int64{saw}, Kinds: ItemKinds}); err != nil {
		t.Fatal(err)
	}
	if open := openReview(t, db); len(open) != 1 {
		t.Errorf("open = %+v, want a change that still stands kept", open)
	}
	if _, err := db.Exec(ctx, `UPDATE titles SET year = 2004 WHERE id = $1`, saw); err != nil {
		t.Fatal(err)
	}
	if err := SettleReview(ctx, db, nil, Settle{Titles: []int64{saw}, Kinds: ItemKinds}); err != nil {
		t.Fatal(err)
	}
	if open := openReview(t, db); len(open) != 0 {
		t.Errorf("open = %+v, want a change Wikidata undid cleared", open)
	}
}

func TestDismissAndAccept(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	akira := reviewTitle(t, db, "movie", WikidataTitle{QID: "Q1", Name: "Akira", Year: 1988, TMDB: 149})
	mezon := reviewTitle(t, db, "show", WikidataTitle{QID: "Q4", Name: "Maison Ikkoku", Year: 1986})
	heat := reviewTitle(t, db, "movie", WikidataTitle{QID: "Q2", Name: "Heat", Year: 1995, TMDB: 949})
	flags := []ReviewFlag{
		{Kind: KeptName, QID: "Q1", Title: akira, Tethr: "Akira", Wikidata: "アキラ", Why: "out of latin script", Batch: "QSv2/7"},
		{Kind: KeptName, QID: "Q4", Title: mezon, Tethr: "Maison Ikkoku", Wikidata: "めぞん一刻", Why: "out of latin script", Batch: "QSv2/7"},
		{Kind: FilmAndShow, QID: "Q2"},
	}
	if err := FlagForReview(ctx, db, flags); err != nil {
		t.Fatal(err)
	}
	ids := map[string]int64{}
	for _, e := range openReview(t, db) {
		ids[e.QID] = e.ID
	}

	if _, err := AcceptReview(ctx, db, strconv.FormatInt(ids["Q2"], 10)); !errors.Is(err, ErrReviewAccept) {
		t.Errorf("accepting a film-and-show item = %v, want only kept names accepted", err)
	}
	if _, err := DismissReview(ctx, db, strconv.FormatInt(ids["Q2"], 10), ""); !errors.Is(err, ErrReviewReason) {
		t.Errorf("dismissing with no reason = %v", err)
	}
	if n, err := AcceptReview(ctx, db, "QSv2/7"); err != nil || n != 2 {
		t.Fatalf("accepting a batch = %d, %v", n, err)
	}
	for id, want := range map[int64]string{akira: "アキラ", mezon: "めぞん一刻"} {
		var name string
		if err := db.QueryRow(ctx, `SELECT name FROM titles WHERE id = $1`, id).Scan(&name); err != nil || name != want {
			t.Errorf("title %d named %q, %v, want Wikidata's name taken", id, name, err)
		}
	}
	if n, err := DismissReview(ctx, db, strconv.FormatInt(ids["Q2"], 10), "it really is both"); err != nil || n != 1 {
		t.Fatalf("dismissing = %d, %v", n, err)
	}
	if n, err := DismissReview(ctx, db, strconv.FormatInt(ids["Q2"], 10), "again"); !errors.Is(err, ErrNotFound) || n != 0 {
		t.Errorf("dismissing a cleared entry = %d, %v", n, err)
	}
	if open := openReview(t, db); len(open) != 0 {
		t.Fatalf("open = %+v", open)
	}

	if err := FlagForReview(ctx, db, []ReviewFlag{{Kind: FilmAndShow, QID: "Q2"}, {Kind: KeptName, QID: "Q2", Title: heat, Tethr: "Heat", Wikidata: "ヒート", Why: "out of latin script"}}); err != nil {
		t.Fatal(err)
	}
	open := openReview(t, db)
	if len(open) != 1 || open[0].Kind != KeptName {
		t.Errorf("open = %+v, want a dismissed problem kept quiet while Wikidata says the same", open)
	}
	if err := FlagForReview(ctx, db, []ReviewFlag{{Kind: KeptName, QID: "Q2", Title: heat, Tethr: "Heat", Wikidata: "ヒート", Why: "out of latin script"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := DismissReview(ctx, db, strconv.FormatInt(open[0].ID, 10), "wrong"); err != nil {
		t.Fatal(err)
	}
	if err := FlagForReview(ctx, db, []ReviewFlag{{Kind: KeptName, QID: "Q2", Title: heat, Tethr: "Heat", Wikidata: "熱", Why: "out of latin script"}}); err != nil {
		t.Fatal(err)
	}
	if open := openReview(t, db); len(open) != 1 || open[0].Wikidata != "熱" {
		t.Errorf("open = %+v, want a new value on Wikidata to open a new entry", open)
	}
}

func TestCatalogueChecks(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	if err := SaveClasses(ctx, db, []WikidataClass{
		{QID: "Q11424", Label: "film"},
		{QID: "Q130232", Label: "drama film", Parents: []string{"Q25372"}},
		{QID: "Q62132", Label: "Sápmi"},
		{QID: "Q183", Label: "Germany", Country: "DE"},
		{QID: "Q99001", Label: "mumblecore", Parents: []string{"Q99002"}},
		{QID: "Q99002", Label: "style"},
	}); err != nil {
		t.Fatal(err)
	}
	for i := range reviewThreshold {
		reviewTitle(t, db, "movie", WikidataTitle{QID: fmt.Sprintf("Q%d", 500+i), Name: fmt.Sprintf("Film %d", i), Year: 2001, TMDB: int32(500 + i),
			InstanceOf: []string{"Q11424"}, Genres: []string{"Q130232", "Q99001"}, Countries: []string{"Q62132", "Q183"}})
	}
	reviewTitle(t, db, "movie", WikidataTitle{QID: "Q600", Name: "Twin", Year: 1965, TMDB: 600, InstanceOf: []string{"Q11424"}, Countries: []string{"Q99003"}})
	reviewTitle(t, db, "show", WikidataTitle{QID: "Q600", Name: "Twin", Year: 1965})

	if err := CheckCatalogue(ctx, db); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, e := range openReview(t, db) {
		got[e.Kind+" "+e.Subject+" "+e.QID] = true
	}
	want := map[string]bool{
		"film_and_show  Q600":           true,
		"unmapped_value country Q62132": true,
		"unmapped_value genre Q99001":   true,
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("open = %v, want %v (mapped values, rare values and the drama map left alone)", got, want)
	}

	if _, err := db.Exec(ctx, `UPDATE titles SET wikidata_countries = '{}' WHERE wikidata_countries @> '{Q62132}'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE titles SET wikidata_id = 'Q601' WHERE kind = 'show' AND wikidata_id = 'Q600'`); err != nil {
		t.Fatal(err)
	}
	if err := CheckCatalogue(ctx, db); err != nil {
		t.Fatal(err)
	}
	if open := openReview(t, db); len(open) != 1 || open[0].Subject != "genre" {
		t.Errorf("open = %+v, want fixed values and items cleared", open)
	}

	if _, err := db.Exec(ctx, `DELETE FROM wikidata_classes WHERE qid = 'Q99002'`); err != nil {
		t.Fatal(err)
	}
	if err := CheckCatalogue(ctx, db); err != nil {
		t.Fatal(err)
	}
	if open := openReview(t, db); len(open) != 1 {
		t.Errorf("open = %+v, want a value kept open while its classes aren't all known", open)
	}
}

func TestUnreadableClassClearsOnceRead(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	if err := FlagForReview(ctx, db, []ReviewFlag{{Kind: UnreadableClass, QID: "Q42"}}); err != nil {
		t.Fatal(err)
	}
	if err := SaveClasses(ctx, db, []WikidataClass{{QID: "Q42"}}); err != nil {
		t.Fatal(err)
	}
	if open := openReview(t, db); len(open) != 1 {
		t.Errorf("open = %+v, want saving the empty row for an unreadable class to leave its entry", open)
	}
	if err := ClassesRead(ctx, db, []string{"Q42"}); err != nil {
		t.Fatal(err)
	}
	if open := openReview(t, db); len(open) != 0 {
		t.Errorf("open = %+v, want a class cleared once it's read", open)
	}
}
