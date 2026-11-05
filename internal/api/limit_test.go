package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/tethrtv/tethr/internal/dbtest"
	"github.com/tethrtv/tethr/internal/limit"
)

func limitedServer(t *testing.T) (http.Handler, *fakeMail) {
	mail := &fakeMail{}
	s := &Server{DB: dbtest.New(t), Mail: mail, AppURL: "https://staging.tethr.tv", AllowedOrigins: []string{"https://staging.tethr.tv"}, Cookie: CookieConfig{Name: "tethr_session"}, Limits: limit.Standard("")}
	return s.Handler(), mail
}

func TestRateLimits(t *testing.T) {
	h, mail := limitedServer(t)
	cookie := signIn(t, h, mail)

	for i := range 10 {
		if rec := do(t, h, "POST", "/v1/profiles", `{"name":"Sam","kind":"nope"}`, cookie); rec.Code != http.StatusBadRequest {
			t.Fatalf("write %d = %d %s", i+1, rec.Code, rec.Body)
		}
	}
	rec := do(t, h, "POST", "/v1/profiles", `{"name":"Sam","kind":"nope"}`, cookie)
	if rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), "rate_limited") {
		t.Fatalf("eleventh write = %d %s", rec.Code, rec.Body)
	}
	if rec.Header().Get("Retry-After") != "1" || !strings.Contains(rec.Header().Get("X-Ratelimit"), `"name":"AUTHED_API_POST_LIMIT"`) {
		t.Errorf("headers = %v", rec.Header())
	}
	if exposed := rec.Header().Get("Access-Control-Expose-Headers"); !strings.Contains(exposed, "Retry-After") || !strings.Contains(exposed, "X-Ratelimit") {
		t.Errorf("exposed headers = %q", exposed)
	}
	if rec := do(t, h, "GET", "/v1/me", "", cookie); rec.Code != http.StatusOK {
		t.Errorf("read after the writes ran out = %d", rec.Code)
	}
}

func TestRateLimitsWithoutSignIn(t *testing.T) {
	h, _ := limitedServer(t)
	for i := range 20 {
		if rec := do(t, h, "POST", "/v1/auth/email-link", `{}`); rec.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d refused", i+1)
		}
	}
	rec := do(t, h, "POST", "/v1/auth/email-link", `{}`)
	if rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Header().Get("X-Ratelimit"), `"name":"UNAUTHED_API_POST_LIMIT"`) {
		t.Errorf("twenty-first = %d %v", rec.Code, rec.Header())
	}
	if rec := do(t, h, "GET", "/v1/me", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("read without sign-in = %d", rec.Code)
	}
}
