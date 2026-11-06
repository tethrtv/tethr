package core

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/tethrtv/tethr/internal/dbtest"
	"github.com/tethrtv/tethr/internal/store"
)

func TestShowProgress(t *testing.T) {
	now := time.Date(2026, 11, 5, 20, 0, 0, 0, time.UTC)
	day := func(n int) *time.Time {
		d := time.Date(2026, 11, 5-n, 0, 0, 0, 0, time.UTC)
		return &d
	}
	outline := []OrderedEpisode{
		{Episode: store.Episode{ID: 1, SeasonNumber: 0, EpisodeNumber: 1, FirstAired: day(400)}, Season: 0, Number: 1},
		{Episode: store.Episode{ID: 2, SeasonNumber: 1, EpisodeNumber: 1, FirstAired: day(300)}, Season: 1, Number: 1},
		{Episode: store.Episode{ID: 3, SeasonNumber: 1, EpisodeNumber: 2, FirstAired: day(293)}, Season: 1, Number: 2},
		{Episode: store.Episode{ID: 4, SeasonNumber: 1, EpisodeNumber: 3, FirstAired: day(286)}, Season: 1, Number: 3},
		{Episode: store.Episode{ID: 5, SeasonNumber: 2, EpisodeNumber: 1, FirstAired: day(20)}, Season: 2, Number: 1},
		{Episode: store.Episode{ID: 6, SeasonNumber: 2, EpisodeNumber: 2, FirstAired: day(0)}, Season: 2, Number: 2},
		{Episode: store.Episode{ID: 7, SeasonNumber: 2, EpisodeNumber: 3, FirstAired: day(-7)}, Season: 2, Number: 3},
		{Episode: store.Episode{ID: 8, SeasonNumber: 2, EpisodeNumber: 4}, Season: 2, Number: 4},
	}
	dvd := []OrderedEpisode{
		{Episode: outline[1].Episode, Season: 1, Number: 1},
		{Episode: outline[2].Episode, Season: 1, Number: 2},
		{Episode: outline[3].Episode, Season: 1, Number: 3},
		{Episode: outline[0].Episode, Season: 1, Number: 4},
		{Episode: outline[4].Episode, Season: 2, Number: 1},
	}
	at := func(hour int) time.Time { return now.Add(time.Duration(hour-100) * time.Hour) }

	tests := []struct {
		name     string
		episodes []OrderedEpisode
		watched  map[int64]time.Time
		hidden   []int32
		specials bool
		aired    int
		done     int
		next     string
		last     string
	}{
		{name: "nothing watched", watched: nil, aired: 5, next: "1x1"},
		{name: "next follows the last watch", watched: map[int64]time.Time{2: at(1), 3: at(2)}, aired: 5, done: 2, next: "1x3", last: "1x2"},
		{name: "a rewatched early episode doesn't move progress back", watched: map[int64]time.Time{2: at(3), 3: at(1), 4: at(2)}, aired: 5, done: 3, next: "2x1", last: "1x1"},
		{name: "a gap is next once the last aired episode is watched", watched: map[int64]time.Time{2: at(1), 5: at(2), 6: at(3)}, aired: 5, done: 3, next: "1x2", last: "2x2"},
		{name: "caught up", watched: map[int64]time.Time{2: at(1), 3: at(1), 4: at(1), 5: at(1), 6: at(1)}, aired: 5, done: 5, last: "2x2"},
		{name: "the same watch time picks the later episode", watched: map[int64]time.Time{2: at(1), 3: at(1)}, aired: 5, done: 2, next: "1x3", last: "1x2"},
		{name: "unaired episodes don't count", watched: map[int64]time.Time{7: at(1)}, aired: 5, next: "1x1", last: "2x3"},
		{name: "a special is never next", watched: map[int64]time.Time{1: at(1)}, aired: 5, next: "1x1", last: "0x1"},
		{name: "specials count when asked for", watched: map[int64]time.Time{1: at(1), 2: at(2)}, specials: true, aired: 6, done: 2, next: "1x2", last: "1x1"},
		{name: "hidden seasons don't count", watched: map[int64]time.Time{2: at(1), 3: at(2), 4: at(3)}, hidden: []int32{1}, aired: 2, next: "2x1", last: "1x3"},
		{name: "next follows the order given", episodes: []OrderedEpisode{outline[3], outline[1], outline[2]}, watched: map[int64]time.Time{4: at(1)}, aired: 3, done: 1, next: "1x1", last: "1x3"},
		{name: "seasons are the order's", episodes: dvd, watched: map[int64]time.Time{2: at(1), 3: at(1), 4: at(1)}, hidden: []int32{2}, aired: 4, done: 3, next: "1x4", last: "1x3"},
		{name: "a special the order places in a season counts", episodes: dvd, watched: map[int64]time.Time{2: at(1)}, aired: 5, done: 1, next: "1x2", last: "1x1"},
	}
	name := func(e *OrderedEpisode) string {
		if e == nil {
			return ""
		}
		return fmt.Sprintf("%dx%d", e.Season, e.Number)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			episodes := tt.episodes
			if episodes == nil {
				episodes = outline
			}
			p := showProgress(episodes, tt.watched, tt.hidden, tt.specials, now)
			if p.Aired != tt.aired || p.Completed != tt.done || name(p.Next) != tt.next || name(p.Last) != tt.last {
				t.Errorf("aired %d, done %d, next %q, last %q; want %d, %d, %q, %q", p.Aired, p.Completed, name(p.Next), name(p.Last), tt.aired, tt.done, tt.next, tt.last)
			}
			if p.Last != nil && !p.LastWatchedAt.Equal(tt.watched[p.Last.ID]) {
				t.Errorf("last watched at %v", p.LastWatchedAt)
			}
		})
	}
}

