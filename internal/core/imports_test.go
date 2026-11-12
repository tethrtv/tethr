package core

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/tethrtv/tethr/internal/dbtest"
)

func at(s string) *time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return &t
}

func traktFile() ImportFile {
	silo := IDs{TMDB: 125988, IMDB: "tt14688458"}
	season := int32(1)
	return ImportFile{
		History: []ImportItem{
			{Ref: inception(), Label: "Inception (2010)", At: at("2024-01-01T20:00:00Z"), Action: "scrobble"},
			{Ref: inception(), Label: "Inception (2010)", At: at("2025-06-01T20:00:00Z"), Action: "watch"},
			{Ref: episode(1, 1), Label: "Breaking Bad 1x01", At: at("2024-02-01T20:00:00Z"), Action: "checkin"},
			{Ref: episode(1, 2), Label: "Breaking Bad 1x02", At: at("2024-02-02T20:00:00Z")},
			{Ref: Ref{Kind: "movie"}, Label: "Home video (1998)", At: at("2024-03-01T20:00:00Z")},
		},
		Watchlist: []ImportItem{
			{Ref: Ref{Kind: "show", IDs: silo}, Label: "Silo (2023)", At: at("2026-07-07T13:02:19Z")},
			{Ref: Ref{Kind: "movie", IDs: IDs{TMDB: 1318447}}, Label: "Apex (2026)", Notes: "with Sam"},
		},
		Playback: []ImportItem{
			{Ref: episode(1, 3), Label: "Breaking Bad 1x03", At: at("2024-02-03T20:00:00Z"), Progress: 42.5},
		},
		Ratings: []ImportItem{
			{Ref: inception(), Label: "Inception (2010)", Rating: 9, At: at("2024-01-02T10:00:00Z")},
			{Ref: Ref{Kind: "season", Show: silo, Season: &season}, Label: "Silo season 1", Rating: 8},
		},
		Hidden: []ImportItem{
			{Ref: Ref{Kind: "show", IDs: silo}, Label: "Silo (2023)", Section: "progress_watched", At: at("2026-01-01T00:00:00Z")},
			{Ref: Ref{Kind: "show", IDs: silo}, Label: "Silo (2023)", Section: "made_up"},
		},
	}
}

