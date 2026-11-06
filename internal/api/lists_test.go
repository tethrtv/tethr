package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestListEndpoints(t *testing.T) {
	h, mail := newTestServer(t)
	cookie := signIn(t, h, mail)

	rec := do(t, h, "POST", "/v1/lists", `{"name":"Friday films","privacy":"public","description":"Short ones"}`, cookie)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body)
	}
	var list List
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	path := "/v1/lists/" + strconv.FormatInt(list.Id, 10)
	if list.Access != ListAccessMine || list.Owner.Username != "nathan" || list.Url != "https://staging.tethr.tv/lists/"+strconv.FormatInt(list.Id, 10)+"-friday-films" || *list.WatchedCount != 0 {
		t.Errorf("created = %+v", list)
	}
	if rec := do(t, h, "POST", "/v1/lists", `{"name":""}`, cookie); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_name") {
		t.Errorf("no name = %d %s", rec.Code, rec.Body)
	}

	rec = do(t, h, "POST", "/v1/profiles", `{"name":"Ruby","kind":"kids"}`, cookie)
	var ruby Profile
	_ = json.Unmarshal(rec.Body.Bytes(), &ruby)
	asRuby := "?profile_id=" + strconv.FormatInt(ruby.Id, 10)
	if rec := do(t, h, "POST", "/v1/lists"+asRuby, `{"name":"Mine","privacy":"public"}`, cookie); rec.Code != http.StatusForbidden {
		t.Errorf("kids making a public list = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "PATCH", path+asRuby, `{"name":"Taken"}`, cookie); rec.Code != http.StatusNotFound {
		t.Errorf("changing someone else's list = %d %s", rec.Code, rec.Body)
	}
	rec = do(t, h, "GET", "/v1/lists"+asRuby, "", cookie)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"access":"household"`) {
		t.Errorf("household lists = %d %s", rec.Code, rec.Body)
	}

	rec = do(t, h, "POST", path+"/items", `{"items":[{"item":{"type":"movie","ids":{"tmdb":27205}},"notes":"Twice"},{"item":{"type":"show","ids":{"tmdb":1396}}}]}`, cookie)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"added":2`) {
		t.Fatalf("add = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "GET", "/v1/titles/tmdb-27205?type=movie", "", cookie); !strings.Contains(rec.Body.String(), `"lists":[{"item_id":`) ||
		!strings.Contains(rec.Body.String(), `"list_id":`+strconv.FormatInt(list.Id, 10)) {
		t.Errorf("title's lists = %s", rec.Body)
	}
	if rec := do(t, h, "POST", "/v1/history", `{"items":[{"item":{"type":"movie","ids":{"tmdb":27205}}}]}`, cookie); rec.Code != http.StatusOK {
		t.Fatalf("watch = %d %s", rec.Code, rec.Body)
	}

	rec = do(t, h, "GET", path+"/items", "", cookie)
	var page ListItemsPage
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	tag := rec.Header().Get("ETag")
	if rec.Code != http.StatusOK || tag == "" || page.Total != 2 || *page.List.WatchedCount != 1 || page.Items[0].Progress.Watched != 1 || *page.Items[0].Notes != "Twice" {
		t.Fatalf("items = %d %s", rec.Code, rec.Body)
	}
	film, show := page.Items[0].Id, page.Items[1].Id

	if rec := do(t, h, "GET", path+"/items?hide_watched=true", "", cookie); !strings.Contains(rec.Body.String(), `"total":1`) {
		t.Errorf("hiding watched = %s", rec.Body)
	}

	anyone := func(target, ifNoneMatch string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", target, nil)
		if ifNoneMatch != "" {
			req.Header.Set("If-None-Match", ifNoneMatch)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	rec = anyone(path+"/items", "")
	signedOutTag := rec.Header().Get("ETag")
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "progress") || strings.Contains(rec.Body.String(), "watched_count") {
		t.Errorf("signed out = %d %s", rec.Code, rec.Body)
	}
	if rec := anyone(path+"/items", signedOutTag); rec.Code != http.StatusNotModified {
		t.Errorf("same ETag = %d", rec.Code)
	}
	if rec := anyone(path, signedOutTag); rec.Code != http.StatusNotModified {
		t.Errorf("same ETag on the list = %d", rec.Code)
	}

	if rec := do(t, h, "PUT", path+"/order", `{"ids":[`+strconv.FormatInt(show, 10)+`]}`, cookie); rec.Code != http.StatusBadRequest {
		t.Errorf("order missing an item = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "PUT", path+"/order", `{"ids":[`+strconv.FormatInt(show, 10)+`,`+strconv.FormatInt(film, 10)+`]}`, cookie); rec.Code != http.StatusNoContent {
		t.Errorf("reorder = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "PATCH", path, `{"cover_item_id":`+strconv.FormatInt(show, 10)+`}`, cookie); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"cover_item_id":`+strconv.FormatInt(show, 10)) {
		t.Errorf("pick a cover = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "PATCH", path, `{"cover_item_id":1}`, cookie); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_cover") {
		t.Errorf("cover not on the list = %d %s", rec.Code, rec.Body)
	}
	var signedOut List
	_ = json.Unmarshal(anyone(path, "").Body.Bytes(), &signedOut)
	if len(signedOut.Cover) != 1 || signedOut.Cover[0].Type != ItemTypeShow || *signedOut.Cover[0].Title.Ids.Tmdb != 1396 {
		t.Errorf("cover signed out = %+v", signedOut.Cover)
	}
	if rec := do(t, h, "PATCH", path+"/items/"+strconv.FormatInt(show, 10), `{"notes":"From the start"}`, cookie); rec.Code != http.StatusNoContent {
		t.Errorf("notes = %d %s", rec.Code, rec.Body)
	}
	if rec := anyone(path+"/items", signedOutTag); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "From the start") {
		t.Errorf("after a change = %d %s", rec.Code, rec.Body)
	}

	if rec := do(t, h, "PATCH", path, `{"privacy":"household"}`, cookie); rec.Code != http.StatusOK {
		t.Errorf("make household = %d %s", rec.Code, rec.Body)
	}
	if rec := anyone(path, ""); rec.Code != http.StatusNotFound {
		t.Errorf("signed out reading a household list = %d", rec.Code)
	}
	if rec := bearer(t, h, "GET", path, "", "not-a-token"); rec.Code != http.StatusUnauthorized {
		t.Errorf("a bad token = %d", rec.Code)
	}

	if rec := do(t, h, "DELETE", path+"/items/"+strconv.FormatInt(film, 10), "", cookie); rec.Code != http.StatusNoContent {
		t.Errorf("remove = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "POST", path+"/items/remove", `{"ids":[`+strconv.FormatInt(show, 10)+`,`+strconv.FormatInt(film, 10)+`]}`, cookie); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"removed":1`) {
		t.Errorf("remove several = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "GET", "/v1/activity", "", cookie); !strings.Contains(rec.Body.String(), `"lists":`) {
		t.Errorf("activity = %s", rec.Body)
	}
	if rec := do(t, h, "DELETE", path, "", cookie); rec.Code != http.StatusNoContent {
		t.Errorf("delete = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "GET", path, "", cookie); rec.Code != http.StatusNotFound {
		t.Errorf("after deleting = %d", rec.Code)
	}
}
