package traktcompat

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/dbtest"
)

func TestEpisodeOrderNumbers(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	c, _ := signIn(t, db)
	var profile int64
	if err := db.QueryRow(ctx, "SELECT id FROM profiles").Scan(&profile); err != nil {
		t.Fatal(err)
	}

	show := map[string]any{"ids": map[string]any{"tmdb": 1396}}

	first := c.valid("POST", "/scrobble/stop", map[string]any{"progress": 100, "show": show, "episode": map[string]any{"season": 1, "number": 1}}, http.StatusCreated)
	showID := tethrID(int64(first["show"].(map[string]any)["ids"].(map[string]any)["trakt"].(float64)), catalogueOffset)
	aired := time.Now().AddDate(-1, 0, 0)
	if err := core.SaveOutline(ctx, db, showID, []core.OutlineEpisode{
		{TMDBID: 900, Season: 0, Number: 1, FirstAired: &aired},
		{TMDBID: 101, Season: 1, Number: 1, FirstAired: &aired},
		{TMDBID: 102, Season: 1, Number: 2, FirstAired: &aired},
		{TMDBID: 103, Season: 1, Number: 3, FirstAired: &aired},
		{TMDBID: 104, Season: 1, Number: 4, FirstAired: &aired},
	}); err != nil {
		t.Fatal(err)
	}
	if err := core.PickEpisodeOrder(ctx, db, profile, "tmdb-1396", &core.EpisodeOrder{TMDBGroupID: "dvd", TMDBType: 3, Episodes: []core.OrderEntry{
		{TMDBID: 101, ShowTMDBID: 1396, Season: 1, Number: 1},
		{TMDBID: 900, ShowTMDBID: 1396, Season: 1, Number: 2},
		{TMDBID: 103, ShowTMDBID: 1396, Season: 2, Number: 1},
		{TMDBID: 102, ShowTMDBID: 1396, Season: 2, Number: 2},
	}}); err != nil {
		t.Fatal(err)
	}

	numbers := func(item map[string]any) (int, int, int) {
		e := item["episode"].(map[string]any)
		return int(e["season"].(float64)), int(e["number"].(float64)), int(e["ids"].(map[string]any)["tmdb"].(float64))
	}

	stopped := c.valid("POST", "/scrobble/stop", map[string]any{"progress": 100, "show": show, "episode": map[string]any{"season": 2, "number": 1}}, http.StatusCreated)
	if s, n, id := numbers(stopped); s != 2 || n != 1 || id != 103 {
		t.Errorf("S2E1 on the dvd should be aired S1E3 and come back as S2E1: S%dE%d, tmdb %d", s, n, id)
	}

	byID := c.valid("POST", "/scrobble/stop", map[string]any{"progress": 100, "show": show, "episode": map[string]any{"season": 1, "number": 1, "ids": map[string]any{"tmdb": 102}}}, http.StatusCreated)
	if s, n, id := numbers(byID); s != 2 || n != 2 || id != 102 {
		t.Errorf("an episode ID should win over numbers: S%dE%d, tmdb %d", s, n, id)
	}

	if rec := c.do("POST", "/scrobble/stop", map[string]any{"progress": 100, "show": show, "episode": map[string]any{"season": 3, "number": 1}}); rec.Code != http.StatusNotFound {
		t.Errorf("a number outside the order = %d: %s", rec.Code, rec.Body)
	}

	added := c.valid("POST", "/sync/history", map[string]any{"shows": []any{map[string]any{"ids": map[string]any{"tmdb": 1396}, "seasons": []any{
		map[string]any{"number": 1, "episodes": []any{map[string]any{"number": 2, "watched_at": "2026-01-01T20:00:00.000Z"}}},
	}}}}, http.StatusCreated)
	if added["added"].(map[string]any)["episodes"].(float64) != 1 {
		t.Errorf("history add = %v", added)
	}

	history := c.list("GET", "/sync/history/episodes?limit=10", nil)
	want := map[int]string{103: "S2E1", 102: "S2E2", 101: "S1E1", 900: "S1E2"}
	for _, h := range history {
		s, n, id := numbers(h)
		if got := fmt.Sprintf("S%dE%d", s, n); want[id] != got {
			t.Errorf("history tmdb %d = %s, want %s", id, got, want[id])
		}
	}
	if len(history) != 4 {
		t.Errorf("history has %d episodes", len(history))
	}

	watched := c.list("GET", "/sync/watched/shows", nil)
	seasons := watched[0]["seasons"].([]any)
	if len(seasons) != 2 || seasons[0].(map[string]any)["number"].(float64) != 1 || len(seasons[1].(map[string]any)["episodes"].([]any)) != 2 {
		t.Errorf("watched seasons should follow the dvd: %v", seasons)
	}
}
