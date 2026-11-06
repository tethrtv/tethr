package core

import (
	"context"
	"errors"
	"testing"

	"github.com/tethrtv/tethr/internal/dbtest"
)

func TestEpisodeOrder(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	caller, _, _ := connectedProfile(t, db)
	profile := caller.ProfileID

	first, err := ScrobbleStop(ctx, db, caller, Scrobble{Ref: episode(1, 1), Progress: 100})
	if err != nil {
		t.Fatal(err)
	}
	show := first.Item.Title.ID
	if err := SaveOutline(ctx, db, show, []OutlineEpisode{
		{TMDBID: 900, Season: 0, Number: 1, FirstAired: aired(400)},
		{TMDBID: 101, Season: 1, Number: 1, FirstAired: aired(300)},
		{TMDBID: 102, Season: 1, Number: 2, FirstAired: aired(293)},
		{TMDBID: 103, Season: 1, Number: 3, FirstAired: aired(286)},
		{TMDBID: 104, Season: 1, Number: 4, FirstAired: aired(279)},
	}); err != nil {
		t.Fatal(err)
	}
	dvd := EpisodeOrder{TMDBGroupID: "dvd", TMDBType: 3, Episodes: []OrderEntry{
		{TMDBID: 101, ShowTMDBID: 1396, Season: 1, Number: 1},
		{TMDBID: 900, ShowTMDBID: 1396, Season: 1, Number: 2},
		{TMDBID: 103, ShowTMDBID: 1396, Season: 2, Number: 1},
		{TMDBID: 102, ShowTMDBID: 1396, Season: 2, Number: 2},
	}}

	if err := PickEpisodeOrder(ctx, db, profile, "tmdb-1396", &EpisodeOrder{TMDBGroupID: "other", Episodes: []OrderEntry{{TMDBID: 1, ShowTMDBID: 1399}}}); !errors.Is(err, ErrWrongShow) {
		t.Errorf("another show's group = %v", err)
	}
	if err := PickEpisodeOrder(ctx, db, profile, "tmdb-27205", &dvd); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown show = %v", err)
	}
	if err := PickEpisodeOrder(ctx, db, profile, "tmdb-1396", &dvd); err != nil {
		t.Fatal(err)
	}

	next, err := UpNext(ctx, db, profile)
	if err != nil || len(next) != 1 {
		t.Fatalf("up next = %+v, %v", next, err)
	}
	if n := next[0].Next; n.Season != 1 || n.Number != 2 || n.SeasonNumber != 0 || next[0].Aired != 4 {
		t.Errorf("next = %+v, aired %d; the special is second on the dvd", n, next[0].Aired)
	}
	p, err := Progress(ctx, db, profile, Ref{Kind: "show", IDs: IDs{TMDB: 1396}}, false)
	if err != nil || len(p.Seasons) != 2 || p.Seasons[1].Number != 2 || p.Seasons[1].Episodes[0].Episode.EpisodeNumber != 3 {
		t.Errorf("progress = %+v, %v", p, err)
	}
	title, err := Title(ctx, db, profile, "tmdb-1396", "show")
	if err != nil || title.Order != "dvd" || len(title.Seasons) != 2 || len(title.Seasons[0].Episodes) != 2 {
		t.Errorf("title = %+v, %v", title, err)
	}

	household, err := AddProfile(ctx, db, caller.AccountID, NewProfile{Name: "Alex", Kind: "person"})
	if err != nil {
		t.Fatal(err)
	}
	if title, _ := Title(ctx, db, household.ID, "tmdb-1396", "show"); title.Order != "" || len(title.Seasons) != 2 || title.Seasons[1].Number != 0 {
		t.Errorf("another profile should see aired order: %+v", title)
	}

	dvd.Episodes = dvd.Episodes[:3]
	if err := RefreshEpisodeOrder(ctx, db, dvd); err != nil {
		t.Fatal(err)
	}
	if p, _ := Progress(ctx, db, profile, Ref{Kind: "show", IDs: IDs{TMDB: 1396}}, false); p.Aired != 3 {
		t.Errorf("after the group lost an episode = %+v", p)
	}

	if err := EpisodeOrderMissing(ctx, db, "dvd"); err != nil {
		t.Fatal(err)
	}
	if title, _ := Title(ctx, db, profile, "tmdb-1396", "show"); title.Order != "" || title.Seasons[0].Episodes[3].Episode.Number != 4 {
		t.Errorf("a group gone from TMDB should leave aired order: %+v", title)
	}
	if err := PickEpisodeOrder(ctx, db, profile, "tmdb-1396", &dvd); err != nil {
		t.Fatal(err)
	}
	if err := PickEpisodeOrder(ctx, db, profile, "tmdb-1396", nil); err != nil {
		t.Fatal(err)
	}
	if due, err := DueEpisodeOrders(ctx, db, 10); err != nil || len(due) != 0 {
		t.Errorf("due = %v, %v", due, err)
	}
	var orders int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM episode_orders").Scan(&orders); err != nil || orders != 0 {
		t.Errorf("an order nobody follows should go: %d, %v", orders, err)
	}
}
