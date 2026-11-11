package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tethrtv/tethr/internal/dbtest"
)

func inception() Ref {
	return Ref{Kind: "movie", IDs: IDs{TMDB: 27205, IMDB: "tt1375666"}}
}

func episode(season, number int32) Ref {
	return Ref{Kind: "episode", Show: IDs{TMDB: 1396}, Season: &season, Number: &number}
}

func TestScrobbleRules(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	caller, _, _ := connectedProfile(t, db)

	started, err := ScrobbleStart(ctx, db, caller, Scrobble{Ref: inception(), Progress: 2})
	if err != nil {
		t.Fatal(err)
	}
	movie := started.Item.Title
	if started.Action != "start" || movie.ID != 1 || movie.Slug != "t-1" || movie.Imdb != "tt1375666" {
		t.Errorf("start = %+v", started)
	}

	paused, err := ScrobblePause(ctx, db, caller, Scrobble{Ref: Ref{Kind: "movie", IDs: IDs{IMDB: "tt1375666"}}, Progress: 41.5})
	if err != nil {
		t.Fatal(err)
	}
	if paused.Item.Title.ID != movie.ID || paused.ID != 1 {
		t.Errorf("pause matched another title or didn't count from 1: %+v", paused)
	}
	playback, err := Playback(ctx, db, caller.ProfileID, "", Range{}, Page{Page: 1, Limit: 100})
	if err != nil || len(playback) != 1 || playback[0].Progress != 41.5 {
		t.Fatalf("playback = %+v, %v", playback, err)
	}

	stopped, err := ScrobbleStop(ctx, db, caller, Scrobble{Ref: inception(), Progress: 50})
	if err != nil || stopped.Action != "pause" {
		t.Fatalf("stop under 80%% = %+v, %v", stopped, err)
	}
	if _, err := ScrobbleStop(ctx, db, caller, Scrobble{Ref: inception(), Progress: 0.5}); !errors.Is(err, ErrProgressTooLow) {
		t.Errorf("stop under 1%%: %v", err)
	}

	if _, err := AddToWatchlist(ctx, db, caller.ProfileID, []Listing{{Ref: inception()}}); err != nil {
		t.Fatal(err)
	}
	stopped, err = ScrobbleStop(ctx, db, caller, Scrobble{Ref: inception(), Progress: 91})
	if err != nil || stopped.Action != "scrobble" || stopped.ID != 1 {
		t.Fatalf("stop at 91%% = %+v, %v", stopped, err)
	}
	var already AlreadyScrobbled
	if _, err := ScrobbleStop(ctx, db, caller, Scrobble{Ref: inception(), Progress: 95}); !errors.As(err, &already) {
		t.Errorf("second stop: %v", err)
	}

	if playback, _ := Playback(ctx, db, caller.ProfileID, "", Range{}, Page{Page: 1, Limit: 100}); len(playback) != 0 {
		t.Errorf("playback kept after scrobble: %+v", playback)
	}
	if listed, _ := Watchlist(ctx, db, caller.ProfileID, ""); len(listed) != 0 {
		t.Errorf("watched film stayed on the watchlist: %+v", listed)
	}
	history, total, err := History(ctx, db, caller.ProfileID, HistoryFilter{}, Page{Page: 1, Limit: 10})
	if err != nil || total != 1 || history[0].Action != "scrobble" || history[0].Item.Title.ID != movie.ID {
		t.Fatalf("history = %+v (%d), %v", history, total, err)
	}

	activity, err := LastActivities(ctx, db, caller.ProfileID)
	if err != nil {
		t.Fatal(err)
	}
	for _, area := range []string{"movies.watched_at", "movies.paused_at", "movies.watchlisted_at", "watchlist.updated_at"} {
		if activity.Marks[area].IsZero() {
			t.Errorf("%s not marked", area)
		}
	}
}

