package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/dbtest"
)

func TestReportTitle(t *testing.T) {
	mail := &fakeMail{}
	s := &Server{DB: dbtest.New(t), Mail: mail, AppURL: "https://staging.tethr.tv", AllowedOrigins: []string{"https://staging.tethr.tv"}, Cookie: CookieConfig{Name: "tethr_session"}}
	h := s.Handler()
	cookie := signIn(t, h, mail)
	res, err := core.SeedTitle(context.Background(), s.DB, "movie", core.WikidataTitle{QID: "Q1", Name: "Akira", Year: 1988, TMDB: 149}, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	reports := "/v1/titles/" + strconv.FormatInt(res.Title, 10) + "/reports"

	var kid Profile
	_ = json.Unmarshal(do(t, h, "POST", "/v1/profiles", `{"name":"Ollie","kind":"kids"}`, cookie).Body.Bytes(), &kid)

	for _, c := range []struct {
		path, body string
		code       int
		problem    string
	}{
		{reports, `{"reason":"name","suggestion":"AKIRA","note":"The poster says so"}`, http.StatusNoContent, ""},
		{reports, `{"reason":"plot"}`, http.StatusBadRequest, ""},
		{reports, `{"reason":"year","suggestion":"soon"}`, http.StatusBadRequest, "invalid_suggestion"},
		{reports, `{"reason":"year","suggestion":"1988"}`, http.StatusBadRequest, "same_as_now"},
		{reports, `{"reason":"link","provider":"tvmaze"}`, http.StatusBadRequest, "invalid_provider"},
		{reports, `{"reason":"duplicate","suggestion":"https://www.imdb.com/title/tt9999999/"}`, http.StatusBadRequest, "unknown_duplicate"},
		{reports, `{"reason":"duplicate","suggestion":"Q1"}`, http.StatusBadRequest, "same_title"},
		{reports + "?profile_id=" + strconv.FormatInt(kid.Id, 10), `{"reason":"name"}`, http.StatusForbidden, "adults_only"},
		{"/v1/titles/999999/reports", `{"reason":"name"}`, http.StatusNotFound, ""},
	} {
		rec := do(t, h, "POST", c.path, c.body, cookie)
		if rec.Code != c.code || !strings.Contains(rec.Body.String(), c.problem) {
			t.Errorf("%s %s = %d %s", c.path, c.body, rec.Code, rec.Body)
		}
	}
	if rec := do(t, h, "POST", reports, `{"reason":"name"}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("signed out = %d %s", rec.Code, rec.Body)
	}
}
