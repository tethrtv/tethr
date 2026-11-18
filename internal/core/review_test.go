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

func TestAcceptFixesFromWikidata(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	account, _, err := CreateAccount(ctx, db, SignUp{Email: "viewer@example.com", Name: "Viewer", Username: "viewer"})
	if err != nil {
		t.Fatal(err)
	}
	caller, err := AccountCaller(ctx, db, account.ID)
	if err != nil {
		t.Fatal(err)
	}
	moved := reviewTitle(t, db, "movie", WikidataTitle{QID: "Q1", Name: "Moved", Year: 2001, TMDB: 1})
	taken := reviewTitle(t, db, "movie", WikidataTitle{QID: "Q2", Name: "Taken", Year: 2002, TMDB: 2})
	reviewTitle(t, db, "movie", WikidataTitle{QID: "Q3", Name: "Holder", Year: 2003, TMDB: 33})
	show := reviewTitle(t, db, "show", WikidataTitle{QID: "Q4", Name: "Renumbered", Year: 2004, TMDB: 4})
	if err := SaveOutline(ctx, db, show, []OutlineEpisode{{TMDBID: 401, Season: 1, Number: 1}}); err != nil {
		t.Fatal(err)
	}
	season, number := int32(1), int32(1)
	if _, err := ScrobbleStop(ctx, db, caller, Scrobble{Ref: Ref{Kind: "episode", Show: IDs{TMDB: 4}, Season: &season, Number: &number}, Progress: 100}); err != nil {
		t.Fatal(err)
	}
	twin := reviewTitle(t, db, "show", WikidataTitle{QID: "Q5", Name: "Twin", Year: 2005, TMDB: 5})
	watched := reviewTitle(t, db, "show", WikidataTitle{QID: "Q6", Name: "Watched", Year: 2006, TMDB: 6})
	if _, err := ScrobbleStop(ctx, db, caller, Scrobble{Ref: Ref{Kind: "episode", Show: IDs{TMDB: 6}, Season: &season, Number: &number}, Progress: 100}); err != nil {
		t.Fatal(err)
	}
	gone := reviewTitle(t, db, "movie", WikidataTitle{QID: "Q7", Name: "Gone", Year: 2007, TMDB: 7})
	flags := []ReviewFlag{
		{Kind: TMDBDiffers, QID: "Q1", Title: moved, Subject: "tmdb", Wikidata: "11"},
		{Kind: TMDBDiffers, QID: "Q2", Title: taken, Subject: "tmdb", Wikidata: "33"},
		{Kind: TMDBDiffers, QID: "Q4", Title: show, Subject: "tmdb", Wikidata: "44"},
		{Kind: KindChange, QID: "Q5", Title: twin, Tethr: "show", Wikidata: "movie"},
		{Kind: KindChange, QID: "Q6", Title: watched, Tethr: "show", Wikidata: "movie"},
		{Kind: DeletedItem, QID: "Q7", Title: gone},
		{Kind: FilmAndShow, QID: "Q5", Title: twin},
	}
	if err := FlagForReview(ctx, db, flags); err != nil {
		t.Fatal(err)
	}
	ids := map[string]int64{}
	for _, e := range openReview(t, db) {
		ids[e.Kind+" "+e.QID] = e.ID
	}
	accept := func(key string) error {
		t.Helper()
		_, err := AcceptReview(ctx, db, strconv.FormatInt(ids[key], 10))
		return err
	}
	tmdbOf := func(id int64) int32 {
		t.Helper()
		var tmdb *int32
		if err := db.QueryRow(ctx, `SELECT tmdb_id FROM titles WHERE id = $1`, id).Scan(&tmdb); err != nil {
			t.Fatal(err)
		}
		return deref(tmdb)
	}
	exists := func(id int64) bool {
		t.Helper()
		var n int
		if err := db.QueryRow(ctx, `SELECT count(*) FROM titles WHERE id = $1`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n == 1
	}

	if err := accept("tmdb_differs Q1"); err != nil || tmdbOf(moved) != 11 {
		t.Errorf("accepting a TMDB disagreement = %v, TMDB %d, want Wikidata's 11", err, tmdbOf(moved))
	}
	if err := accept("tmdb_differs Q2"); !errors.Is(err, ErrTMDBTaken) || tmdbOf(taken) != 2 {
		t.Errorf("accepting a TMDB ID another title has = %v, TMDB %d, want it refused", err, tmdbOf(taken))
	}
	if err := accept("tmdb_differs Q4"); err != nil || tmdbOf(show) != 44 {
		t.Errorf("accepting a show's TMDB disagreement = %v, TMDB %d", err, tmdbOf(show))
	}
	var episodeTMDB *int32
	var watches int
	if err := db.QueryRow(ctx, `SELECT e.tmdb_episode_id, (SELECT count(*) FROM watch_events w WHERE w.episode_id = e.id) FROM episodes e WHERE e.show_id = $1`, show).Scan(&episodeTMDB, &watches); err != nil {
		t.Fatal(err)
	}
	if episodeTMDB != nil || watches != 1 {
		t.Errorf("episode after a new show ID: TMDB %v, %d watches, want the old ID forgotten and the watch kept", episodeTMDB, watches)
	}
	if err := SaveOutline(ctx, db, show, []OutlineEpisode{{TMDBID: 4401, Season: 1, Number: 1}}); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM watch_events w JOIN episodes e ON e.id = w.episode_id WHERE e.show_id = $1 AND e.tmdb_episode_id = 4401 AND e.removed_at IS NULL`, show).Scan(&watches); err != nil || watches != 1 {
		t.Errorf("watches on the new show's episode = %d, %v, want the outline to match it by number", watches, err)
	}

	if err := accept("kind_change Q6"); !errors.Is(err, ErrTitleInUse) || !exists(watched) {
		t.Errorf("accepting a kind change on a watched title = %v, want it refused and the title kept", err)
	}
	if err := accept("kind_change Q5"); err != nil || exists(twin) {
		t.Errorf("accepting a kind change = %v, exists %v, want the unused title retired", err, exists(twin))
	}
	if err := accept("deleted_item Q7"); err != nil || exists(gone) {
		t.Errorf("accepting a deleted item = %v, exists %v, want the unused title retired", err, exists(gone))
	}
	var kept, closed int
	if err := db.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE cleared_at IS NOT NULL) FROM review_entries WHERE qid = 'Q5'`).Scan(&kept, &closed); err != nil || kept != 2 || closed != 2 {
		t.Errorf("Q5's entries after retiring its title: %d kept, %d closed, %v, want both kept as history", kept, closed, err)
	}

	var open []string
	for _, e := range openReview(t, db) {
		open = append(open, e.Kind+" "+e.QID)
	}
	if fmt.Sprint(open) != "[kind_change Q6 tmdb_differs Q2]" {
		t.Errorf("open = %v", open)
	}
}

