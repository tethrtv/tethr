package core

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/tethrtv/tethr/internal/dbtest"
)

func TestRewatch(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	caller, _, _ := connectedProfile(t, db)
	profile := caller.ProfileID
	watch := func(season, number int32) int64 {
		t.Helper()
		r, err := ScrobbleStop(ctx, db, caller, Scrobble{Ref: episode(season, number), Progress: 100})
		if err != nil {
			t.Fatal(err)
		}
		return r.Item.Title.ID
	}
	again := func(season, number int32) {
		t.Helper()
		if _, err := AddHistory(ctx, db, caller, []Watch{{Ref: episode(season, number), At: time.Now()}}); err != nil {
			t.Fatal(err)
		}
	}
	next := func() []UpNextEntry {
		t.Helper()
		entries, err := UpNext(ctx, db, profile)
		if err != nil {
			t.Fatal(err)
		}
		return entries
	}

	show := watch(1, 1)
	if err := SaveOutline(ctx, db, show, []OutlineEpisode{
		{TMDBID: 101, Season: 1, Number: 1, FirstAired: aired(30)},
		{TMDBID: 102, Season: 1, Number: 2, FirstAired: aired(20)},
		{TMDBID: 103, Season: 1, Number: 3, FirstAired: aired(10)},
	}); err != nil {
		t.Fatal(err)
	}
	watch(1, 2)
	watch(1, 3)
	if got := next(); len(got) != 0 {
		t.Fatalf("up next once finished = %+v", got)
	}

	if err := StartRewatch(ctx, db, profile, show); err != nil {
		t.Fatal(err)
	}
	got := next()
	if len(got) != 1 || got[0].RewatchFrom == nil || got[0].Next.EpisodeNumber != 1 || got[0].Watched != 0 || got[0].Aired != 3 || got[0].LastWatchedAt.IsZero() {
		t.Fatalf("up next at the start of a rewatch = %+v", got)
	}

	again(1, 1)
	got = next()
	if len(got) != 1 || got[0].Next.EpisodeNumber != 2 || got[0].Watched != 1 {
		t.Fatalf("up next one episode in = %+v", got)
	}
	p, err := Progress(ctx, db, profile, Ref{Kind: "show", IDs: IDs{TMDB: 1396}}, false)
	if err != nil || p.Completed != 1 || p.ResetAt == nil {
		t.Errorf("progress = %+v, %v", p, err)
	}
	state, err := Title(ctx, db, profile, strconv.FormatInt(show, 10), "show")
	if err != nil || state.RewatchFrom == nil || !state.RewatchFrom.Equal(*p.ResetAt) {
		t.Errorf("title = %+v, %v", state.RewatchFrom, err)
	}

	again(1, 2)
	again(1, 3)
	if got := next(); len(got) != 0 {
		t.Errorf("up next once the rewatch is done = %+v", got)
	}

	if err := StopRewatch(ctx, db, profile, show); err != nil {
		t.Fatal(err)
	}
	if err := StopRewatch(ctx, db, profile, show); !errors.Is(err, ErrNotFound) {
		t.Errorf("stopping twice = %v", err)
	}
	if state, _ := Title(ctx, db, profile, strconv.FormatInt(show, 10), "show"); state.RewatchFrom != nil {
		t.Errorf("still rewatching after stopping: %v", state.RewatchFrom)
	}

	film, err := ScrobbleStop(ctx, db, caller, Scrobble{Ref: inception(), Progress: 100})
	if err != nil {
		t.Fatal(err)
	}
	if err := StartRewatch(ctx, db, profile, film.Item.Title.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("rewatching a film = %v", err)
	}
	if err := StartRewatch(ctx, db, profile, 999999); !errors.Is(err, ErrNotFound) {
		t.Errorf("rewatching nothing = %v", err)
	}
}
