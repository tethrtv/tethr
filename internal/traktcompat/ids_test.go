package traktcompat

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/dbtest"
)

func traktOf(item map[string]any, key string) int64 {
	return int64(item[key].(map[string]any)["ids"].(map[string]any)["trakt"].(float64))
}

func idString(n int64) string { return strconv.FormatInt(n, 10) }

func TestCatalogueIDs(t *testing.T) {
	db := dbtest.New(t)
	c, _ := signIn(t, db)

	paused := c.valid("POST", "/scrobble/pause", map[string]any{"progress": 30.0, "movie": map[string]any{"ids": map[string]any{"tmdb": 603}}}, http.StatusCreated)
	movie := traktOf(paused, "movie")
	if movie != 1_000_000_000 {
		t.Errorf("Tethr's first title should go out as a billion: %d", movie)
	}
	found := c.list("GET", "/search/trakt/"+idString(movie)+"?type=movie", nil)
	if len(found) != 1 || traktOf(found[0], "movie") != movie {
		t.Errorf("search by the ID that went out = %v", found)
	}
	if found := c.list("GET", "/search/trakt/"+idString(movie-catalogueOffset)+"?type=movie", nil); len(found) != 0 {
		t.Errorf("a number below the offset found %v", found)
	}

	stopped := c.valid("POST", "/scrobble/stop", map[string]any{"progress": 100.0, "show": map[string]any{"ids": map[string]any{"tmdb": 1396}}, "episode": map[string]any{"season": 1, "number": 1}}, http.StatusCreated)
	show, episode := traktOf(stopped, "show"), traktOf(stopped, "episode")
	found = c.list("GET", "/search/trakt/"+idString(episode)+"?type=episode", nil)
	if len(found) != 1 || traktOf(found[0], "episode") != episode || traktOf(found[0], "show") != show {
		t.Errorf("search by the episode ID that went out = %v", found)
	}
	if found := c.list("GET", "/search/trakt/"+idString(episode-catalogueOffset)+"?type=episode", nil); len(found) != 0 {
		t.Errorf("an episode number below the offset found %v", found)
	}

	if rec := c.do("GET", "/shows/"+idString(show)+"/progress/watched", nil); rec.Code != http.StatusOK {
		t.Errorf("progress by the show ID that went out = %d", rec.Code)
	}
	if rec := c.do("GET", "/shows/"+idString(show-catalogueOffset)+"/progress/watched", nil); rec.Code != http.StatusNotFound {
		t.Errorf("progress by a number below the offset = %d", rec.Code)
	}

	added := c.valid("POST", "/sync/history", map[string]any{
		"movies":   []any{map[string]any{"watched_at": "2024-01-01T20:00:00.000Z", "ids": map[string]any{"trakt": movie}}},
		"episodes": []any{map[string]any{"watched_at": "2024-01-02T20:00:00.000Z", "ids": map[string]any{"trakt": episode}}, map[string]any{"ids": map[string]any{"trakt": 1388}}},
		"shows":    []any{map[string]any{"ids": map[string]any{"trakt": 1390}, "seasons": []any{map[string]any{"number": 1, "episodes": []any{map[string]any{"number": 1}}}}}},
	}, http.StatusCreated)
	if a := added["added"].(map[string]any); a["movies"] != 1.0 || a["episodes"] != 1.0 {
		t.Errorf("added by the IDs that went out = %v", added)
	}
	missing := added["not_found"].(map[string]any)
	if eps := missing["episodes"].([]any); len(eps) != 1 || eps[0].(map[string]any)["ids"].(map[string]any)["trakt"] != 1388.0 {
		t.Errorf("an episode number below the offset should come back as sent: %v", missing)
	}
	if shows := missing["shows"].([]any); len(shows) != 1 || shows[0].(map[string]any)["ids"].(map[string]any)["trakt"] != 1390.0 {
		t.Errorf("a show number below the offset should come back as sent: %v", missing)
	}

	if got := c.list("GET", "/sync/history/episodes/"+idString(episode), nil); len(got) != 2 || traktOf(got[0], "episode") != episode {
		t.Errorf("history for the episode ID that went out = %v", got)
	}
	if got := c.list("GET", "/sync/history/episodes/"+idString(episode-catalogueOffset), nil); len(got) != 0 {
		t.Errorf("history for a number below the offset = %v", got)
	}
	if got := c.list("GET", "/sync/history/episodes/breaking-bad", nil); len(got) != 0 {
		t.Errorf("history for an ID that isn't a number = %v", got)
	}
}

