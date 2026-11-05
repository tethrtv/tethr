package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tethrtv/tethr/internal/dbtest"
)

func TestSignedIn(t *testing.T) {
	mail := &fakeMail{}
	s := &Server{DB: dbtest.New(t), Mail: mail, AllowedOrigins: []string{"https://staging.tethr.tv"}, Cookie: CookieConfig{Name: "tethr_session"}}
	cookie := signIn(t, s.Handler(), mail)
	var seen int64
	h := s.SignedIn(func(w http.ResponseWriter, r *http.Request, accountID int64) {
		seen = accountID
	})

	get := func(c *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/meta/search?q=bad", nil)
		req.Header.Set("Origin", "https://staging.tethr.tv")
		if c != nil {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if rec := get(nil); rec.Code != http.StatusUnauthorized || seen != 0 {
		t.Errorf("no cookie = %d", rec.Code)
	}
	if rec := get(&http.Cookie{Name: "tethr_session", Value: "nope"}); rec.Code != http.StatusUnauthorized || seen != 0 {
		t.Errorf("unknown session = %d", rec.Code)
	}
	rec := get(cookie)
	if rec.Code != http.StatusOK || seen == 0 {
		t.Errorf("signed in = %d, account %d", rec.Code, seen)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://staging.tethr.tv" || rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Errorf("cors = %v", rec.Header())
	}
}
