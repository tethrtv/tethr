package core

import (
	"context"
	"testing"

	"github.com/tethrtv/tethr/internal/dbtest"
)

func TestUpcoming(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	caller, _, _ := connectedProfile(t, db)
	profile := caller.ProfileID
	ep := func(show int64, season, number int32) Ref {
		return Ref{Kind: "episode", Show: IDs{TMDB: show}, Season: &season, Number: &number}
	}
	watch := func(r Ref) int64 {
		t.Helper()
		res, err := ScrobbleStop(ctx, db, caller, Scrobble{Ref: r, Progress: 100})
		if err != nil {
			t.Fatal(err)
		}
		return res.Item.Title.ID
	}
	outline := func(show int64, episodes []OutlineEpisode) {
		t.Helper()
		if err := SaveOutline(ctx, db, show, episodes); err != nil {
			t.Fatal(err)
		}
	}

	watching := watch(ep(1396, 1, 1))
	outline(watching, []OutlineEpisode{
		{TMDBID: 101, Season: 1, Number: 1, FirstAired: aired(300)},
		{TMDBID: 102, Season: 1, Number: 2, FirstAired: aired(-2)},
		{TMDBID: 103, Season: 1, Number: 3, FirstAired: aired(-9)},
		{TMDBID: 104, Season: 1, Number: 4},
		{TMDBID: 900, Season: 0, Number: 1, FirstAired: aired(-1)},
	})

	caughtUp := watch(ep(1399, 1, 1))
	outline(caughtUp, []OutlineEpisode{
		{TMDBID: 201, Season: 1, Number: 1, FirstAired: aired(100)},
		{TMDBID: 211, Season: 2, Number: 1, FirstAired: aired(-30)},
		{TMDBID: 212, Season: 2, Number: 2, FirstAired: aired(-37)},
	})

	dropped := watch(ep(1400, 1, 1))
	outline(dropped, []OutlineEpisode{
		{TMDBID: 301, Season: 1, Number: 1, FirstAired: aired(50)},
		{TMDBID: 302, Season: 1, Number: 2, FirstAired: aired(-3)},
	})
	if err := HideFromUpNext(ctx, db, profile, dropped); err != nil {
		t.Fatal(err)
	}

	if _, err := AddToWatchlist(ctx, db, profile, []Listing{{Ref: Ref{Kind: "show", IDs: IDs{TMDB: 2000}}}}); err != nil {
		t.Fatal(err)
	}
	list, _ := Watchlist(ctx, db, profile, "")
	if len(list) != 1 {
		t.Fatalf("watchlist = %+v", list)
	}
	wanted := list[0].Item.Title.ID
	outline(wanted, []OutlineEpisode{
		{TMDBID: 401, Season: 1, Number: 1, FirstAired: aired(-5)},
		{TMDBID: 402, Season: 1, Number: 2, FirstAired: aired(-6)},
	})

	got, err := Upcoming(ctx, db, profile)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		show      int64
		tmdb      int32
		watchlist bool
	}{{watching, 102, false}, {wanted, 401, true}, {caughtUp, 211, false}}
	if len(got) != len(want) {
		t.Fatalf("upcoming = %+v", got)
	}
	for i, w := range want {
		g := got[i]
		if g.Show.Title.ID != w.show || g.Episode.TmdbEpisodeID == nil || *g.Episode.TmdbEpisodeID != w.tmdb || g.Watchlist != w.watchlist {
			t.Errorf("entry %d = show %d episode %v watchlist %v, want %+v", i, g.Show.Title.ID, g.Episode.TmdbEpisodeID, g.Watchlist, w)
		}
	}
	if got[0].Episode.FirstAired == nil || !got[0].Episode.FirstAired.Equal(*aired(-2)) {
		t.Errorf("air date = %v", got[0].Episode.FirstAired)
	}

	if _, err := Hide(ctx, db, profile, "progress_watched", []Ref{{Kind: "season", Show: IDs{TMDB: 1399}, Season: ptr(int32(2))}}); err != nil {
		t.Fatal(err)
	}
	if got, _ := Upcoming(ctx, db, profile); len(got) != 2 || got[1].Show.Title.ID == caughtUp {
		t.Errorf("upcoming with a hidden season = %+v", got)
	}

	due, err := DueOutlines(ctx, db, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Errorf("due right after saving = %+v", due)
	}
}

func TestWatchlistShowsGetOutlines(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	caller, _, _ := connectedProfile(t, db)
	if _, err := AddToWatchlist(ctx, db, caller.ProfileID, []Listing{{Ref: Ref{Kind: "show", IDs: IDs{TMDB: 2000}}}, {Ref: inception()}}); err != nil {
		t.Fatal(err)
	}
	due, err := DueOutlines(ctx, db, 10)
	if err != nil || len(due) != 1 || due[0].TmdbID == nil || *due[0].TmdbID != 2000 {
		t.Errorf("due = %+v, %v", due, err)
	}
}