func TestCatalogueChecks(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	if err := SaveClasses(ctx, db, []WikidataClass{
		{QID: "Q11424", Label: "film"},
		{QID: "Q130232", Label: "drama film", Parents: []string{"Q25372"}},
		{QID: "Q99062", Label: "Doggerland"},
		{QID: "Q183", Label: "Germany", Country: "DE"},
		{QID: "Q99001", Label: "mumblecore", Parents: []string{"Q99002"}},
		{QID: "Q99002", Label: "style"},
	}); err != nil {
		t.Fatal(err)
	}
	for i := range reviewThreshold {
		reviewTitle(t, db, "movie", WikidataTitle{QID: fmt.Sprintf("Q%d", 500+i), Name: fmt.Sprintf("Film %d", i), Year: 2001, TMDB: int32(500 + i),
			InstanceOf: []string{"Q11424"}, Genres: []string{"Q130232", "Q99001"}, Countries: []string{"Q99062", "Q183"}})
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
		"unmapped_value country Q99062": true,
		"unmapped_value genre Q99001":   true,
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("open = %v, want %v (mapped values, rare values and the drama map left alone)", got, want)
	}

	if _, err := db.Exec(ctx, `UPDATE titles SET wikidata_countries = '{}' WHERE wikidata_countries @> '{Q99062}'`); err != nil {
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