func TestImport(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	caller, _, _ := connectedProfile(t, db)
	profile := caller.ProfileID

	if _, err := ScrobbleStop(ctx, db, caller, Scrobble{Ref: inception(), Progress: 100}); err != nil {
		t.Fatal(err)
	}
	if _, err := AddHistory(ctx, db, caller, []Watch{{Ref: inception(), At: *at("2024-01-01T20:00:00Z")}}); err != nil {
		t.Fatal(err)
	}

	parsed, err := CreateImport(ctx, db, profile, "trakt", "trakt.zip", traktFile())
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Status != "parsed" || parsed.Summary.History.Total != 5 || parsed.Summary.History.Unmatched != 1 || len(parsed.Summary.Unmatched) != 1 || parsed.Summary.Unmatched[0] != "Home video (1998)" {
		t.Fatalf("parsed = %+v", parsed)
	}
	if again, _, _ := History(ctx, db, profile, HistoryFilter{}, Page{Page: 1, Limit: 10}); len(again) != 2 {
		t.Errorf("parsing wrote history: %d entries", len(again))
	}

	queued, err := QueueImport(ctx, db, profile, parsed.ID)
	if err != nil || queued.Status != "queued" {
		t.Fatalf("queue = %+v, %v", queued, err)
	}
	if _, err := CreateImport(ctx, db, profile, "trakt", "again.zip", traktFile()); !errors.Is(err, ErrImportBusy) {
		t.Errorf("second import while one is queued: %v", err)
	}
	if found, _, err := ApplyNextImport(ctx, db); !found || err != nil {
		t.Fatalf("apply = %v, %v", found, err)
	}
	if found, _, err := ApplyNextImport(ctx, db); found || err != nil {
		t.Fatalf("nothing left to apply = %v, %v", found, err)
	}

	applied, err := ImportByID(ctx, db, profile, parsed.ID)
	if err != nil {
		t.Fatal(err)
	}
	s := applied.Summary
	if applied.Status != "applied" || applied.AppliedAt == nil {
		t.Errorf("status = %s", applied.Status)
	}
	if s.History.Added != 3 || s.History.Skipped != 1 || s.History.Unmatched != 1 {
		t.Errorf("history = %+v", s.History)
	}
	if s.Watchlist.Added != 2 || s.Playback.Added != 1 || s.Ratings.Added != 2 || s.Hidden.Added != 1 || s.Hidden.Skipped != 1 {
		t.Errorf("summary = %+v", s)
	}

	entries, total, err := History(ctx, db, profile, HistoryFilter{}, Page{Page: 1, Limit: 10})
	if err != nil || total != 5 {
		t.Fatalf("history after import = %d, %v", total, err)
	}
	actions := map[string]int{}
	for _, e := range entries {
		actions[e.Action]++
	}
	if actions["scrobble"] != 1 || actions["checkin"] != 1 || actions["watch"] != 3 {
		t.Errorf("actions = %v", actions)
	}
	list, _ := Watchlist(ctx, db, profile, "")
	if len(list) != 2 || list[0].Item.Title.TmdbID == nil || *list[0].Item.Title.TmdbID != 125988 || list[1].Notes == nil || *list[1].Notes != "with Sam" {
		t.Errorf("watchlist = %+v", list)
	}
	ratings, _ := Ratings(ctx, db, profile, "", nil)
	if len(ratings) != 2 || ratings[1].Rating != 9 || ratings[0].Item.Kind() != "season" {
		t.Errorf("ratings = %+v", ratings)
	}
	hidden, _ := Hidden(ctx, db, profile, "progress_watched", "")
	if len(hidden) != 1 {
		t.Errorf("hidden = %+v", hidden)
	}

	if _, err := UndoImport(ctx, db, profile, parsed.ID); err != nil {
		t.Fatal(err)
	}
	after, _, _ := History(ctx, db, profile, HistoryFilter{}, Page{Page: 1, Limit: 10})
	if len(after) != 2 {
		t.Errorf("undo left %d plays, want the 2 logged outside the import", len(after))
	}
	for _, check := range []func() (int, error){
		func() (int, error) { e, err := Watchlist(ctx, db, profile, ""); return len(e), err },
		func() (int, error) { e, err := Ratings(ctx, db, profile, "", nil); return len(e), err },
		func() (int, error) { e, err := Hidden(ctx, db, profile, "progress_watched", ""); return len(e), err },
		func() (int, error) {
			e, err := Playback(ctx, db, profile, "", Range{}, Page{Page: 1, Limit: 10})
			return len(e), err
		},
	} {
		if n, err := check(); n != 0 || err != nil {
			t.Errorf("left after undo: %d, %v", n, err)
		}
	}
	if _, err := UndoImport(ctx, db, profile, parsed.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second undo: %v", err)
	}
}

func TestImportNeedsSomething(t *testing.T) {
	db := dbtest.New(t)
	caller, _, _ := connectedProfile(t, db)
	_, err := CreateImport(context.Background(), db, caller.ProfileID, "trakt", "empty.zip", ImportFile{History: []ImportItem{{Ref: Ref{Kind: "movie"}}}})
	if !errors.Is(err, ErrEmptyImport) {
		t.Errorf("err = %v", err)
	}
}

