package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/dbtest"
)

func TestFollowAndReportEndpoints(t *testing.T) {
	mail := &fakeMail{}
	s := &Server{
		DB:             dbtest.New(t),
		Mail:           mail,
		AppURL:         "https://staging.tethr.tv",
		AllowedOrigins: []string{"https://staging.tethr.tv"},
		Cookie:         CookieConfig{Name: "tethr_session"},
		ReportsTo:      "reports@example.com",
	}
	h := s.Handler()
	nathan := signIn(t, h, mail)
	if rec := do(t, h, "POST", "/v1/accounts", `{"email":"sam@example.com","name":"Sam","username":"sam"}`); rec.Code != http.StatusAccepted {
		t.Fatalf("sam signs up = %d %s", rec.Code, rec.Body)
	}
	body := mail.last(t).body
	token := body[strings.Index(body, "token=")+len("token="):]
	rec := do(t, h, "POST", "/v1/auth/verify", `{"token":"`+token[:strings.IndexAny(token, "\n ")]+`"}`)
	sam := rec.Result().Cookies()[0]

	rec = do(t, h, "POST", "/v1/lists", `{"name":"Slow films","privacy":"public"}`, nathan)
	var list List
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	id := strconv.FormatInt(list.Id, 10)
	if rec := do(t, h, "POST", "/v1/lists/"+id+"/items", `{"items":[{"item":{"type":"movie","ids":{"tmdb":27205}}}]}`, nathan); rec.Code != http.StatusOK {
		t.Fatalf("add = %d %s", rec.Code, rec.Body)
	}

	if list.Url != "https://staging.tethr.tv/@nathan/lists/slow-films" {
		t.Errorf("url = %s", list.Url)
	}
	if rec := do(t, h, "PATCH", "/v1/lists/"+id, `{"name":"Slower films"}`, nathan); rec.Code != http.StatusOK {
		t.Fatalf("rename = %d %s", rec.Code, rec.Body)
	}
	rec = do(t, h, "GET", "/v1/users/nathan/lists/slow-films", "")
	var found List
	_ = json.Unmarshal(rec.Body.Bytes(), &found)
	if rec.Code != http.StatusOK || found.Id != list.Id || found.Url != "https://staging.tethr.tv/@nathan/lists/slower-films" {
		t.Errorf("by an old address = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "GET", "/v1/users/nathan/lists/nothing", ""); rec.Code != http.StatusNotFound {
		t.Errorf("an address with no list = %d", rec.Code)
	}
	if rec := do(t, h, "PATCH", "/v1/lists/"+id, `{"name":"Slow films"}`, nathan); rec.Code != http.StatusOK {
		t.Fatalf("rename back = %d %s", rec.Code, rec.Body)
	}

	rec = do(t, h, "GET", "/v1/public-lists?order=popular&q=slow", "", sam)
	var page ListsPage
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if rec.Code != http.StatusOK || page.Total != 1 || page.Lists[0].Owner.Name != nil || page.Lists[0].Owner.Username != "nathan" || page.Limit != 20 {
		t.Fatalf("browse = %d %s", rec.Code, rec.Body)
	}

	rec = do(t, h, "PUT", "/v1/followed-lists/"+id, "", sam)
	var followed List
	_ = json.Unmarshal(rec.Body.Bytes(), &followed)
	if rec.Code != http.StatusOK || !followed.Following || followed.FollowerCount != 1 {
		t.Errorf("follow = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "PUT", "/v1/followed-lists/"+id, "", nathan); rec.Code != http.StatusConflict {
		t.Errorf("following your own list = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "GET", "/v1/followed-lists", "", sam); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"Slow films"`) {
		t.Errorf("followed lists = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "GET", "/v1/lists/"+id+"/followers", "", nathan); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"username":"sam"`) || strings.Contains(rec.Body.String(), `"name"`) {
		t.Errorf("followers = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "GET", "/v1/lists/"+id+"/followers", "", sam); rec.Code != http.StatusForbidden {
		t.Errorf("followers asked by someone else = %d %s", rec.Code, rec.Body)
	}

	signedOut := httptest.NewRecorder()
	h.ServeHTTP(signedOut, httptest.NewRequest("GET", "/v1/users/nathan", nil))
	var user User
	_ = json.Unmarshal(signedOut.Body.Bytes(), &user)
	if signedOut.Code != http.StatusOK || user.Name != nil || !user.Private || len(user.Lists) != 1 {
		t.Errorf("profile page signed out = %d %s", signedOut.Code, signedOut.Body)
	}
	if rec := do(t, h, "GET", "/v1/users/nobody", "", sam); rec.Code != http.StatusNotFound {
		t.Errorf("no such user = %d", rec.Code)
	}

	if rec := do(t, h, "POST", "/v1/lists/"+id+"/reports", `{"reason":"rude"}`, sam); rec.Code != http.StatusBadRequest {
		t.Errorf("bad reason = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "POST", "/v1/lists/"+id+"/reports", `{"reason":"spam","note":"A scam link"}`, sam); rec.Code != http.StatusNoContent {
		t.Fatalf("report = %d %s", rec.Code, rec.Body)
	}
	sent := mail.last(t)
	if sent.to != "reports@example.com" || !strings.Contains(sent.body, `sam reported "Slow films" by nathan for spam`) || !strings.Contains(sent.body, "tethr report keep "+id) {
		t.Errorf("report email = %+v", sent)
	}
	if rec := do(t, h, "GET", "/v1/public-lists", "", sam); !strings.Contains(rec.Body.String(), `"total":0`) {
		t.Errorf("browse after a report = %s", rec.Body)
	}
	if rec := do(t, h, "POST", "/v1/lists/"+id+"/reports", `{"reason":"spam"}`, nathan); rec.Code != http.StatusNotFound {
		t.Errorf("reporting your own list = %d", rec.Code)
	}

	removed, err := core.RemoveList(t.Context(), s.DB, list.Id)
	if err != nil {
		t.Fatal(err)
	}
	m, err := RemovedListEmail(s.AppURL, removed, "it links to a scam")
	if err != nil || m.To != "nathan@example.com" || !strings.Contains(m.Text, "breaks Tethr’s rules: it links to a scam") || !strings.Contains(m.HTML, "https://staging.tethr.tv/terms") {
		t.Errorf("removal email = %+v, %v", m, err)
	}

	if rec := do(t, h, "DELETE", "/v1/followed-lists/"+id, "", sam); rec.Code != http.StatusNoContent {
		t.Errorf("unfollow = %d %s", rec.Code, rec.Body)
	}
}
