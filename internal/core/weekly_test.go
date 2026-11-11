package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tethrtv/tethr/internal/dbtest"
)

func nextSunday(now time.Time) time.Time {
	d := time.Date(now.Year(), now.Month(), now.Day(), 18, 30, 0, 0, time.UTC)
	for d.Weekday() != time.Sunday || d.Before(now) {
		d = d.AddDate(0, 0, 1)
	}
	return d
}

func TestWeekAhead(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	caller, _, _ := connectedProfile(t, db)
	profile := caller.ProfileID
	now := nextSunday(time.Now())
	day := func(n int) *time.Time {
		d := time.Date(now.Year(), now.Month(), now.Day()+n, 0, 0, 0, 0, time.UTC)
		return &d
	}

	season, number := int32(1), int32(1)
	watched, err := ScrobbleStop(ctx, db, caller, Scrobble{Ref: Ref{Kind: "episode", Show: IDs{TMDB: 1396}, Season: &season, Number: &number}, Progress: 100})
	if err != nil {
		t.Fatal(err)
	}
	show := watched.Item.Title.ID
	if err := SaveOutline(ctx, db, show, []OutlineEpisode{
		{TMDBID: 101, Season: 1, Number: 1, FirstAired: day(-30)},
		{TMDBID: 102, Season: 1, Number: 2, FirstAired: day(0)},
		{TMDBID: 103, Season: 1, Number: 3, FirstAired: day(1)},
		{TMDBID: 104, Season: 1, Number: 4, FirstAired: day(7)},
		{TMDBID: 105, Season: 1, Number: 5, FirstAired: day(8)},
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := AddToWatchlist(ctx, db, profile, []Listing{{Ref: Ref{Kind: "show", IDs: IDs{TMDB: 2000}}}}); err != nil {
		t.Fatal(err)
	}
	list, _ := Watchlist(ctx, db, profile, "")
	if err := SaveOutline(ctx, db, list[0].Item.Title.ID, []OutlineEpisode{{TMDBID: 401, Season: 1, Number: 1, FirstAired: day(3)}}); err != nil {
		t.Fatal(err)
	}

	_, owner := household(t, db, "curator")
	followed, err := CreateList(ctx, db, owner, ListChanges{Name: ptr("Slow films"), Privacy: ptr("public")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddToList(ctx, db, owner, followed.ID, []Listing{{Ref: inception()}}); err != nil {
		t.Fatal(err)
	}
	if _, err := FollowList(ctx, db, profile, followed.ID); err != nil {
		t.Fatal(err)
	}

	london, _ := time.LoadLocation("Europe/London")
	w, err := WeekAhead(ctx, db, profile, now, london)
	if err != nil {
		t.Fatal(err)
	}
	if !w.From.Equal(*day(1)) {
		t.Errorf("week starts %v, want the Monday after %v", w.From, now)
	}
	var episodes []int32
	for _, e := range w.Episodes {
		episodes = append(episodes, e.Episode.Number)
	}
	if len(episodes) != 2 || episodes[0] != 3 || episodes[1] != 4 {
		t.Errorf("episodes this week = %v, want 3 and 4", episodes)
	}
	if len(w.Premieres) != 1 || w.Premieres[0].Episode.FirstAired.Weekday() != time.Wednesday {
		t.Errorf("premieres = %+v", w.Premieres)
	}
	if len(w.Lists) != 1 || w.Lists[0].List.ID != followed.ID || len(w.Lists[0].Added) != 1 {
		t.Errorf("lists = %+v", w.Lists)
	}
	if w.Watched.Episodes != 1 || w.Watched.Films != 0 {
		t.Errorf("watched = %+v", w.Watched)
	}
	if w.Empty() {
		t.Error("a week with episodes counts as empty")
	}
}

func TestAccountSettings(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	accountID, _ := household(t, db, "nathan")

	a, err := UpdateAccount(ctx, db, accountID, AccountChanges{TimeZone: ptr("America/New_York")})
	if err != nil || a.TimeZone == nil || *a.TimeZone != "America/New_York" || !a.WeeklyEmail {
		t.Fatalf("setting the zone = %+v, %v", a, err)
	}
	for _, bad := range []string{"", "Local", "Mars/Olympus"} {
		if _, err := UpdateAccount(ctx, db, accountID, AccountChanges{TimeZone: &bad}); !errors.Is(err, ErrInvalidTimeZone) {
			t.Errorf("zone %q = %v", bad, err)
		}
	}

	for _, bad := range []string{"not a token", "00000000-0000-0000-0000-000000000000"} {
		if err := Unsubscribe(ctx, db, bad); !errors.Is(err, ErrNotFound) {
			t.Errorf("token %q = %v", bad, err)
		}
	}
	if err := Unsubscribe(ctx, db, a.UnsubscribeToken.String()); err != nil {
		t.Fatal(err)
	}
	a, _, err = Me(ctx, db, accountID)
	if err != nil || a.WeeklyEmail || *a.TimeZone != "America/New_York" {
		t.Errorf("after unsubscribing = %+v, %v", a, err)
	}
}
