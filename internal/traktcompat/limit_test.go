package traktcompat

import (
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/tethrtv/tethr/internal/dbtest"
	"github.com/tethrtv/tethr/internal/limit"
)

func TestRateLimits(t *testing.T) {
	db := dbtest.New(t)
	c, app := signIn(t, db)
	mux := http.NewServeMux()
	(&Server{DB: db, AppURL: "https://tethr.tv", Log: slog.New(slog.DiscardHandler), Limits: limit.Standard("")}).Routes(mux)
	c.server = mux

	for i := range 10 {
		if rec := c.do("POST", "/sync/history", map[string]any{}); rec.Code == http.StatusTooManyRequests {
			t.Fatalf("write %d refused", i+1)
		}
	}
	rec := c.do("POST", "/sync/history", map[string]any{})
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "1" || !strings.Contains(rec.Header().Get("X-Ratelimit"), `"name":"AUTHED_API_POST_LIMIT"`) {
		t.Fatalf("eleventh write = %d %v %s", rec.Code, rec.Header(), rec.Body)
	}
	if exposed := rec.Header().Get("Access-Control-Expose-Headers"); !strings.Contains(exposed, "Retry-After") || !strings.Contains(exposed, "X-Ratelimit") {
		t.Errorf("exposed headers = %q", exposed)
	}
	if rec := c.do("GET", "/sync/last_activities", nil); rec.Code != http.StatusOK {
		t.Errorf("read after the writes ran out = %d", rec.Code)
	}

	c.token = ""
	for range 25 {
		if rec := c.do("POST", "/oauth/device/token", map[string]string{"code": "nope", "client_id": app.ClientID}); strings.Contains(rec.Body.String(), "rate_limited") {
			t.Fatal("device code polling was rate limited")
		}
	}
	for i := range 20 {
		if rec := c.do("POST", "/oauth/device/code", map[string]string{"client_id": app.ClientID}); rec.Code == http.StatusTooManyRequests {
			t.Fatalf("device code %d refused", i+1)
		}
	}
	if rec := c.do("POST", "/oauth/device/code", map[string]string{"client_id": app.ClientID}); rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Header().Get("X-Ratelimit"), `"name":"UNAUTHED_API_POST_LIMIT"`) {
		t.Errorf("twenty-first device code = %d %v", rec.Code, rec.Header())
	}
	if rec := c.do("GET", "/search/tmdb/1396?type=show", nil); rec.Code == http.StatusTooManyRequests {
		t.Errorf("a lookup was refused = %d", rec.Code)
	}
}
