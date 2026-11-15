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

func TestCatalogueSearchAndMatch(t *testing.T) {
	mail := &fakeMail{}
	s := &Server{DB: dbtest.New(t), Mail: mail, AppURL: "https://staging.tethr.tv", AllowedOrigins: []string{"https://staging.tethr.tv"}, Cookie: CookieConfig{Name: "tethr_session"}}
	h := s.Handler()
	cookie := signIn(t, h, mail)
	res, err := core.SeedTitle(context.Background(), s.DB, "movie", core.WikidataTitle{QID: "Q1", Name: "Amélie", Year: 2001, Sitelinks: 70}, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	amelie := strconv.FormatInt(res.Title, 10)

	if rec := do(t, h, "GET", "/v1/search?q=amelie", "", cookie); rec.Code != http.StatusOK || rec.Body.String() != "[]\n" {
		t.Errorf("library search = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "GET", "/v1/search?q=amelie&scope=catalogue", "", cookie); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"tethr":`+amelie) {
		t.Errorf("catalogue search = %d %s", rec.Code, rec.Body)
	}

	if rec := do(t, h, "GET", "/v1/search?q=amelie&scope=everything", "", cookie); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown scope = %d %s", rec.Code, rec.Body)
	}

	rec := do(t, h, "POST", "/v1/match", `{"films":[{"name":"Amelie","year":2001},{"name":"Nothing like it"}]}`, cookie)
	var body struct {
		Results []FilmMatch `json:"results"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusOK || len(body.Results) != 2 || body.Results[0].Match == nil || body.Results[1].Match != nil || body.Results[1].Candidates == nil {
		t.Errorf("match = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "POST", "/v1/match", `{"films":[]}`, cookie); rec.Code != http.StatusBadRequest {
		t.Errorf("no films = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "POST", "/v1/match", `{"films":[{"name":"Amelie"}]}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("signed out = %d %s", rec.Code, rec.Body)
	}

	imported := do(t, h, "POST", "/v1/imports?filename=letterboxd.zip", `{"source":"letterboxd","history":[{"type":"movie","tethr":`+amelie+`,"label":"Amélie (2001)","at":"2024-01-02T00:00:00Z","precision":"day"}]}`, cookie)
	if imported.Code != http.StatusCreated || !strings.Contains(imported.Body.String(), `"history":{"added":0,"skipped":0,"total":1,"unmatched":0}`) {
		t.Errorf("import by tethr id = %d %s", imported.Code, imported.Body)
	}
}