func TestImportPlaysWithoutTimes(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	caller, _, _ := connectedProfile(t, db)
	profile := caller.ProfileID
	if _, err := AddHistory(ctx, db, caller, []Watch{{Ref: inception(), At: *at("2024-01-01T20:00:00Z")}}); err != nil {
		t.Fatal(err)
	}

	heat := Ref{Kind: "movie", IDs: IDs{TMDB: 949}}
	matrix := Ref{Kind: "movie", IDs: IDs{TMDB: 603}}
	parsed, err := CreateImport(ctx, db, profile, "trakt", "trakt.zip", ImportFile{History: []ImportItem{
		{Ref: inception(), Label: "Inception (2010)"},
		{Ref: heat, Label: "Heat (1995)"},
		{Ref: heat, Label: "Heat (1995)"},
		{Ref: matrix, Label: "The Matrix (1999)", At: at("2026-11-08T12:00:00Z"), Precision: "day"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := QueueImport(ctx, db, profile, parsed.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ApplyNextImport(ctx, db); err != nil {
		t.Fatal(err)
	}
	applied, _ := ImportByID(ctx, db, profile, parsed.ID)
	if h := applied.Summary.History; h.Added != 2 || h.Skipped != 2 {
		t.Errorf("history = %+v", h)
	}

	entries, _, err := History(ctx, db, profile, HistoryFilter{}, Page{Page: 1, Limit: 10})
	if err != nil || len(entries) != 3 {
		t.Fatalf("history = %+v, %v", entries, err)
	}
	want := []struct {
		tmdb      int32
		precision string
	}{{603, "day"}, {27205, "time"}, {949, "none"}}
	for i, w := range want {
		e := entries[i]
		if *e.Item.Title.TmdbID != w.tmdb || e.Precision != w.precision {
			t.Errorf("entry %d = %d %s, want %d %s", i, *e.Item.Title.TmdbID, e.Precision, w.tmdb, w.precision)
		}
	}
	if !entries[2].At.Equal(UnknownDate) {
		t.Errorf("undated play at %v", entries[2].At)
	}
}

func TestImportTraktIDs(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	caller, _, _ := connectedProfile(t, db)

	if _, err := LookUp(ctx, db, "movie", IDs{TMDB: 27205, IMDB: "tt1375666"}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "INSERT INTO external_ids (title_id, provider, value) SELECT id, 'imdb', 'tt0000002' FROM titles WHERE tmdb_id = 27205"); err != nil {
		t.Fatal(err)
	}
	when := at("2024-01-01T20:00:00Z")
	watched := func(kind string, ids IDs) ImportItem {
		return ImportItem{Ref: Ref{Kind: kind, IDs: ids}, Label: "x", At: when, Action: "watch"}
	}
	f := ImportFile{
		History: []ImportItem{
			watched("movie", IDs{Trakt: 481, TMDB: 603}),
			watched("movie", IDs{Trakt: 16662, TMDB: 27205, IMDB: "tt0000001"}),
			watched("movie", IDs{Trakt: 481, TMDB: 155}),
			watched("movie", IDs{Trakt: 16663, TMDB: 27205, IMDB: "tt0000002"}),
			{Ref: Ref{Kind: "episode", Show: IDs{Trakt: 1388, TMDB: 1396}, Season: ptr(int32(1)), Number: ptr(int32(1))}, Label: "x", At: when, Action: "watch"},
		},
		Ratings: []ImportItem{{Ref: Ref{Kind: "movie", IDs: IDs{Trakt: 999, TMDB: 603}}, Label: "x", At: when, Rating: 8}},
	}
	parsed, err := CreateImport(ctx, db, caller.ProfileID, "trakt", "trakt.zip", f)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := QueueImport(ctx, db, caller.ProfileID, parsed.ID); err != nil {
		t.Fatal(err)
	}
	_, conflicts, err := ApplyNextImport(ctx, db)
	if err != nil {
		t.Fatal(err)
	}

	byTrakt := func(kind string, n int64) int64 {
		t.Helper()
		items, err := LookUp(ctx, db, kind, IDs{Trakt: n}, false)
		if err != nil || len(items) > 1 {
			t.Fatalf("look up %s %d = %v, %v", kind, n, items, err)
		}
		if len(items) == 0 {
			return 0
		}
		return items[0].Title.ID
	}
	matrix, _ := LookUp(ctx, db, "movie", IDs{TMDB: 603}, false)
	if byTrakt("movie", 481) != matrix[0].Title.ID || byTrakt("show", 481) != 0 {
		t.Error("a film's Trakt number should find it, and only as a film")
	}
	breaking, _ := LookUp(ctx, db, "show", IDs{TMDB: 1396}, false)
	if byTrakt("show", 1388) != breaking[0].Title.ID {
		t.Error("an episode's show should learn its Trakt number")
	}
	inception, _ := LookUp(ctx, db, "movie", IDs{TMDB: 27205}, false)
	if byTrakt("movie", 16663) != inception[0].Title.ID {
		t.Error("an import matching any of a title's IMDb IDs agrees with it")
	}
	if byTrakt("movie", 16662) != 0 || byTrakt("movie", 999) != 0 {
		t.Error("a Trakt number should only fill a gap, and only when the import's other IDs agree")
	}
	var outcomes []string
	for _, c := range conflicts {
		outcomes = append(outcomes, c.Value+" "+c.Outcome)
	}
	want := []string{
		"16662 ignored an import's, its other ids disagree",
		"481 ignored an import's, another title has it",
		"999 ignored an import's, the title has one",
	}
	if fmt.Sprint(outcomes) != fmt.Sprint(want) {
		t.Errorf("conflicts = %v, want %v", outcomes, want)
	}
}