func TestProgress(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	caller, _, _ := connectedProfile(t, db)
	profile := caller.ProfileID

	first, err := ScrobbleStop(ctx, db, caller, Scrobble{Ref: episode(1, 1), Progress: 100})
	if err != nil {
		t.Fatal(err)
	}
	show := first.Item.Title.ID
	if _, err := AddHistory(ctx, db, caller, []Watch{{Ref: episode(1, 1), At: *aired(5)}, {Ref: episode(2, 1), At: *aired(1)}, {Ref: inception(), At: *aired(2)}}); err != nil {
		t.Fatal(err)
	}
	err = SaveOutline(ctx, db, show, []OutlineEpisode{
		{TMDBID: 900, Season: 0, Number: 1, FirstAired: aired(400)},
		{TMDBID: 101, Season: 1, Number: 1, FirstAired: aired(300)},
		{TMDBID: 102, Season: 1, Number: 2, FirstAired: aired(293)},
		{TMDBID: 201, Season: 2, Number: 1, FirstAired: aired(20)},
		{TMDBID: 202, Season: 2, Number: 2, FirstAired: aired(13)},
		{TMDBID: 203, Season: 2, Number: 3, FirstAired: aired(-7)},
	})
	if err != nil {
		t.Fatal(err)
	}

	p, err := Progress(ctx, db, profile, Ref{Kind: "show", IDs: IDs{TMDB: 1396}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if p.Aired != 4 || p.Completed != 2 || len(p.Seasons) != 2 || p.Seasons[0].Completed != 1 || p.Seasons[1].Aired != 2 {
		t.Fatalf("progress = %+v", p)
	}
	if p.Next == nil || p.Next.SeasonNumber != 1 || p.Next.EpisodeNumber != 2 {
		t.Errorf("next = %+v, want the episode after the latest watch", p.Next)
	}
	if p.Last == nil || p.Last.SeasonNumber != 1 || p.Last.EpisodeNumber != 1 {
		t.Errorf("last = %+v, the S1E1 scrobble is the latest watch", p.Last)
	}
	withSpecials, _ := Progress(ctx, db, profile, Ref{Kind: "show", IDs: IDs{TMDB: 1396}}, true)
	if withSpecials.Aired != 5 || withSpecials.Seasons[0].Number != 0 {
		t.Errorf("with specials = %+v", withSpecials)
	}

	if _, err := Hide(ctx, db, profile, "progress_watched", []Ref{{Kind: "season", Show: IDs{TMDB: 1396}, Season: ptr(int32(2))}}); err != nil {
		t.Fatal(err)
	}
	p, _ = Progress(ctx, db, profile, Ref{Kind: "show", IDs: IDs{TMDB: 1396}}, false)
	if len(p.HiddenSeasons) != 1 || len(p.Seasons) != 1 || p.Aired != 2 {
		t.Errorf("with season 2 hidden = %+v", p)
	}
	result, err := Unhide(ctx, db, profile, "progress_watched", []Ref{{Kind: "season", Show: IDs{TMDB: 1396}, Season: ptr(int32(2))}, inception()})
	if err != nil || result.Deleted.Seasons != 1 || len(result.NotFound) != 1 {
		t.Errorf("unhide = %+v, %v", result, err)
	}

	movies, err := Watched(ctx, db, profile, "movie")
	if err != nil || len(movies) != 1 || movies[0].Plays.Plays != 1 {
		t.Errorf("watched movies = %+v, %v", movies, err)
	}
	shows, err := Watched(ctx, db, profile, "show")
	if err != nil || len(shows) != 1 || shows[0].Plays.Plays != 3 || len(shows[0].Episodes) != 2 || shows[0].Episodes[0].Plays.Plays != 2 {
		t.Errorf("watched shows = %+v, %v", shows, err)
	}
}

func TestRatings(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	caller, _, _ := connectedProfile(t, db)
	profile := caller.ProfileID

	result, err := Rate(ctx, db, profile, []Rating{{Ref: inception(), Rating: 7}, {Ref: episode(1, 1), Rating: 10}, {Ref: inception(), Rating: 11}})
	if err != nil || result.Added.Movies != 1 || result.Added.Episodes != 1 || len(result.NotFound) != 1 {
		t.Fatalf("rate = %+v, %v", result, err)
	}
	if result, err := Rate(ctx, db, profile, []Rating{{Ref: inception(), Rating: 7}}); err != nil || result.Existing.Movies != 1 || result.Added.Movies != 0 {
		t.Errorf("same rating again = %+v, %v", result, err)
	}
	if result, err := Rate(ctx, db, profile, []Rating{{Ref: inception(), Rating: 8}}); err != nil || result.Added.Movies != 1 {
		t.Fatalf("new rating = %+v, %v", result, err)
	}
	movies, _ := Ratings(ctx, db, profile, "movie", nil)
	if len(movies) != 1 || movies[0].Rating != 8 {
		t.Errorf("re-rating should replace: %+v", movies)
	}
	if tens, _ := Ratings(ctx, db, profile, "", []int16{10}); len(tens) != 1 || tens[0].Item.Kind() != "episode" {
		t.Errorf("filtered = %+v", tens)
	}
	if result, err := Unrate(ctx, db, profile, []Ref{inception()}); err != nil || result.Deleted.Movies != 1 {
		t.Errorf("unrate = %+v, %v", result, err)
	}
	all, _ := Ratings(ctx, db, profile, "", nil)
	if err := RemoveRating(ctx, db, profile, all[0].ID); err != nil {
		t.Fatal(err)
	}
	if left, _ := Ratings(ctx, db, profile, "", nil); len(left) != 0 {
		t.Errorf("left = %+v", left)
	}
}

func ptr[T any](v T) *T { return &v }
