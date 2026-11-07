package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tethrtv/tethr/internal/dbtest"
)

func TestInviteEndpoints(t *testing.T) {
	mail := &fakeMail{}
	s := &Server{DB: dbtest.New(t), Mail: mail, AppURL: "https://staging.tethr.tv", AllowedOrigins: []string{"https://staging.tethr.tv"}, Cookie: CookieConfig{Name: "tethr_session"}}
	h := s.Handler()
	cookie := signIn(t, h, mail)

	if rec := do(t, h, "GET", "/v1/invites", "", cookie); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"open":true`) {
		t.Errorf("invites with open sign-ups = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "POST", "/v1/invites", "", cookie); rec.Code != http.StatusConflict {
		t.Errorf("making an invite with open sign-ups = %d %s", rec.Code, rec.Body)
	}

	s.RequireInvite = true
	rec := do(t, h, "POST", "/v1/invites", "", cookie)
	var made Invite
	_ = json.Unmarshal(rec.Body.Bytes(), &made)
	if rec.Code != http.StatusCreated || made.Url != "https://staging.tethr.tv/join?invite="+made.Code {
		t.Fatalf("make = %d %s", rec.Code, rec.Body)
	}

	signedOut := httptest.NewRecorder()
	h.ServeHTTP(signedOut, httptest.NewRequest("GET", "/v1/invites/"+strings.ToLower(made.Code), nil))
	if signedOut.Code != http.StatusOK || signedOut.Body.String() != `{"invited_by":"nathan","valid":true}`+"\n" {
		t.Errorf("check = %d %s", signedOut.Code, signedOut.Body)
	}

	if rec := do(t, h, "POST", "/v1/accounts", `{"email":"sam@example.com","name":"Sam","username":"sam","invite":"`+made.Code+`"}`); rec.Code != http.StatusAccepted {
		t.Fatalf("sam joins = %d %s", rec.Code, rec.Body)
	}
	rec = do(t, h, "GET", "/v1/invites", "", cookie)
	var invites Invites
	_ = json.Unmarshal(rec.Body.Bytes(), &invites)
	if rec.Code != http.StatusOK || invites.Open || invites.Left != 2 || len(invites.Invites) != 1 || *invites.Invites[0].UsedBy != "sam" {
		t.Errorf("invites = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "GET", "/v1/invites", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("signed out = %d", rec.Code)
	}
}
