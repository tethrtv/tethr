package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/dbtest"
)

func TestRuleListEndpoints(t *testing.T) {
	mail := &fakeMail{}
	s := &Server{DB: dbtest.New(t), Mail: mail, AppURL: "https://staging.tethr.tv", AllowedOrigins: []string{"https://staging.tethr.tv"}, Cookie: CookieConfig{Name: "tethr_session"}}
	h := s.Handler()
	cookie := signIn(t, h, mail)
	ctx := context.Background()
	ids := map[string]int64{}
	for _, w := range []core.WikidataTitle{
		{QID: "Q171048", Name: "Toy Story", Year: 1995, Sitelinks: 120},
		{QID: "Q174811", Name: "Up", Year: 2009, Sitelinks: 90},
		{QID: "Q36479", Name: "The Lion King", Year: 1994, Sitelinks: 110},
	} {
		res, err := core.SeedTitle(ctx, s.DB, "movie", w, 0, false)
		if err != nil {
			t.Fatal(err)
		}
		ids[w.Name] = res.Title
	}
	for _, sql := range []string{
		fmt.Sprintf(`UPDATE titles SET studios = '{Q127552}', genres = '{comedy}', type_flags = '{animated}', runtime_minutes = 81 WHERE id = %d`, ids["Toy Story"]),
		fmt.Sprintf(`UPDATE titles SET studios = '{Q127552}', genres = '{adventure}', type_flags = '{animated}' WHERE id = %d`, ids["Up"]),
		fmt.Sprintf(`UPDATE titles SET studios = '{Q1047410}', genres = '{drama}', type_flags = '{animated}' WHERE id = %d`, ids["The Lion King"]),
		`INSERT INTO wikidata_classes (qid, label) VALUES ('Q127552', 'Pixar'), ('Q1047410', 'Walt Disney Animation Studios')`,
	} {
		if _, err := s.DB.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}

	rec := do(t, h, "GET", "/v1/lists/preview?type=animated&studio=Q127552&studio=Q1047410&not_genre=drama", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("preview = %d %s", rec.Code, rec.Body)
	}
	var preview RulePreview
	_ = json.Unmarshal(rec.Body.Bytes(), &preview)
	if preview.MatchedCount != 2 || len(preview.Items) != 2 || preview.Items[0].Id != ids["Toy Story"] || preview.Items[1].Rank != 2 {
		t.Errorf("preview = %s", rec.Body)
	}
	if rec := do(t, h, "GET", "/v1/lists/preview?studio=Pixar", ""); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `Pixar\" isn’t one of the studios`) {
		t.Errorf("a studio name instead of an ID = %d %s", rec.Code, rec.Body)
	}

	rec = do(t, h, "GET", "/v1/companies?q=pix", "")
	if rec.Code != http.StatusOK || rec.Body.String() != `[{"name":"Pixar","network_of":0,"studio_of":2,"wikidata":"Q127552"}]`+"\n" {
		t.Errorf("companies = %d %s", rec.Code, rec.Body)
	}

	rec = do(t, h, "POST", "/v1/lists", `{"name":"Pixar","rule":{"studios":{"any":["Q127552"]},"runtime":{"to":100}}}`, cookie)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body)
	}
	var list List
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	path := "/v1/lists/" + strconv.FormatInt(list.Id, 10)
	if list.Rule == nil || list.ItemCount != 1 || list.MatchedCount == nil || *list.MatchedCount != 1 ||
		!strings.Contains(rec.Body.String(), `"rule":{"names":{"Q127552":"Pixar"},"runtime":{"to":100},"studios":{"any":["Q127552"]}}`) {
		t.Errorf("created = %s", rec.Body)
	}
	rec = do(t, h, "PATCH", path, `{"rule":{"studios":{"any":["Q127552"]}}}`, cookie)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"item_count":2`) {
		t.Errorf("new rule = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "GET", path+"/items", "", cookie); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"total":2`) {
		t.Errorf("items = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "POST", path+"/items", `{"items":[{"item":{"type":"movie","ids":{"tmdb":27205}}}]}`, cookie); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "rule_list") {
		t.Errorf("adding by hand = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "PATCH", path, `{"sort_by":"added"}`, cookie); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "rank, title or year") {
		t.Errorf("sorting by date added = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "DELETE", path+"/items/"+strconv.FormatInt(ids["Up"], 10), "", cookie); rec.Code != http.StatusNoContent {
		t.Errorf("taking a title out = %d %s", rec.Code, rec.Body)
	}
	rec = do(t, h, "GET", path, "", cookie)
	if !strings.Contains(rec.Body.String(), `"left_out":[`+strconv.FormatInt(ids["Up"], 10)+`]`) || !strings.Contains(rec.Body.String(), `"item_count":1`) {
		t.Errorf("after taking one out = %s", rec.Body)
	}

	rec = do(t, h, "POST", path+"/copy", ``, cookie)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"slug":"pixar-2"`) || strings.Contains(rec.Body.String(), `"rule"`) {
		t.Errorf("copy = %d %s", rec.Code, rec.Body)
	}
	byHand := do(t, h, "POST", "/v1/lists", `{"name":"By hand"}`, cookie)
	var hand List
	_ = json.Unmarshal(byHand.Body.Bytes(), &hand)
	if rec := do(t, h, "POST", "/v1/lists/"+strconv.FormatInt(hand.Id, 10)+"/copy", `{}`, cookie); rec.Code != http.StatusNotFound {
		t.Errorf("copying a hand-made list = %d %s", rec.Code, rec.Body)
	}
}