func TestEpisodesAreCreatedFromNumbers(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	caller, _, _ := connectedProfile(t, db)

	first, err := ScrobbleStop(ctx, db, caller, Scrobble{Ref: episode(1, 1), Progress: 100})
	if err != nil {
		t.Fatal(err)
	}
	second, err := ScrobbleStop(ctx, db, caller, Scrobble{Ref: episode(1, 2), Progress: 100})
	if err != nil {
		t.Fatal(err)
	}
	if first.Item.Title.ID != second.Item.Title.ID || first.Item.Episode.ID == second.Item.Episode.ID {
		t.Errorf("episodes = %+v, %+v", first.Item, second.Item)
	}
	if first.Item.Title.Kind != "show" || *first.Item.Title.TmdbID != 1396 {
		t.Errorf("show = %+v", first.Item.Title)
	}

	byID := Ref{Kind: "episode", IDs: IDs{Tethr: second.Item.Episode.ID}}
	added, err := AddHistory(ctx, db, caller, []Watch{
		{Ref: byID, At: time.Date(2024, 3, 1, 20, 0, 0, 0, time.UTC)},
		{Ref: byID, At: time.Date(2024, 3, 1, 20, 0, 0, 0, time.UTC)},
		{Ref: Ref{Kind: "episode", IDs: IDs{Tethr: 12345}}, At: time.Now()},
		{Ref: Ref{Kind: "show", IDs: IDs{TMDB: 1396}}, At: time.Now()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if added.Added.Episodes != 1 || len(added.NotFound) != 2 {
		t.Errorf("add history = %+v", added)
	}

	episodes, total, err := History(ctx, db, caller.ProfileID, HistoryFilter{Kind: "episode", ID: second.Item.Episode.ID}, Page{Page: 1, Limit: 10})
	if err != nil || total != 2 || len(episodes) != 2 {
		t.Errorf("history for one episode = %d, %v", total, err)
	}

	season := int32(1)
	removed, err := RemoveHistory(ctx, db, caller.ProfileID, []int64{first.ID, 12345}, []Ref{{Kind: "season", Show: IDs{TMDB: 1396}, Season: &season}})
	if err != nil {
		t.Fatal(err)
	}
	if removed.Deleted.Episodes != 3 || len(removed.NotIDs) != 1 || removed.NotIDs[0] != 12345 {
		t.Errorf("remove = %+v", removed)
	}
}

func TestWatchlist(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	caller, _, _ := connectedProfile(t, db)

	shows, err := LookUp(ctx, db, "show", IDs{TMDB: 1396}, true)
	if err != nil || len(shows) != 1 {
		t.Fatalf("look up = %+v, %v", shows, err)
	}
	slug := shows[0].Title.Slug
	result, err := AddToWatchlist(ctx, db, caller.ProfileID, []Listing{
		{Ref: Ref{Kind: "show", IDs: IDs{Slug: slug}}},
		{Ref: inception()},
		{Ref: Ref{Kind: "movie", IDs: IDs{Slug: "not-a-film"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Added.Shows != 1 || result.Added.Movies != 1 || len(result.NotFound) != 1 || result.Listed != 2 {
		t.Errorf("add = %+v", result)
	}
	again, err := AddToWatchlist(ctx, db, caller.ProfileID, []Listing{{Ref: inception()}})
	if err != nil || again.Existing.Movies != 1 || again.Added.Movies != 0 {
		t.Errorf("adding twice = %+v, %v", again, err)
	}

	listed, err := Watchlist(ctx, db, caller.ProfileID, "")
	if err != nil || len(listed) != 2 || listed[0].Rank != 1 || listed[0].Item.Kind() != "show" {
		t.Fatalf("watchlist = %+v, %v", listed, err)
	}
	if movies, _ := Watchlist(ctx, db, caller.ProfileID, "movie"); len(movies) != 1 {
		t.Errorf("movies = %+v", movies)
	}

	removed, err := RemoveFromWatchlist(ctx, db, caller.ProfileID, []Ref{{Kind: "show", IDs: IDs{Slug: slug}}})
	if err != nil || removed.Deleted.Shows != 1 || removed.Listed != 1 {
		t.Errorf("remove = %+v, %v", removed, err)
	}

	if found, _ := LookUp(ctx, db, "", IDs{TMDB: 1396}, true); len(found) != 1 || found[0].Title.Kind != "show" {
		t.Errorf("untyped look up = %+v", found)
	}
	if found, _ := LookUp(ctx, db, "", IDs{TMDB: 99999}, true); len(found) != 0 {
		t.Errorf("untyped look up created a title: %+v", found)
	}
}

func TestDroppedStops(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	caller, _, _ := connectedProfile(t, db)
	length := 2700.0
	start := func(number int32, progress float64) {
		t.Helper()
		if _, err := ScrobbleStart(ctx, db, caller, Scrobble{Ref: episode(3, number), Progress: progress, Duration: &length}); err != nil {
			t.Fatal(err)
		}
	}
	rewind := func(by string) {
		t.Helper()
		if _, err := db.Exec(ctx, `UPDATE watching SET started_at = started_at - $1::interval, expires_at = expires_at - $1::interval`, by); err != nil {
			t.Fatal(err)
		}
	}
	plays := func() []Entry {
		t.Helper()
		history, _, err := History(ctx, db, caller.ProfileID, HistoryFilter{}, Page{Page: 1, Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		return history
	}

	start(8, 0)
	if _, err := ScrobblePause(ctx, db, caller, Scrobble{Ref: episode(3, 8), Progress: 56, Duration: &length}); err != nil {
		t.Fatal(err)
	}
	start(8, 56)
	rewind("20 minutes")
	start(9, 0)
	got := plays()
	if len(got) != 1 || got[0].Item.Episode == nil || got[0].Item.Episode.Number != 8 || got[0].Action != "scrobble" {
		t.Fatalf("plays after moving on = %+v", got)
	}
	if playback, _ := Playback(ctx, db, caller.ProfileID, "", Range{}, Page{Page: 1, Limit: 10}); len(playback) != 0 {
		t.Errorf("paused point kept = %+v", playback)
	}

	start(10, 0)
	if got := plays(); len(got) != 1 {
		t.Errorf("counted an episode left after a minute: %+v", got)
	}

	rewind("3 hours")
	start(11, 0)
	if got := plays(); len(got) != 1 {
		t.Errorf("counted an episode started hours ago: %+v", got)
	}

	rewind("40 minutes")
	start(11, 0)
	if got := plays(); len(got) != 1 {
		t.Errorf("counted an episode started again: %+v", got)
	}

	rewind("60 minutes")
	start(12, 0)
	if got := plays(); len(got) != 2 {
		t.Fatalf("plays after the last one ran out = %+v", got)
	}
	_, err := ScrobbleStop(ctx, db, caller, Scrobble{Ref: episode(3, 11), Progress: 100})
	if _, ok := errors.AsType[AlreadyScrobbled](err); !ok {
		t.Errorf("late stop = %v", err)
	}
	if got := plays(); len(got) != 2 {
		t.Errorf("late stop counted again: %+v", got)
	}
}