func TestEntryIDs(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	c, _ := signIn(t, db)
	var profile int64
	if err := db.QueryRow(ctx, "SELECT id FROM profiles").Scan(&profile); err != nil {
		t.Fatal(err)
	}

	paused := c.valid("POST", "/scrobble/pause", map[string]any{"progress": 30.0, "movie": map[string]any{"ids": map[string]any{"tmdb": 603}}}, http.StatusCreated)
	playback := int64(paused["id"].(float64))
	if playback != 1_000_000_000_000 {
		t.Errorf("Tethr's first playback should go out as a trillion: %d", playback)
	}
	if listed := c.list("GET", "/sync/playback", nil); len(listed) != 1 || int64(listed[0]["id"].(float64)) != playback {
		t.Errorf("playback = %v", listed)
	}
	if rec := c.do("DELETE", "/sync/playback/"+idString(playback-entryOffset), nil); rec.Code != http.StatusNotFound {
		t.Errorf("removing playback by a number below the offset = %d", rec.Code)
	}
	if rec := c.do("DELETE", "/sync/playback/"+idString(playback), nil); rec.Code != http.StatusNoContent {
		t.Errorf("removing playback by the ID that went out = %d", rec.Code)
	}

	stopped := c.valid("POST", "/scrobble/stop", map[string]any{"progress": 100.0, "movie": map[string]any{"ids": map[string]any{"tmdb": 603}}}, http.StatusCreated)
	event := int64(stopped["id"].(float64))
	if history := c.list("GET", "/sync/history", nil); len(history) != 1 || int64(history[0]["id"].(float64)) != event {
		t.Errorf("history = %v, want id %d", history, event)
	}
	removed := c.valid("POST", "/sync/history/remove", map[string]any{"ids": []any{event, event - entryOffset}}, http.StatusOK)
	missing := removed["not_found"].(map[string]any)["ids"].([]any)
	if removed["deleted"].(map[string]any)["movies"] != 1.0 || len(missing) != 1 || missing[0] != float64(event-entryOffset) {
		t.Errorf("removed = %v", removed)
	}

	c.valid("POST", "/sync/watchlist", map[string]any{"shows": []any{map[string]any{"ids": map[string]any{"tmdb": 1396}}}}, http.StatusCreated)
	mine, err := core.Watchlist(ctx, db, profile, "")
	if err != nil || len(mine) != 1 {
		t.Fatal(mine, err)
	}
	if listed := c.list("GET", "/sync/watchlist", nil); len(listed) != 1 || int64(listed[0]["id"].(float64)) != mine[0].ID+entryOffset {
		t.Errorf("watchlist = %v, want id %d", listed, mine[0].ID+entryOffset)
	}
}

func TestListIDs(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	c, _ := signIn(t, db)
	var profile int64
	if err := db.QueryRow(ctx, "SELECT id FROM profiles").Scan(&profile); err != nil {
		t.Fatal(err)
	}
	name := "Sunday films"
	l, err := core.CreateList(ctx, db, profile, core.ListChanges{Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := core.AddToList(ctx, db, profile, l.ID, []core.Listing{{Ref: core.Ref{Kind: "movie", IDs: core.IDs{TMDB: 603}}}}); err != nil {
		t.Fatal(err)
	}
	_, entries, _, err := core.ListEntries(ctx, db, profile, l.ID, core.ListQuery{})
	if err != nil || len(entries) != 1 {
		t.Fatal(entries, err)
	}

	mine := c.list("GET", "/users/me/lists", nil)
	if len(mine) != 1 {
		t.Fatalf("lists = %v", mine)
	}
	id := int64(mine[0]["ids"].(map[string]any)["trakt"].(float64))
	if id != 1_000_000_000 {
		t.Errorf("Tethr's first list should go out as a billion: %d", id)
	}
	var got map[string]any
	if rec := c.do("GET", "/lists/"+idString(id), nil); rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil || got["name"] != name {
		t.Errorf("list by the ID that went out = %d %s", rec.Code, rec.Body)
	}
	var items []map[string]any
	rec := c.do("GET", "/users/me/lists/"+idString(id)+"/items", nil)
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil || len(items) != 1 || int64(items[0]["id"].(float64)) != entries[0].ID+entryOffset {
		t.Errorf("items = %s, want id %d", rec.Body, entries[0].ID+entryOffset)
	}
	for _, path := range []string{"/lists/" + idString(l.ID), "/users/me/lists/" + idString(l.ID) + "/items"} {
		if rec := c.do("GET", path, nil); rec.Code != http.StatusNotFound {
			t.Errorf("%s with a number below the offset = %d", path, rec.Code)
		}
	}
}
