package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tethrtv/tethr/internal/dbtest"
)

func TestYear(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	caller, _, _ := connectedProfile(t, db)
	profile := caller.ProfileID
	london, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatal(err)
	}
	when := func(s string) time.Time {
		t.Helper()
		at, err := time.ParseInLocation("2006-01-02 15:04", s, london)
		if err != nil {
			t.Fatal(err)
		}
		return at
	}
	if _, err := AddHistory(ctx, db, caller, []Watch{
		{Ref: episode(1, 1), At: when("2024-12-30 21:00")},
		{Ref: episode(1, 1), At: when("2025-03-01 20:00")},
		{Ref: episode(1, 2), At: when("2025-03-01 21:00")},
		{Ref: episode(1, 3), At: when("2025-03-02 22:30")},
		{Ref: episode(1, 4), At: when("2025-03-03 00:30")},
		{Ref: inception(), At: when("2025-07-04 19:00")},
		{Ref: inception(), At: when("2025-12-31 23:30")},
		{Ref: episode(2, 1), At: when("2026-01-01 00:30")},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE titles SET runtime_minutes = CASE kind WHEN 'show' THEN 47 ELSE 90 END`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE watch_events SET duration_seconds = 3000 WHERE watched_at = $1`, when("2025-03-02 22:30")); err != nil {
		t.Fatal(err)
	}
	if _, err := AddHistory(ctx, db, caller, []Watch{{Ref: episode(1, 5), At: when("2025-05-05 12:00")}, {Ref: episode(1, 6), At: when("2025-05-06 12:00")}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE watch_events SET precision = 'day', watched_at = '2025-05-05 12:00Z' WHERE watched_at = $1`, when("2025-05-05 12:00")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE watch_events SET precision = 'none', watched_at = $1 WHERE watched_at = $2`, UnknownDate, when("2025-05-06 12:00")); err != nil {
		t.Fatal(err)
	}

	y, err := Year(ctx, db, profile, 2025, london, when("2026-02-01 12:00"))
	if err != nil {
		t.Fatal(err)
	}
	if y.Plays != 7 || y.Episodes != 5 || y.Films != 2 || y.Rewatched != 2 || y.NewShows != 0 {
		t.Errorf("counts = %d plays, %d episodes, %d films, %d again, %d new", y.Plays, y.Episodes, y.Films, y.Rewatched, y.NewShows)
	}
	if y.Minutes != 4*47+50+2*90 {
		t.Errorf("minutes = %d", y.Minutes)
	}
	if len(y.Days) != 365 || y.Days[59].Plays != 2 || y.Days[60].Plays != 1 || y.Days[61].Plays != 1 || y.Days[124].Plays != 1 || y.Days[364].Plays != 1 {
		t.Errorf("days = %d long, %+v", len(y.Days), y.Days[59:62])
	}
	if y.Hours[0] != 1 || y.Hours[20] != 1 || y.Hours[22] != 1 || y.Hours[12] != 0 || y.Hours[23] != 1 {
		t.Errorf("hours = %v", y.Hours)
	}
	if y.Run.Days != 3 || !y.Run.From.Equal(time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("run = %+v", y.Run)
	}
	if y.Busiest == nil || y.Busiest.Plays != 2 || y.Busiest.Minutes != 94 || y.Busiest.Title.Kind() != "show" || y.Busiest.TitlePlays != 2 {
		t.Errorf("busiest = %+v", y.Busiest)
	}
	if len(y.Shows) != 1 || y.Shows[0].Episodes != 5 || y.Shows[0].New || y.Shows[0].Show.Title.TmdbID == nil || *y.Shows[0].Show.Title.TmdbID != 1396 {
		t.Errorf("shows = %+v", y.Shows)
	}
	if len(y.FilmsSeen) != 2 || !y.FilmsSeen[0].New || !y.FilmsSeen[1].Again {
		t.Errorf("films = %+v", y.FilmsSeen)
	}
	if y.First == nil || y.First.Item.Episode == nil || y.First.Item.Episode.Number != 1 || y.Latest == nil || y.Latest.Item.Kind() != "movie" {
		t.Errorf("first and latest = %+v, %+v", y.First, y.Latest)
	}
	if len(y.Years) != 3 || y.Years[0] != 2026 {
		t.Errorf("years = %v", y.Years)
	}

	now, err := Year(ctx, db, profile, 2026, london, when("2026-02-01 12:00"))
	if err != nil || len(now.Days) != 32 || now.Plays != 1 || now.NewShows != 0 {
		t.Errorf("year so far = %d days, %d plays, %v", len(now.Days), now.Plays, err)
	}
	if _, err := Year(ctx, db, profile, 2027, london, when("2026-02-01 12:00")); !errors.Is(err, ErrNotFound) {
		t.Errorf("next year = %v", err)
	}
	empty, err := Year(ctx, db, profile, 2020, time.UTC, when("2026-02-01 12:00"))
	if err != nil || empty.Plays != 0 || len(empty.Days) != 366 || empty.Busiest != nil || empty.First != nil {
		t.Errorf("a year with nothing = %+v, %v", empty, err)
	}
}
