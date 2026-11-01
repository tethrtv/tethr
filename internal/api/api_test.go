package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/dbtest"
	"github.com/tethrtv/tethr/internal/mail"
)

type sentMail struct{ to, subject, body, html string }

type fakeMail struct {
	mu   sync.Mutex
	sent []sentMail
}

func (f *fakeMail) Send(_ context.Context, m mail.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, sentMail{m.To, m.Subject, m.Text, m.HTML})
	return nil
}

func (f *fakeMail) last(t *testing.T) sentMail {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.sent) == 0 {
		t.Fatal("no email sent")
	}
	return f.sent[len(f.sent)-1]
}

func newTestServer(t *testing.T) (http.Handler, *fakeMail) {
	mail := &fakeMail{}
	s := &Server{
		DB:             dbtest.New(t),
		Mail:           mail,
		AppURL:         "https://staging.tethr.tv",
		AllowedOrigins: []string{"https://staging.tethr.tv"},
		Cookie:         CookieConfig{Name: "tethr_session", Domain: ".tethr.tv", Secure: true},
	}
	return s.Handler(), mail
}

func do(t *testing.T, h http.Handler, method, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://staging.tethr.tv")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestSignUpFlow(t *testing.T) {
	h, mail := newTestServer(t)

	rec := do(t, h, "POST", "/v1/accounts", `{"email":"nathan@example.com","name":"Nathan","username":"nathan"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("sign up = %d %s", rec.Code, rec.Body)
	}
	email := mail.last(t)
	if email.to != "nathan@example.com" || !strings.Contains(email.body, "https://staging.tethr.tv/verify?token=") || !strings.Contains(email.html, `href="https://staging.tethr.tv/verify?token=`) {
		t.Fatalf("email = %+v", email)
	}
	token := email.body[strings.Index(email.body, "token=")+len("token="):]
	token = token[:strings.IndexAny(token, "\n ")]

	rec = do(t, h, "POST", "/v1/auth/verify", `{"token":"`+token+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("verify = %d %s", rec.Code, rec.Body)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "tethr_session" || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].Domain != "tethr.tv" {
		t.Fatalf("cookies = %+v", cookies)
	}

	rec = do(t, h, "GET", "/v1/me", "", cookies[0])
	if rec.Code != http.StatusOK {
		t.Fatalf("me = %d %s", rec.Code, rec.Body)
	}
	var me Me
	if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
		t.Fatal(err)
	}
	if me.Account.Email != "nathan@example.com" || len(me.Profiles) != 1 || me.Profiles[0].Username != "nathan" {
		t.Errorf("me = %+v", me)
	}

	rec = do(t, h, "POST", "/v1/auth/sign-out", "", cookies[0])
	if rec.Code != http.StatusNoContent {
		t.Fatalf("sign out = %d", rec.Code)
	}
	if rec := do(t, h, "GET", "/v1/me", "", cookies[0]); rec.Code != http.StatusUnauthorized {
		t.Errorf("me after sign-out = %d, want 401", rec.Code)
	}
}

func TestProblems(t *testing.T) {
	h, _ := newTestServer(t)
	do(t, h, "POST", "/v1/accounts", `{"email":"nathan@example.com","name":"Nathan","username":"nathan"}`)

	tests := []struct {
		name, path, body string
		status           int
		code             string
	}{
		{"taken username", "/v1/accounts", `{"email":"sam@example.com","name":"Sam","username":"nathan"}`, 409, "username_taken"},
		{"bad username", "/v1/accounts", `{"email":"sam@example.com","name":"Sam","username":"Sam!"}`, 400, "invalid_username"},
		{"bad email", "/v1/auth/email-link", `{"email":"nope"}`, 400, "invalid_email"},
		{"bad token", "/v1/auth/verify", `{"token":"made-up"}`, 400, "invalid_link"},
		{"nothing to verify", "/v1/auth/verify", `{}`, 400, "invalid_request"},
		{"broken json", "/v1/auth/verify", `{`, 400, "invalid_request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, h, "POST", tt.path, tt.body)
			var p Problem
			_ = json.Unmarshal(rec.Body.Bytes(), &p)
			if rec.Code != tt.status || p.Code != tt.code {
				t.Errorf("= %d %q, want %d %q (%s)", rec.Code, p.Code, tt.status, tt.code, rec.Body)
			}
		})
	}

	if rec := do(t, h, "GET", "/v1/me", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("me without a cookie = %d, want 401", rec.Code)
	}
}

func TestUnknownEmailStillAccepted(t *testing.T) {
	h, mail := newTestServer(t)
	if rec := do(t, h, "POST", "/v1/auth/email-link", `{"email":"nobody@example.com"}`); rec.Code != http.StatusAccepted {
		t.Errorf("= %d, want 202", rec.Code)
	}
	if len(mail.sent) != 0 {
		t.Errorf("sent %d emails to an address with no account", len(mail.sent))
	}
}

