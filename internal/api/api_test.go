package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

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

func TestConnectingApps(t *testing.T) {
	mail := &fakeMail{}
	s := &Server{
		DB:             dbtest.New(t),
		Mail:           mail,
		AppURL:         "https://staging.tethr.tv",
		AllowedOrigins: []string{"https://staging.tethr.tv"},
		Cookie:         CookieConfig{Name: "tethr_session"},
	}
	h := s.Handler()
	cookie := signIn(t, h, mail)
	ctx := context.Background()
	app, err := core.CreateApp(ctx, s.DB, "Zuno", []string{"zuno://trakt/callback"})
	if err != nil {
		t.Fatal(err)
	}
	var me Me
	_ = json.Unmarshal(do(t, h, "GET", "/v1/me", "", cookie).Body.Bytes(), &me)
	profile := strconv.FormatInt(me.Profiles[0].Id, 10)

	device, err := core.StartDeviceCode(ctx, s.DB, app.ClientID)
	if err != nil {
		t.Fatal(err)
	}
	code := strings.ToLower(device.UserCode[:4] + "-" + device.UserCode[4:])
	if rec := do(t, h, "GET", "/v1/devices/"+code, "", cookie); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"app_name":"Zuno"`) {
		t.Errorf("look up = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "GET", "/v1/devices/AAAAAAAA", "", cookie); rec.Code != http.StatusNotFound {
		t.Errorf("unknown code = %d", rec.Code)
	}
	if rec := do(t, h, "POST", "/v1/devices/"+code+"/approve", `{"profile_id":`+profile+`}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("signed out approval = %d", rec.Code)
	}
	if rec := do(t, h, "POST", "/v1/devices/"+code+"/approve", `{"profile_id":`+profile+`}`, cookie); rec.Code != http.StatusNoContent {
		t.Fatalf("approve = %d %s", rec.Code, rec.Body)
	}
	if _, err := core.PollDeviceCode(ctx, s.DB, app.ClientID, "", device.Code, "Zuno/2.3"); err != nil {
		t.Fatal(err)
	}

	query := "?client_id=" + app.ClientID + "&redirect_uri=zuno%3A%2F%2Ftrakt%2Fcallback"
	if rec := do(t, h, "GET", "/v1/authorize"+query, "", cookie); rec.Code != http.StatusOK {
		t.Errorf("check authorize = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "GET", "/v1/authorize?client_id="+app.ClientID+"&redirect_uri=https%3A%2F%2Fevil.example", "", cookie); rec.Code != http.StatusBadRequest {
		t.Errorf("unregistered redirect = %d", rec.Code)
	}
	rec := do(t, h, "POST", "/v1/authorize", `{"client_id":"`+app.ClientID+`","redirect_uri":"zuno://trakt/callback","profile_id":`+profile+`,"state":"xyz"}`, cookie)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `zuno://trakt/callback?code=`) || !strings.Contains(rec.Body.String(), `state=xyz`) {
		t.Errorf("authorize = %d %s", rec.Code, rec.Body)
	}

	var connections []Connection
	_ = json.Unmarshal(do(t, h, "GET", "/v1/connections", "", cookie).Body.Bytes(), &connections)
	if len(connections) != 1 || connections[0].AppName != "Zuno" || *connections[0].UserAgent != "Zuno/2.3" {
		t.Fatalf("connections = %+v", connections)
	}
	path := "/v1/connections/" + strconv.FormatInt(connections[0].Id, 10)
	if rec := do(t, h, "PATCH", path, `{"label":"Living room"}`, cookie); rec.Code != http.StatusNoContent {
		t.Errorf("rename = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "DELETE", path, "", cookie); rec.Code != http.StatusNoContent {
		t.Errorf("disconnect = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "DELETE", path, "", cookie); rec.Code != http.StatusNotFound {
		t.Errorf("disconnect twice = %d", rec.Code)
	}
}

func bearer(t *testing.T, h http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestNativeTracking(t *testing.T) {
	mail := &fakeMail{}
	s := &Server{DB: dbtest.New(t), Mail: mail, AppURL: "https://staging.tethr.tv", AllowedOrigins: []string{"https://staging.tethr.tv"}, Cookie: CookieConfig{Name: "tethr_session"}}
	h := s.Handler()
	cookie := signIn(t, h, mail)
	ctx := context.Background()

	rec := do(t, h, "POST", "/v1/profiles", `{"name":"Sam","kind":"person"}`, cookie)
	var sam Profile
	_ = json.Unmarshal(rec.Body.Bytes(), &sam)
	app, err := core.CreateApp(ctx, s.DB, "Zuno", nil)
	if err != nil {
		t.Fatal(err)
	}
	device, err := core.StartDeviceCode(ctx, s.DB, app.ClientID)
	if err != nil {
		t.Fatal(err)
	}
	if rec := do(t, h, "POST", "/v1/devices/"+device.UserCode+"/approve", `{"profile_id":`+strconv.FormatInt(sam.Id, 10)+`}`, cookie); rec.Code != http.StatusNoContent {
		t.Fatalf("approve = %d %s", rec.Code, rec.Body)
	}
	tokens, err := core.PollDeviceCode(ctx, s.DB, app.ClientID, "", device.Code, "Zuno")
	if err != nil {
		t.Fatal(err)
	}
	token := tokens.Access

	if rec := bearer(t, h, "GET", "/v1/profile", "", token); !strings.Contains(rec.Body.String(), `"username":"nathan-sam"`) {
		t.Errorf("app profile = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "GET", "/v1/profile", "", cookie); !strings.Contains(rec.Body.String(), `"username":"nathan"`) {
		t.Errorf("website profile = %d %s", rec.Code, rec.Body)
	}
	if rec := bearer(t, h, "GET", "/v1/profile", "", "nope"); rec.Code != http.StatusUnauthorized {
		t.Errorf("bad token = %d", rec.Code)
	}

	episode := `{"event":"%s","progress":%d,"item":{"type":"episode","show":{"tmdb":1396},"season":1,"number":2}}`
	rec = bearer(t, h, "POST", "/v1/scrobble", fmt.Sprintf(episode, "start", 1), token)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"result":"watching"`) {
		t.Fatalf("start = %d %s", rec.Code, rec.Body)
	}
	rec = bearer(t, h, "POST", "/v1/scrobble", fmt.Sprintf(episode, "stop", 40), token)
	var paused ScrobbleResult
	_ = json.Unmarshal(rec.Body.Bytes(), &paused)
	if paused.Result != Paused || paused.Item.Episode == nil || paused.Item.Episode.Number != 2 || *paused.Item.Title.Ids.Tmdb != 1396 {
		t.Errorf("early stop = %s", rec.Body)
	}
	var resume []ResumePoint
	_ = json.Unmarshal(bearer(t, h, "GET", "/v1/playback", "", token).Body.Bytes(), &resume)
	if len(resume) != 1 || resume[0].Id != *paused.Id {
		t.Errorf("playback = %+v", resume)
	}
	if rec := bearer(t, h, "POST", "/v1/scrobble", fmt.Sprintf(episode, "stop", 97), token); !strings.Contains(rec.Body.String(), `"result":"watched"`) {
		t.Errorf("stop = %s", rec.Body)
	}
	if rec := bearer(t, h, "POST", "/v1/scrobble", fmt.Sprintf(episode, "stop", 98), token); rec.Code != http.StatusConflict {
		t.Errorf("second stop = %d", rec.Code)
	}
	if rec := bearer(t, h, "POST", "/v1/scrobble", `{"event":"stop","progress":90,"item":{"type":"movie"}}`, token); rec.Code != http.StatusNotFound {
		t.Errorf("no ids = %d", rec.Code)
	}

	rec = bearer(t, h, "POST", "/v1/history", `{"items":[{"item":{"type":"movie","ids":{"imdb":"tt0133093"}},"watched_at":"2024-05-01T20:00:00Z"},{"item":{"type":"movie","ids":{"imdb":"tt0133093"}},"watched_at":"2024-05-01T20:00:00Z"},{"item":{"type":"show","ids":{"tmdb":1399}}}]}`, token)
	if !strings.Contains(rec.Body.String(), `"added":1,"existing":1`) || !strings.Contains(rec.Body.String(), `"not_found":[{"ids":{"tmdb":1399},"type":"show"}]`) {
		t.Errorf("add history = %s", rec.Body)
	}
	if err := core.SaveWikidata(ctx, s.DB, *paused.Item.Title.Ids.Tethr, "show", core.WikidataTitle{QID: "Q1079", Name: "Breaking Bad", Original: "Breaking Bad", Year: 2008}); err != nil {
		t.Fatal(err)
	}
	var history HistoryPage
	_ = json.Unmarshal(bearer(t, h, "GET", "/v1/history?limit=1", "", token).Body.Bytes(), &history)
	if history.Total != 2 || len(history.Items) != 1 || history.Items[0].Item.Type != ItemTypeEpisode {
		t.Fatalf("history = %+v", history)
	}
	if show := history.Items[0].Item.Title; *show.Name != "Breaking Bad" || *show.OriginalName != "Breaking Bad" || *show.Year != 2008 {
		t.Errorf("show = %+v", show)
	}
	var detail TitleDetail
	rec = bearer(t, h, "GET", "/v1/titles/breaking-bad-2008", "", token)
	_ = json.Unmarshal(rec.Body.Bytes(), &detail)
	if rec.Code != http.StatusOK || detail.Type != TitleDetailTypeShow || *detail.Title.Slug != "breaking-bad-2008" || detail.Seasons == nil {
		t.Fatalf("title = %d %s", rec.Code, rec.Body)
	}
	if e := (*detail.Seasons)[0].Episodes[0]; e.Episode.Number != 2 || len(e.Watches) != 1 || e.Watches[0].Action != PlayActionScrobble {
		t.Errorf("episode = %+v", e)
	}
	if rec := bearer(t, h, "GET", "/v1/titles/no-such-show-1999", "", token); rec.Code != http.StatusNotFound {
		t.Errorf("unknown title = %d", rec.Code)
	}
	var found []Item
	_ = json.Unmarshal(bearer(t, h, "GET", "/v1/search?q=bad", "", token).Body.Bytes(), &found)
	if len(found) != 1 || *found[0].Title.Slug != "breaking-bad-2008" {
		t.Errorf("search = %+v", found)
	}
	if rec := do(t, h, "GET", "/v1/search?q=bad", "", cookie); rec.Body.String() != "[]\n" {
		t.Errorf("Sam's library showed up in Nathan's search: %s", rec.Body)
	}
	var mine HistoryPage
	_ = json.Unmarshal(do(t, h, "GET", "/v1/history", "", cookie).Body.Bytes(), &mine)
	if mine.Total != 0 {
		t.Errorf("Sam's watches showed up in Nathan's history: %+v", mine)
	}
	watch := "/v1/history/" + strconv.FormatInt(history.Items[0].Id, 10)
	if rec := bearer(t, h, "DELETE", watch, "", token); rec.Code != http.StatusNoContent {
		t.Errorf("remove watch = %d", rec.Code)
	}
	if rec := bearer(t, h, "DELETE", watch, "", token); rec.Code != http.StatusNotFound {
		t.Errorf("remove watch twice = %d", rec.Code)
	}

	if rec := bearer(t, h, "POST", "/v1/watchlist", `{"items":[{"item":{"type":"show","ids":{"tmdb":1399}},"notes":"after Andor"},{"item":{"type":"season","show":{"tmdb":1399},"season":2}}]}`, token); !strings.Contains(rec.Body.String(), `"added":2`) {
		t.Errorf("watchlist add = %s", rec.Body)
	}
	var listed []Listing
	_ = json.Unmarshal(bearer(t, h, "GET", "/v1/watchlist", "", token).Body.Bytes(), &listed)
	if len(listed) != 2 || *listed[0].Notes != "after Andor" || listed[1].Item.Type != ItemTypeSeason || *listed[1].Item.Season != 2 {
		t.Fatalf("watchlist = %+v", listed)
	}
	if rec := bearer(t, h, "DELETE", "/v1/watchlist/"+strconv.FormatInt(listed[1].Id, 10), "", token); rec.Code != http.StatusNoContent {
		t.Errorf("watchlist remove = %d", rec.Code)
	}

	var activity Activity
	_ = json.Unmarshal(bearer(t, h, "GET", "/v1/activity", "", token).Body.Bytes(), &activity)
	if !activity.Latest.Equal(activity.Watchlist) || activity.History.IsZero() || activity.Playback.After(activity.History) {
		t.Errorf("activity = %+v", activity)
	}
}

func TestDevelopers(t *testing.T) {
	h, mail := newTestServer(t)
	cookie := signIn(t, h, mail)

	rec := do(t, h, "POST", "/v1/apps", `{"name":"My Player","redirect_uris":["myplayer://callback"]}`, cookie)
	var registered RegisteredApp
	_ = json.Unmarshal(rec.Body.Bytes(), &registered)
	if rec.Code != http.StatusCreated || registered.ClientSecret == "" || registered.App.RedirectUris[0] != "myplayer://callback" {
		t.Fatalf("register = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "POST", "/v1/apps", `{"name":"Bad","redirect_uris":["http://evil.example"]}`, cookie); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_redirect") {
		t.Errorf("bad redirect = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "POST", "/v1/apps", `{"name":"Nope"}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("signed out = %d", rec.Code)
	}

	path := "/v1/apps/" + strconv.FormatInt(registered.App.Id, 10)
	if rec := do(t, h, "PATCH", path, `{"name":"My Player 2"}`, cookie); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"redirect_uris":["myplayer://callback"]`) {
		t.Errorf("rename = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "POST", path+"/secret", "", cookie); rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), registered.ClientSecret) {
		t.Errorf("new secret = %d %s", rec.Code, rec.Body)
	}
	var apps []DeveloperApp
	_ = json.Unmarshal(do(t, h, "GET", "/v1/apps", "", cookie).Body.Bytes(), &apps)
	if len(apps) != 1 || apps[0].Name != "My Player 2" {
		t.Errorf("apps = %+v", apps)
	}
	if rec := do(t, h, "DELETE", path, "", cookie); rec.Code != http.StatusNoContent {
		t.Errorf("revoke = %d", rec.Code)
	}
	if rec := do(t, h, "DELETE", path, "", cookie); rec.Code != http.StatusNotFound {
		t.Errorf("revoke twice = %d", rec.Code)
	}
}

func TestUpNextEndpoints(t *testing.T) {
	mail := &fakeMail{}
	s := &Server{DB: dbtest.New(t), Mail: mail, AppURL: "https://staging.tethr.tv", AllowedOrigins: []string{"https://staging.tethr.tv"}, Cookie: CookieConfig{Name: "tethr_session"}}
	h := s.Handler()
	cookie := signIn(t, h, mail)

	rec := do(t, h, "POST", "/v1/scrobble", `{"event":"stop","progress":100,"item":{"type":"episode","show":{"tmdb":1396},"season":1,"number":1}}`, cookie)
	var watched ScrobbleResult
	_ = json.Unmarshal(rec.Body.Bytes(), &watched)
	show := *watched.Item.Title.Ids.Tethr
	day := func(n int) *time.Time { d := time.Now().AddDate(0, 0, -n); return &d }
	if err := core.SaveOutline(context.Background(), s.DB, show, []core.OutlineEpisode{
		{TMDBID: 1, Season: 1, Number: 1, FirstAired: day(20)},
		{TMDBID: 2, Season: 1, Number: 2, FirstAired: day(13)},
	}); err != nil {
		t.Fatal(err)
	}

	var next []UpNextEntry
	_ = json.Unmarshal(do(t, h, "GET", "/v1/up-next", "", cookie).Body.Bytes(), &next)
	if len(next) != 1 || next[0].Next.Number != 2 || next[0].Next.FirstAired == nil || next[0].Aired != 2 || next[0].Watched != 1 || *next[0].Show.Ids.Tmdb != 1396 {
		t.Fatalf("up next = %+v", next)
	}
	path := "/v1/up-next/hidden/" + strconv.FormatInt(show, 10)
	if rec := do(t, h, "PUT", path, "", cookie); rec.Code != http.StatusNoContent {
		t.Errorf("hide = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "GET", "/v1/up-next", "", cookie); rec.Body.String() != "[]\n" {
		t.Errorf("hidden = %s", rec.Body)
	}
	if rec := do(t, h, "DELETE", path, "", cookie); rec.Code != http.StatusNoContent {
		t.Errorf("unhide = %d", rec.Code)
	}
	if rec := do(t, h, "DELETE", path, "", cookie); rec.Code != http.StatusNotFound {
		t.Errorf("unhide twice = %d", rec.Code)
	}
	if rec := do(t, h, "PUT", "/v1/up-next/hidden/5", "", cookie); rec.Code != http.StatusNotFound {
		t.Errorf("hide unknown = %d", rec.Code)
	}
}

func traktZip(t *testing.T) string {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	w, _ := z.Create("watched-history.json")
	_, _ = w.Write([]byte(`[
		{"id":1,"watched_at":"2024-01-01T20:00:00.000Z","action":"watch","type":"movie","movie":{"title":"Inception","year":2010,"ids":{"trakt":16662,"imdb":"tt1375666","tmdb":27205}}},
		{"id":2,"watched_at":"2024-01-02T20:00:00.000Z","action":"watch","type":"movie","movie":{"title":"Lost tape","year":1990,"ids":{"trakt":5}}}
	]`))
	w, _ = z.Create("ratings-movies.json")
	_, _ = w.Write([]byte(`[{"rated_at":"2024-01-03T10:00:00.000Z","rating":9,"type":"movie","movie":{"title":"Inception","year":2010,"ids":{"tmdb":27205}}}]`))
	_ = z.Close()
	return buf.String()
}

func upload(t *testing.T, h http.Handler, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/zip")
	req.Header.Set("Origin", "https://staging.tethr.tv")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestImports(t *testing.T) {
	mail := &fakeMail{}
	s := &Server{DB: dbtest.New(t), Mail: mail, AppURL: "https://staging.tethr.tv", AllowedOrigins: []string{"https://staging.tethr.tv"}, Cookie: CookieConfig{Name: "tethr_session"}}
	h := s.Handler()
	cookie := signIn(t, h, mail)

	var sam Profile
	_ = json.Unmarshal(do(t, h, "POST", "/v1/profiles", `{"name":"Sam","kind":"person"}`, cookie).Body.Bytes(), &sam)
	samID := strconv.FormatInt(sam.Id, 10)

	if rec := upload(t, h, "/v1/imports", "not a zip", cookie); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "not_an_export") {
		t.Errorf("not a zip = %d %s", rec.Code, rec.Body)
	}
	if rec := upload(t, h, "/v1/imports?profile_id=99999", traktZip(t), cookie); rec.Code != http.StatusNotFound {
		t.Errorf("someone else's profile = %d %s", rec.Code, rec.Body)
	}
	rec := upload(t, h, "/v1/imports?profile_id="+samID+"&filename=trakt.zip", traktZip(t), cookie)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body)
	}
	var imp Import
	_ = json.Unmarshal(rec.Body.Bytes(), &imp)
	if imp.Status != Parsed || imp.Filename != "trakt.zip" || imp.Summary.History.Total != 2 || imp.Summary.History.Unmatched != 1 || imp.Summary.Unmatched[0] != "Lost tape (1990)" {
		t.Fatalf("import = %+v", imp)
	}

	id := strconv.FormatInt(imp.Id, 10)
	if rec := do(t, h, "POST", "/v1/imports/"+id+"/apply", "", cookie); rec.Code != http.StatusNotFound {
		t.Errorf("applying from the owner's profile = %d", rec.Code)
	}
	if rec := do(t, h, "POST", "/v1/imports/"+id+"/apply?profile_id="+samID, "", cookie); rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), `"status":"queued"`) {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body)
	}
	if _, err := core.ApplyNextImport(context.Background(), s.DB); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(do(t, h, "GET", "/v1/imports/"+id+"?profile_id="+samID, "", cookie).Body.Bytes(), &imp)
	if imp.Status != Applied || imp.Summary.History.Added != 1 || imp.Summary.Ratings.Added != 1 {
		t.Errorf("applied = %+v", imp)
	}
	if rec := do(t, h, "GET", "/v1/history", "", cookie); !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Errorf("the import landed on the owner's profile: %s", rec.Body)
	}
	var list []Import
	_ = json.Unmarshal(do(t, h, "GET", "/v1/imports?profile_id="+samID, "", cookie).Body.Bytes(), &list)
	if len(list) != 1 {
		t.Errorf("imports = %+v", list)
	}
	if rec := do(t, h, "POST", "/v1/imports/"+id+"/undo?profile_id="+samID, "", cookie); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"undone"`) {
		t.Errorf("undo = %d %s", rec.Code, rec.Body)
	}
}

func TestRatingEndpoints(t *testing.T) {
	h, mail := newTestServer(t)
	cookie := signIn(t, h, mail)

	rec := do(t, h, "POST", "/v1/ratings", `{"items":[{"item":{"type":"movie","ids":{"tmdb":27205}},"rating":8},{"item":{"type":"movie","ids":{"tmdb":603}},"rating":12}]}`, cookie)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"invalid_rating"`) {
		t.Fatalf("rate 12 = %d %s", rec.Code, rec.Body)
	}
	rec = do(t, h, "POST", "/v1/ratings", `{"items":[{"item":{"type":"movie","ids":{"tmdb":27205}},"rating":8}]}`, cookie)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"added":1`) {
		t.Fatalf("rate = %d %s", rec.Code, rec.Body)
	}
	rec = do(t, h, "POST", "/v1/ratings", `{"items":[{"item":{"type":"movie","ids":{"tmdb":27205}},"rating":8}]}`, cookie)
	if !strings.Contains(rec.Body.String(), `"existing":1`) {
		t.Errorf("rate again = %s", rec.Body)
	}
	var activity Activity
	_ = json.Unmarshal(do(t, h, "GET", "/v1/activity", "", cookie).Body.Bytes(), &activity)
	if activity.Ratings.IsZero() || !activity.Latest.Equal(activity.Ratings) {
		t.Errorf("activity = %+v", activity)
	}
	var ratings []Rating
	_ = json.Unmarshal(do(t, h, "GET", "/v1/ratings?type=movie&rating=7,8", "", cookie).Body.Bytes(), &ratings)
	if len(ratings) != 1 || ratings[0].Rating != 8 {
		t.Fatalf("ratings = %+v", ratings)
	}
	path := "/v1/ratings/" + strconv.FormatInt(ratings[0].Id, 10)
	if rec := do(t, h, "DELETE", path, "", cookie); rec.Code != http.StatusNoContent {
		t.Errorf("remove = %d", rec.Code)
	}
	if rec := do(t, h, "DELETE", path, "", cookie); rec.Code != http.StatusNotFound {
		t.Errorf("remove twice = %d", rec.Code)
	}
}
