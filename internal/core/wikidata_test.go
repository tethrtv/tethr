package core

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/dbtest"
)

func TestMergeSharedWikidata(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	caller, _, _ := connectedProfile(t, db)
	byTVDB, byTMDB := IDs{TVDB: 121361}, IDs{TMDB: 1399}
	ep := func(show IDs, season, number int32) Ref {
		return Ref{Kind: "episode", Show: show, Season: &season, Number: &number}
	}
	at := time.Date(2024, 5, 1, 20, 0, 0, 0, time.UTC)

	if _, err := AddHistory(ctx, db, caller, []Watch{
		{Ref: ep(byTVDB, 1, 1), At: at},
		{Ref: ep(byTVDB, 1, 2), At: at.Add(time.Hour)},
		{Ref: ep(byTMDB, 1, 1), At: at.Add(24 * time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ScrobblePause(ctx, db, caller, Scrobble{Ref: ep(byTVDB, 1, 3), Progress: 40}); err != nil {
		t.Fatal(err)
	}
	if _, err := Rate(ctx, db, caller.ProfileID, []Rating{{Ref: Ref{Kind: "show", IDs: byTVDB}, Rating: 6}, {Ref: Ref{Kind: "show", IDs: byTMDB}, Rating: 9}}); err != nil {
		t.Fatal(err)
	}
	if _, err := AddToWatchlist(ctx, db, caller.ProfileID, []Listing{{Ref: Ref{Kind: "season", Show: byTVDB, Season: ptr(int32(2))}}}); err != nil {
		t.Fatal(err)
	}

	shelf, err := CreateList(ctx, db, caller.ProfileID, ListChanges{Name: ptr("Fantasy")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddToList(ctx, db, caller.ProfileID, shelf.ID, []Listing{{Ref: Ref{Kind: "show", IDs: byTVDB}}}); err != nil {
		t.Fatal(err)
	}
	shelf, err = GetList(ctx, db, caller.ProfileID, shelf.ID)
	if err != nil {
		t.Fatal(err)
	}

	lookUp := func(ids IDs) int64 {
		t.Helper()
		items, err := LookUp(ctx, db, "show", ids, false)
		if err != nil || len(items) != 1 {
			t.Fatalf("look up %+v = %+v, %v", ids, items, err)
		}
		return items[0].Title.ID
	}
	loser, winner := lookUp(byTVDB), lookUp(byTMDB)
	if loser == winner {
		t.Fatal("expected two rows before the merge")
	}
	before, err := LastActivities(ctx, db, caller.ProfileID)
	if err != nil {
		t.Fatal(err)
	}

	got := WikidataTitle{QID: "Q23572", Name: "Game of Thrones", Year: 2011, Runtime: 50, TMDB: 1399}
	if err := SaveWikidata(ctx, db, winner, "show", got); err != nil {
		t.Fatal(err)
	}
	if err := SaveWikidata(ctx, db, loser, "show", got); err != nil {
		t.Fatal(err)
	}

	if lookUp(byTVDB) != winner || lookUp(IDs{Tethr: loser}) != winner {
		t.Error("the merged row's IDs should lead to the one it was merged into")
	}
	history, total, err := History(ctx, db, caller.ProfileID, HistoryFilter{}, Page{Page: 1, Limit: 10})
	if err != nil || total != 3 {
		t.Fatalf("history = %d, %v", total, err)
	}
	episodes := map[int64]bool{}
	for _, e := range history {
		if e.Item.Title.ID != winner {
			t.Errorf("watch still on the old row: %+v", e.Item)
		}
		episodes[e.Item.Episode.ID] = true
	}
	if len(episodes) != 2 {
		t.Errorf("1x01 from both rows should be one episode, 1x02 moved across: %d episodes", len(episodes))
	}
	if paused, err := Playback(ctx, db, caller.ProfileID, "", Range{}, Page{Page: 1, Limit: 10}); err != nil || len(paused) != 1 || paused[0].Item.Title.ID != winner {
		t.Errorf("playback = %+v, %v", paused, err)
	}
	if rated, err := Ratings(ctx, db, caller.ProfileID, "", nil); err != nil || len(rated) != 1 || rated[0].Rating != 9 {
		t.Errorf("the surviving row's rating should win: %+v, %v", rated, err)
	}
	listed, err := Watchlist(ctx, db, caller.ProfileID, "")
	if err != nil || len(listed) != 1 || listed[0].Item.Title.ID != winner {
		t.Fatalf("watchlist = %+v, %v", listed, err)
	}
	var season int64
	if err := db.QueryRow(ctx, "SELECT id FROM seasons WHERE show_id = $1 AND season_number = 2", winner).Scan(&season); err != nil || listed[0].Item.Season.ID != season {
		t.Errorf("the listed season should move to the surviving row's season 2 (%d): %+v, %v", season, listed[0].Item.Season, err)
	}
	if items, _ := LookUp(ctx, db, "show", byTMDB, false); items[0].Title.Tvdb != "121361" || *items[0].Title.Name != "Game of Thrones" {
		t.Errorf("the merged row's TVDB ID should move across: %+v", items[0].Title)
	}
	if merged, err := GetList(ctx, db, caller.ProfileID, shelf.ID); err != nil || !merged.UpdatedAt.After(shelf.UpdatedAt) {
		t.Errorf("a list holding the merged row should count as changed: %v", err)
	}
	after, err := LastActivities(ctx, db, caller.ProfileID)
	if err != nil {
		t.Fatal(err)
	}
	if !after.Marks["episodes.watched_at"].After(before.Marks["episodes.watched_at"]) {
		t.Error("apps should be told to sync again after a merge")
	}
	if due, _ := DueWikidata(ctx, db, 10); len(due) != 0 {
		t.Errorf("merged rows shouldn't be looked up again: %+v", due)
	}
}

func aWeekPasses(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	if _, err := db.Exec(context.Background(), "UPDATE titles SET wikidata_checked_at = wikidata_checked_at - interval '8 days'"); err != nil {
		t.Fatal(err)
	}
}

func settle(t *testing.T, db *pgxpool.Pool, id int64, kind string, w WikidataTitle) {
	t.Helper()
	for range 2 {
		if err := SaveWikidata(context.Background(), db, id, kind, w); err != nil {
			t.Fatal(err)
		}
		aWeekPasses(t, db)
	}
}

func TestSlugFor(t *testing.T) {
	for _, c := range []struct {
		name string
		year int
		want string
	}{
		{"Breaking Bad", 2008, "breaking-bad-2008"},
		{"Amélie", 2001, "amelie-2001"},
		{"Grey’s Anatomy", 2005, "greys-anatomy-2005"},
		{"Rob & Romesh Vs", 2019, "rob-romesh-vs-2019"},
		{"  M*A*S*H  ", 1972, "m-a-s-h-1972"},
		{"Axırıncı aşırım", 1971, "axirinci-asirim-1971"},
		{"Smørrebrød", 2010, "smorrebrod-2010"},
		{"Æon Flux", 2005, "aeon-flux-2005"},
		{"Die Straße", 1923, "die-strasse-1923"},
		{"Łódź", 1990, "lodz-1990"},
		{"進撃の巨人", 2013, ""},
		{"Doctor Who", 0, ""},
	} {
		if got := slugFor(c.name, c.year); got != c.want {
			t.Errorf("slugFor(%q, %d) = %q, want %q", c.name, c.year, got, c.want)
		}
	}
}

func TestNameSlugs(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	create := func(tmdb int64) int64 {
		t.Helper()
		items, err := LookUp(ctx, db, "movie", IDs{TMDB: tmdb}, true)
		if err != nil {
			t.Fatal(err)
		}
		return items[0].Title.ID
	}
	slug := func(id int64) string {
		t.Helper()
		items, err := LookUp(ctx, db, "movie", IDs{Tethr: id}, false)
		if err != nil {
			t.Fatal(err)
		}
		return items[0].Title.Slug
	}
	first, second, unnamed := create(194), create(9194), create(19194)
	released := time.Date(2001, 4, 25, 0, 0, 0, 0, time.UTC)

	settle(t, db, first, "movie", WikidataTitle{QID: "Q484048", Name: "Amélie", Year: 2001, Released: released})
	settle(t, db, second, "movie", WikidataTitle{QID: "Q1", Name: "Amelie", Year: 2001, Released: released})
	settle(t, db, unnamed, "movie", WikidataTitle{QID: "Q2", Name: "Untitled"})
	if got := slug(first); got != "amelie-2001" {
		t.Errorf("slug = %q", got)
	}
	if got := slug(second); got != "amelie-2001-2" {
		t.Errorf("a clash should get a number: %q", got)
	}
	if got := slug(unnamed); got != fmt.Sprintf("t-%d", unnamed) {
		t.Errorf("no year should keep the ID slug: %q", got)
	}

	if err := SaveWikidata(ctx, db, first, "movie", WikidataTitle{QID: "Q484048", Name: "Le Fabuleux Destin d'Amélie Poulain", Year: 2001, Released: released}); err != nil {
		t.Fatal(err)
	}
	if got := slug(first); got != "amelie-2001" {
		t.Errorf("a slug shouldn't change once set: %q", got)
	}
	if items, err := LookUp(ctx, db, "", IDs{Slug: fmt.Sprintf("t-%d", first)}, false); err != nil || len(items) != 1 || items[0].Title.ID != first {
		t.Errorf("the ID slug should still resolve: %+v, %v", items, err)
	}
}

func TestSlugsPerKind(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	create := func(kind string, tmdb int64) int64 {
		t.Helper()
		items, err := LookUp(ctx, db, kind, IDs{TMDB: tmdb}, true)
		if err != nil {
			t.Fatal(err)
		}
		return items[0].Title.ID
	}
	film, show := create("movie", 2661), create("show", 2287)
	released := time.Date(1966, 1, 12, 0, 0, 0, 0, time.UTC)
	settle(t, db, film, "movie", WikidataTitle{QID: "Q1056053", Name: "Batman", Year: 1966, Released: released})
	settle(t, db, show, "show", WikidataTitle{QID: "Q2900869", Name: "Batman", Year: 1966, Released: released})
	for kind, id := range map[string]int64{"movie": film, "show": show} {
		items, err := LookUp(ctx, db, kind, IDs{Slug: "batman-1966"}, false)
		if err != nil || len(items) != 1 || items[0].Title.ID != id {
			t.Errorf("%s batman-1966 = %+v, %v", kind, items, err)
		}
	}
	if items, err := LookUp(ctx, db, "", IDs{Slug: "batman-1966"}, false); err != nil || len(items) != 2 {
		t.Errorf("without a kind, both should come back: %+v, %v", items, err)
	}
	if got, err := Title(ctx, db, 0, "batman-1966", "show"); err != nil || got.Item.Title.ID != show {
		t.Errorf("the show's page = %+v, %v", got.Item.Title, err)
	}
}

func TestSlugWaitsForAWeek(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	items, err := LookUp(ctx, db, "movie", IDs{TMDB: 1061474}, true)
	if err != nil {
		t.Fatal(err)
	}
	id := items[0].Title.ID
	save := func(name string, released time.Time) string {
		t.Helper()
		if err := SaveWikidata(ctx, db, id, "movie", WikidataTitle{QID: "Q3", Name: name, Year: released.Year(), Released: released}); err != nil {
			t.Fatal(err)
		}
		items, err := LookUp(ctx, db, "movie", IDs{Tethr: id}, false)
		if err != nil {
			t.Fatal(err)
		}
		return items[0].Title.Slug
	}
	placeholder := fmt.Sprintf("t-%d", id)
	next, yesterday := time.Now().AddDate(1, 0, 0), time.Now().AddDate(0, 0, -1)

	if got := save("Working Title", next); got != placeholder {
		t.Errorf("on first sight = %q", got)
	}
	aWeekPasses(t, db)
	if got := save("Working Title", next); got != placeholder {
		t.Errorf("an unreleased title should keep its ID slug: %q", got)
	}
	aWeekPasses(t, db)
	if due, err := DueWikidata(ctx, db, 10); err != nil || len(due) != 1 {
		t.Errorf("a title waiting for its slug should be looked at again within a week: %+v, %v", due, err)
	}
	if got := save("Supermen", yesterday); got != placeholder {
		t.Errorf("a name seen once shouldn't become a slug: %q", got)
	}
	if got := save("Supermen", yesterday); got != placeholder {
		t.Errorf("the same name a moment later isn't a week: %q", got)
	}
	aWeekPasses(t, db)
	if got := save("Superman", yesterday); got != placeholder {
		t.Errorf("a renamed title should wait another week: %q", got)
	}
	aWeekPasses(t, db)
	if got, want := save("Superman", yesterday), fmt.Sprintf("superman-%d", yesterday.Year()); got != want {
		t.Errorf("after a settled week, slug = %q, want %q", got, want)
	}
}