func TestOrigins(t *testing.T) {
	h, _ := newTestServer(t)

	req := httptest.NewRequest("OPTIONS", "/v1/auth/verify", nil)
	req.Header.Set("Origin", "https://staging.tethr.tv")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") != "https://staging.tethr.tv" || rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Errorf("preflight = %d %v", rec.Code, rec.Header())
	}

	req = httptest.NewRequest("POST", "/v1/auth/email-link", strings.NewReader(`{"email":"a@b.c"}`))
	req.Header.Set("Origin", "https://evil.example")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("post from another site = %d, want 403", rec.Code)
	}
}

func signIn(t *testing.T, h http.Handler, mail *fakeMail) *http.Cookie {
	t.Helper()
	if rec := do(t, h, "POST", "/v1/accounts", `{"email":"nathan@example.com","name":"Nathan","username":"nathan"}`); rec.Code != http.StatusAccepted {
		t.Fatalf("sign up = %d %s", rec.Code, rec.Body)
	}
	body := mail.last(t).body
	token := body[strings.Index(body, "token=")+len("token="):]
	token = token[:strings.IndexAny(token, "\n ")]
	rec := do(t, h, "POST", "/v1/auth/verify", `{"token":"`+token+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("verify = %d %s", rec.Code, rec.Body)
	}
	return rec.Result().Cookies()[0]
}

func TestHousehold(t *testing.T) {
	h, mail := newTestServer(t)
	cookie := signIn(t, h, mail)

	rec := do(t, h, "POST", "/v1/profiles", `{"name":"Sam","kind":"person"}`, cookie)
	if rec.Code != http.StatusCreated {
		t.Fatalf("add = %d %s", rec.Code, rec.Body)
	}
	var sam Profile
	_ = json.Unmarshal(rec.Body.Bytes(), &sam)
	if sam.Username != "nathan-sam" {
		t.Errorf("username = %q", sam.Username)
	}

	path := "/v1/profiles/" + strconv.FormatInt(sam.Id, 10)
	if rec := do(t, h, "PATCH", path, `{"name":"Samantha","private":false}`, cookie); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"name":"Samantha"`) {
		t.Errorf("rename = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "POST", "/v1/profiles", `{"name":"Boss","kind":"owner"}`, cookie); rec.Code != http.StatusBadRequest {
		t.Errorf("owner kind = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "DELETE", path, "", cookie); rec.Code != http.StatusNoContent {
		t.Errorf("remove = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "DELETE", path, "", cookie); rec.Code != http.StatusNotFound {
		t.Errorf("remove twice = %d", rec.Code)
	}
	if rec := do(t, h, "POST", "/v1/profiles", `{"name":"Sam","kind":"person"}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("signed out = %d", rec.Code)
	}
}

func TestSessionsAndDeletion(t *testing.T) {
	h, mail := newTestServer(t)
	cookie := signIn(t, h, mail)

	rec := do(t, h, "GET", "/v1/sessions", "", cookie)
	var sessions []Session
	_ = json.Unmarshal(rec.Body.Bytes(), &sessions)
	if rec.Code != http.StatusOK || len(sessions) != 1 || !sessions[0].Current {
		t.Fatalf("sessions = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "POST", "/v1/sessions/sign-out-others", "", cookie); rec.Code != http.StatusNoContent {
		t.Errorf("sign out others = %d", rec.Code)
	}

	if rec := do(t, h, "DELETE", "/v1/me", `{"username":"someone"}`, cookie); rec.Code != http.StatusBadRequest {
		t.Errorf("delete with the wrong username = %d", rec.Code)
	}
	rec = do(t, h, "DELETE", "/v1/me", `{"username":"nathan"}`, cookie)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "GET", "/v1/me", "", cookie); rec.Code != http.StatusUnauthorized {
		t.Errorf("me after deletion = %d", rec.Code)
	}
}

func TestInviteOnly(t *testing.T) {
	mail := &fakeMail{}
	db := dbtest.New(t)
	s := &Server{DB: db, Mail: mail, RequireInvite: true, AppURL: "https://tethr.tv", AllowedOrigins: []string{"https://staging.tethr.tv"}, Cookie: CookieConfig{Name: "tethr_session"}}
	h := s.Handler()

	rec := do(t, h, "POST", "/v1/accounts", `{"email":"sam@example.com","name":"Sam","username":"sam"}`)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "invite_required") {
		t.Errorf("no invite = %d %s", rec.Code, rec.Body)
	}
	code, err := core.CreateInvite(context.Background(), db, "")
	if err != nil {
		t.Fatal(err)
	}
	if rec := do(t, h, "POST", "/v1/accounts", `{"email":"sam@example.com","name":"Sam","username":"sam","invite":"`+code+`"}`); rec.Code != http.StatusAccepted {
		t.Errorf("with an invite = %d %s", rec.Code, rec.Body)
	}
}
