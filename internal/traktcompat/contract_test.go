package traktcompat

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/dbtest"
)

type client struct {
	t      *testing.T
	server http.Handler
	router routers.Router
	key    string
	token  string
}

func newClient(t *testing.T, db *pgxpool.Pool, key string) *client {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromFile("../../spec/trakt/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	doc.Servers = openapi3.Servers{{URL: "https://api.tethr.tv"}}
	router, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	(&Server{DB: db, AppURL: "https://tethr.tv", Log: slog.New(slog.DiscardHandler)}).Routes(mux)
	return &client{t: t, server: mux, router: router, key: key}
}

func (c *client) do(method, path string, body any) *httptest.ResponseRecorder {
	c.t.Helper()
	var buf []byte
	if body != nil {
		buf, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(method, "https://api.tethr.tv"+path, bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("trakt-api-version", "2")
	req.Header.Set("trakt-api-key", c.key)
	req.Header.Set("User-Agent", "Zuno/2.3")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	rec := httptest.NewRecorder()
	c.server.ServeHTTP(rec, req)
	return rec
}

func (c *client) valid(method, path string, body any, want int) map[string]any {
	c.t.Helper()
	rec := c.do(method, path, body)
	if rec.Code != want {
		c.t.Fatalf("%s %s = %d, want %d: %s", method, path, rec.Code, want, rec.Body)
	}
	c.check(method, path, body, rec)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

func (c *client) list(method, path string, body any) []map[string]any {
	c.t.Helper()
	rec := c.do(method, path, body)
	if rec.Code != http.StatusOK {
		c.t.Fatalf("%s %s = %d: %s", method, path, rec.Code, rec.Body)
	}
	c.check(method, path, body, rec)
	var out []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	return out
}

func (c *client) check(method, path string, body any, rec *httptest.ResponseRecorder) {
	c.t.Helper()
	var buf []byte
	if body != nil {
		buf, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(method, "https://api.tethr.tv"+specPath(method, path), bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	route, params, err := c.router.FindRoute(req)
	if err != nil {
		c.t.Errorf("%s %s isn't in the Trakt snapshot", method, path)
		return
	}
	input := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{Request: req, PathParams: params, Route: route},
		Status:                 rec.Code,
		Header:                 rec.Header(),
		Body:                   io.NopCloser(bytes.NewReader(rec.Body.Bytes())),
		Options:                &openapi3filter.Options{IncludeResponseStatus: false, MultiError: true},
	}
	err = openapi3filter.ValidateResponse(context.Background(), input)
	// Trakt's spec lists the movie and episode scrobble answers as oneOf, but neither variant requires its item, so every answer matches both.
	if err != nil && !strings.Contains(err.Error(), "matches more than one oneOf schemas") {
		var problems []string
		for _, line := range strings.Split(err.Error(), "\n") {
			if strings.Contains(line, "Error at") {
				problems = append(problems, strings.TrimSpace(line))
			}
		}
		if problems == nil {
			problems = []string{strings.SplitN(err.Error(), "\n", 2)[0]}
		}
		c.t.Errorf("%s %s doesn't match Trakt's spec: %s", method, path, strings.Join(problems, "; "))
	}
}

// Trakt's document only spells out the longest form of these paths.
func specPath(method, path string) string {
	route, query, _ := strings.Cut(path, "?")
	if method == http.MethodGet {
		switch parts := strings.Split(strings.Trim(route, "/"), "/"); {
		case route == "/sync/history":
			route = "/sync/history/all/0"
		case len(parts) == 3 && parts[1] == "history":
			route += "/0"
		case parts[0] == "sync" && parts[1] == "watchlist" && len(parts) < 5:
			route = "/sync/watchlist/all/rank/asc"
		}
	}
	return route + "?" + query
}

func signIn(t *testing.T, db *pgxpool.Pool) (*client, core.App) {
	t.Helper()
	ctx := context.Background()
	account, profile, err := core.CreateAccount(ctx, db, core.SignUp{Email: "nathan@example.com", Name: "Nathan", Username: "nathan"})
	if err != nil {
		t.Fatal(err)
	}
	app, err := core.CreateApp(ctx, db, "Zuno", []string{"zuno://trakt/callback"})
	if err != nil {
		t.Fatal(err)
	}
	c := newClient(t, db, app.ClientID)

	device := c.valid("POST", "/oauth/device/code", map[string]string{"client_id": app.ClientID}, http.StatusOK)
	if device["verification_url"] != "https://tethr.tv/activate" || len(device["user_code"].(string)) != 8 {
		t.Errorf("device code = %v", device)
	}
	pending := c.do("POST", "/oauth/device/token", map[string]string{"code": device["device_code"].(string), "client_id": app.ClientID, "client_secret": app.ClientSecret})
	if pending.Code != http.StatusBadRequest {
		t.Errorf("pending poll = %d", pending.Code)
	}
	if err := core.ApproveDevice(ctx, db, account.ID, device["user_code"].(string), profile.ID); err != nil {
		t.Fatal(err)
	}
	tokens := c.valid("POST", "/oauth/device/token", map[string]string{"code": device["device_code"].(string), "client_id": app.ClientID, "client_secret": app.ClientSecret}, http.StatusOK)
	for _, key := range []string{"access_token", "token_type", "expires_in", "refresh_token", "scope", "created_at"} {
		if tokens[key] == nil {
			t.Errorf("token response is missing %s", key)
		}
	}
	c.token = tokens["access_token"].(string)
	return c, app
}

func TestZunoFlow(t *testing.T) {
	db := dbtest.New(t)
	c, app := signIn(t, db)

	settings := c.valid("GET", "/users/settings", nil, http.StatusOK)
	user := settings["user"].(map[string]any)
	if user["username"] != "nathan" || user["private"] != true {
		t.Errorf("settings user = %v", user)
	}
	if len(settings["connections"].(map[string]any)) != 10 || settings["sharing_text"] == nil {
		t.Errorf("settings = %v", settings)
	}

	movie := map[string]any{"progress": 3.5, "movie": map[string]any{"ids": map[string]any{"imdb": "tt1375666", "tmdb": 27205}}}
	started := c.valid("POST", "/scrobble/start", movie, http.StatusCreated)
	if started["action"] != "start" || started["movie"].(map[string]any)["ids"].(map[string]any)["imdb"] != "tt1375666" {
		t.Errorf("start = %v", started)
	}
	movie["progress"] = 40.0
	c.valid("POST", "/scrobble/pause", movie, http.StatusCreated)
	movie["progress"] = 92.0
	stopped := c.valid("POST", "/scrobble/stop", movie, http.StatusCreated)
	if stopped["action"] != "scrobble" {
		t.Errorf("stop = %v", stopped)
	}
	if again := c.do("POST", "/scrobble/stop", movie); again.Code != http.StatusConflict {
		t.Errorf("second stop = %d", again.Code)
	}

	ep := map[string]any{"progress": 85.0, "show": map[string]any{"ids": map[string]any{"tmdb": 1396}}, "episode": map[string]any{"season": 2, "number": 3}}
	scrobbled := c.valid("POST", "/scrobble/stop", ep, http.StatusCreated)
	if e := scrobbled["episode"].(map[string]any); e["season"] != 2.0 || e["number"] != 3.0 {
		t.Errorf("episode = %v", e)
	}

	found := c.list("GET", "/search/tmdb/1399?type=show", nil)
	if len(found) != 1 || found[0]["type"] != "show" {
		t.Fatalf("search = %v", found)
	}
	slug := found[0]["show"].(map[string]any)["ids"].(map[string]any)["slug"].(string)
	if byIMDB := c.list("GET", "/search/imdb/tt1375666", nil); len(byIMDB) != 1 || byIMDB[0]["type"] != "movie" {
		t.Errorf("imdb search = %v", byIMDB)
	}

	added := c.valid("POST", "/sync/watchlist", map[string]any{"shows": []any{map[string]any{"ids": map[string]any{"slug": slug}}}}, http.StatusCreated)
	if added["added"].(map[string]any)["shows"] != 1.0 || added["list"].(map[string]any)["item_count"] != 1.0 {
		t.Errorf("watchlist add = %v", added)
	}
	listed := c.list("GET", "/sync/watchlist?extended=full", nil)
	if len(listed) != 1 || listed[0]["type"] != "show" || listed[0]["rank"] != 1.0 {
		t.Errorf("watchlist = %v", listed)
	}
	removed := c.valid("POST", "/sync/watchlist/remove", map[string]any{"shows": []any{map[string]any{"ids": map[string]any{"slug": slug}}}}, http.StatusOK)
	if removed["deleted"].(map[string]any)["shows"] != 1.0 {
		t.Errorf("watchlist remove = %v", removed)
	}

	for _, path := range []string{"/users/me/lists", "/users/me/lists/collaborations", "/users/me/likes/lists?page=1"} {
		if got := c.list("GET", path, nil); len(got) != 0 {
			t.Errorf("%s = %v", path, got)
		}
	}

	refreshed := c.valid("POST", "/oauth/token", map[string]string{
		"refresh_token": "nope", "client_id": app.ClientID, "client_secret": app.ClientSecret,
		"redirect_uri": "zuno://trakt/callback", "grant_type": "refresh_token",
	}, http.StatusUnauthorized)
	if refreshed["error"] != "invalid_grant" {
		t.Errorf("bad refresh = %v", refreshed)
	}
}

func TestSyncEndpoints(t *testing.T) {
	db := dbtest.New(t)
	c, _ := signIn(t, db)

	c.valid("POST", "/scrobble/pause", map[string]any{"progress": 33.0, "movie": map[string]any{"ids": map[string]any{"tmdb": 603}}}, http.StatusCreated)
	playback := c.list("GET", "/sync/playback", nil)
	if len(playback) != 1 || playback[0]["type"] != "movie" || playback[0]["progress"] != 33.0 {
		t.Fatalf("playback = %v", playback)
	}
	if episodes := c.list("GET", "/sync/playback/episodes", nil); len(episodes) != 0 {
		t.Errorf("episode playback = %v", episodes)
	}

	history := map[string]any{
		"movies": []any{map[string]any{"watched_at": "2024-02-01T20:00:00.000Z", "ids": map[string]any{"tmdb": 603}}},
		"shows": []any{map[string]any{
			"ids":     map[string]any{"tvdb": 81189, "tmdb": 1396},
			"seasons": []any{map[string]any{"number": 1, "episodes": []any{map[string]any{"number": 1}, map[string]any{"number": 2}}}},
		}},
		"episodes": []any{map[string]any{"ids": map[string]any{"trakt": 73640}}},
	}
	added := c.valid("POST", "/sync/history", history, http.StatusCreated)
	if a := added["added"].(map[string]any); a["movies"] != 1.0 || a["episodes"] != 2.0 {
		t.Errorf("added = %v", added)
	}
	if missing := added["not_found"].(map[string]any)["episodes"].([]any); len(missing) != 1 {
		t.Errorf("not found = %v", added["not_found"])
	}
	if left := c.list("GET", "/sync/playback", nil); len(left) != 0 {
		t.Errorf("playback kept after the film was watched: %v", left)
	}

	rec := c.do("GET", "/sync/history?page=1&limit=2", nil)
	if rec.Header().Get("X-Pagination-Item-Count") != "3" || rec.Header().Get("X-Pagination-Page-Count") != "2" {
		t.Errorf("pagination = %v", rec.Header())
	}
	entries := c.list("GET", "/sync/history/episodes", nil)
	if len(entries) != 2 || entries[0]["action"] != "watch" || entries[0]["show"].(map[string]any)["ids"].(map[string]any)["tvdb"] != 81189.0 {
		t.Fatalf("episode history = %v", entries)
	}

	activity := c.valid("GET", "/sync/last_activities", nil, http.StatusOK)
	if activity["all"] != activity["episodes"].(map[string]any)["watched_at"] {
		t.Errorf("all should be the latest change: %v", activity)
	}
	etag := c.do("GET", "/sync/last_activities", nil).Header().Get("ETag")
	req := httptest.NewRequest("GET", "/sync/last_activities", nil)
	req.Header.Set("trakt-api-key", c.key)
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("If-None-Match", etag)
	cached := httptest.NewRecorder()
	c.server.ServeHTTP(cached, req)
	if cached.Code != http.StatusNotModified {
		t.Errorf("unchanged last_activities = %d", cached.Code)
	}

	id := entries[0]["id"].(float64)
	removed := c.valid("POST", "/sync/history/remove", map[string]any{"ids": []any{id, 5}}, http.StatusOK)
	if removed["deleted"].(map[string]any)["episodes"] != 1.0 || len(removed["not_found"].(map[string]any)["ids"].([]any)) != 1 {
		t.Errorf("removed = %v", removed)
	}
}

func TestRejections(t *testing.T) {
	db := dbtest.New(t)
	c, _ := signIn(t, db)

	token := c.token
	c.token = "nope"
	if rec := c.do("GET", "/sync/last_activities", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("bad token = %d", rec.Code)
	}
	c.token, c.key = token, "tz_unknown"
	if rec := c.do("GET", "/sync/last_activities", nil); rec.Code != http.StatusForbidden {
		t.Errorf("unknown api key = %d", rec.Code)
	}
	if rec := c.do("POST", "/oauth/device/code", map[string]string{"client_id": "tz_unknown"}); rec.Code != http.StatusForbidden {
		t.Errorf("unknown client = %d", rec.Code)
	}

	rec := c.do("GET", "/oauth/authorize?response_type=code&client_id=x&redirect_uri=zuno%3A%2F%2Ftrakt%2Fcallback", nil)
	if rec.Code != http.StatusFound || !strings.HasPrefix(rec.Header().Get("Location"), "https://tethr.tv/authorize?response_type=code") {
		t.Errorf("authorize = %d %s", rec.Code, rec.Header().Get("Location"))
	}
}
