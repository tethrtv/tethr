package core

import (
	"context"
	"testing"
	"time"

	"github.com/tethrtv/tethr/internal/dbtest"
)

func aired(daysAgo int) *time.Time {
	t := time.Now().AddDate(0, 0, -daysAgo).Truncate(24 * time.Hour)
	return &t
}

func TestUpNext(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	caller, _, _ := connectedProfile(t, db)
	stop := func(season, number int32) ScrobbleResult {
		t.Helper()
		r, err := ScrobbleStop(ctx, db, caller, Scrobble{Ref: episode(season, number), Progress: 100})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	next := func() []UpNextEntry {
		t.Helper()
		entries, err := UpNext(ctx, db, caller.ProfileID)
		if err != nil {
			t.Fatal(err)
		}
		return entries
	}

	first := stop(1, 1)
	stop(1, 2)
	show := first.Item.Title.ID
	if got := next(); len(got) != 0 {
		t.Errorf("up next before the outline = %+v", got)
	}

	due, err := DueOutlines(ctx, db, 10)
	if err != nil || len(due) != 1 || due[0].ID != show {
		t.Fatalf("due = %+v, %v", due, err)
	}
	err = SaveOutline(ctx, db, show, []OutlineEpisode{
		{TMDBID: 900, Season: 0, Number: 1, FirstAired: aired(400)},
		{TMDBID: 101, Season: 1, Number: 1, FirstAired: aired(300)},
		{TMDBID: 102, Season: 1, Number: 2, FirstAired: aired(293)},
		{TMDBID: 103, Season: 1, Number: 3, FirstAired: aired(286)},
		{TMDBID: 201, Season: 2, Number: 1, FirstAired: aired(10)},
		{TMDBID: 202, Season: 2, Number: 2, FirstAired: aired(-7)},
		{TMDBID: 203, Season: 2, Number: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	if due, _ := DueOutlines(ctx, db, 10); len(due) != 0 {
		t.Errorf("still due after saving = %+v", due)
	}

	got := next()
	if len(got) != 1 || got[0].Next.SeasonNumber != 1 || got[0].Next.EpisodeNumber != 3 || got[0].Aired != 4 || got[0].Watched != 2 {
		t.Fatalf("up next = %+v", got)
	}
	if got[0].Show.Title.ID != show || got[0].Next.TmdbEpisodeID == nil || *got[0].Next.TmdbEpisodeID != 103 {
		t.Errorf("entry = %+v", got[0])
	}

	if _, err := ScrobblePause(ctx, db, caller, Scrobble{Ref: episode(1, 3), Progress: 30}); err != nil {
		t.Fatal(err)
	}
	if _, err := ScrobbleStart(ctx, db, caller, Scrobble{Ref: episode(1, 3), Progress: 30}); err != nil {
		t.Fatal(err)
	}
	if got := next(); got[0].Resume == nil || got[0].Resume.Progress != 30 {
		t.Errorf("resume point lost when playback started again: %+v", got[0].Resume)
	}

	stop(1, 3)
	if got := next(); got[0].Next.SeasonNumber != 2 || got[0].Next.EpisodeNumber != 1 || got[0].Resume != nil {
		t.Errorf("after S1E3 = %+v", got[0])
	}
	stop(2, 1)
	if got := next(); len(got) != 0 {
		t.Errorf("caught up but still listed: %+v", got)
	}

	if _, err := RemoveHistory(ctx, db, caller.ProfileID, []int64{first.ID}, nil); err != nil {
		t.Fatal(err)
	}
	if got := next(); len(got) != 1 || got[0].Next.SeasonNumber != 1 || got[0].Next.EpisodeNumber != 1 {
		t.Errorf("gap = %+v", got)
	}

	if err := HideFromUpNext(ctx, db, caller.ProfileID, show); err != nil {
		t.Fatal(err)
	}
	if got := next(); len(got) != 0 {
		t.Errorf("hidden show listed: %+v", got)
	}
	if err := ShowInUpNext(ctx, db, caller.ProfileID, show); err != nil {
		t.Fatal(err)
	}
	if got := next(); len(got) != 1 {
		t.Errorf("unhidden show missing: %+v", got)
	}
}

func TestOutlineRenumbering(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	caller, _, _ := connectedProfile(t, db)

	first, err := ScrobbleStop(ctx, db, caller, Scrobble{Ref: episode(1, 1), Progress: 100})
	if err != nil {
		t.Fatal(err)
	}
	show := first.Item.Title.ID
	if err := SaveOutline(ctx, db, show, []OutlineEpisode{
		{TMDBID: 101, Season: 1, Number: 1, FirstAired: aired(30)},
		{TMDBID: 102, Season: 1, Number: 2, FirstAired: aired(20)},
	}); err != nil {
		t.Fatal(err)
	}
	lazy, err := ScrobbleStop(ctx, db, caller, Scrobble{Ref: episode(1, 3), Progress: 100})
	if err != nil {
		t.Fatal(err)
	}
	if lazy.Item.Episode.TmdbEpisodeID != nil {
		t.Fatalf("S1E3 shouldn't have a TMDB ID yet: %+v", lazy.Item.Episode)
	}

	if err := SaveOutline(ctx, db, show, []OutlineEpisode{
		{TMDBID: 101, Season: 1, Number: 1, FirstAired: aired(30)},
		{TMDBID: 102, Season: 1, Number: 3, FirstAired: aired(20)},
		{TMDBID: 104, Season: 1, Number: 4, FirstAired: aired(10)},
	}); err != nil {
		t.Fatal(err)
	}

	history, total, err := History(ctx, db, caller.ProfileID, HistoryFilter{}, Page{Page: 1, Limit: 10})
	if err != nil || total != 2 {
		t.Fatalf("history = %d, %v", total, err)
	}
	moved := history[0].Item.Episode
	if moved.ID == lazy.Item.Episode.ID || moved.TmdbEpisodeID == nil || *moved.TmdbEpisodeID != 102 || moved.EpisodeNumber != 3 {
		t.Errorf("the S1E3 watch should now point at TMDB episode 102: %+v", moved)
	}
	got, err := UpNext(ctx, db, caller.ProfileID)
	if err != nil || len(got) != 1 || got[0].Next.EpisodeNumber != 4 {
		t.Errorf("up next = %+v, %v", got, err)
	}
	if again, err := ScrobbleStop(ctx, db, caller, Scrobble{Ref: episode(1, 2), Progress: 100}); err != nil || again.Item.Episode.TmdbEpisodeID != nil {
		t.Errorf("S1E2 is gone from TMDB, so a new scrobble makes a fresh episode: %+v, %v", again.Item.Episode, err)
	}
}
