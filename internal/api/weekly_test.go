package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tethrtv/tethr/internal/dbtest"
)

func TestWeeklyEmailSettings(t *testing.T) {
	db := dbtest.New(t)
	mail := &fakeMail{}
	h := (&Server{DB: db, Mail: mail, AllowedOrigins: []string{"https://staging.tethr.tv"}, Cookie: CookieConfig{Name: "tethr_session"}}).Handler()
	cookie := signIn(t, h, mail)

	if rec := do(t, h, "GET", "/v1/me", "", cookie); !strings.Contains(rec.Body.String(), `"time_zone":null`) || !strings.Contains(rec.Body.String(), `"weekly_email":true`) {
		t.Errorf("a new account = %s", rec.Body)
	}
	if rec := do(t, h, "PATCH", "/v1/me", `{"time_zone":"Narnia"}`, cookie); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_time_zone") {
		t.Errorf("a made-up zone = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "PATCH", "/v1/me", `{"time_zone":"Europe/Paris"}`, cookie); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"time_zone":"Europe/Paris"`) {
		t.Errorf("setting the zone = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "PATCH", "/v1/me", `{"weekly_email":true}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("signed out = %d", rec.Code)
	}

	var token string
	if err := db.QueryRow(t.Context(), "SELECT unsubscribe_token::text FROM accounts").Scan(&token); err != nil {
		t.Fatal(err)
	}
	if rec := do(t, h, "POST", "/v1/unsubscribe?token=nonsense", "List-Unsubscribe=One-Click"); rec.Code != http.StatusNotFound {
		t.Errorf("a bad token = %d", rec.Code)
	}
	req := httptest.NewRequest("POST", "/v1/unsubscribe?token="+token, strings.NewReader("List-Unsubscribe=One-Click"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("one-click unsubscribe from a mail app = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "GET", "/v1/me", "", cookie); !strings.Contains(rec.Body.String(), `"weekly_email":false`) || !strings.Contains(rec.Body.String(), `"time_zone":"Europe/Paris"`) {
		t.Errorf("after unsubscribing = %s", rec.Body)
	}
}
