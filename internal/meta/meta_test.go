package meta

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tethrtv/tethr/internal/tmdb"
)

func server(t *testing.T) (http.Handler, *[]string) {
	t.Helper()
	var asked []string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/3/tv/1396":
			_, _ = w.Write([]byte(`{"name":"Breaking Bad","first_air_date":"2008-01-20","poster_path":"/bb.jpg","overview":"A teacher","vote_average":8.9}`))
		case "/3/movie/27205":
			_, _ = w.Write([]byte(`{"title":"Inception","release_date":"","poster_path":null}`))
		case "/3/tv/1396/season/1":
			_, _ = w.Write([]byte(`{"episodes":[{"episode_number":1,"name":"Pilot","air_date":"2008-01-20","overview":"x","still_path":"/s.jpg"},{"episode_number":2,"name":"Cat's in the Bag...","air_date":""}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(fake.Close)
	mux := http.NewServeMux()
	(&Server{TMDB: &tmdb.Client{Token: "secret", BaseURL: fake.URL}, Log: slog.New(slog.DiscardHandler)}).Routes(mux)
	return mux, &asked
}

func get(h http.Handler, path, site string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if site != "" {
		req.Header.Set("Sec-Fetch-Site", site)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestDetails(t *testing.T) {
	h, _ := server(t)

	rec := get(h, "/meta/tv/1396", "same-site")
	if rec.Code != http.StatusOK || rec.Body.String() != `{"name":"Breaking Bad","year":2008,"poster":"/bb.jpg"}`+"\n" {
		t.Errorf("show = %d %s", rec.Code, rec.Body)
	}
	if cache := rec.Header().Get("Cache-Control"); !strings.Contains(cache, "s-maxage=2592000") || rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("headers = %v", rec.Header())
	}
	if rec := get(h, "/meta/movie/27205", "same-origin"); rec.Body.String() != `{"name":"Inception","year":null,"poster":null}`+"\n" {
		t.Errorf("movie = %s", rec.Body)
	}

	rec = get(h, "/meta/tv/1396/season/1", "same-site")
	var season struct {
		Episodes []map[string]any `json:"episodes"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &season)
	if len(season.Episodes) != 2 || season.Episodes[0]["name"] != "Pilot" || season.Episodes[1]["air_date"] != nil || len(season.Episodes[0]) != 3 {
		t.Errorf("season = %s", rec.Body)
	}
	if !strings.Contains(rec.Header().Get("Cache-Control"), "s-maxage=604800") {
		t.Errorf("season cache = %s", rec.Header().Get("Cache-Control"))
	}
}

func TestOnlyOwnSite(t *testing.T) {
	h, asked := server(t)
	for _, site := range []string{"", "cross-site", "none"} {
		rec := get(h, "/meta/tv/1396", site)
		if rec.Code != http.StatusForbidden || rec.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("Sec-Fetch-Site %q = %d %v", site, rec.Code, rec.Header())
		}
	}
	if len(*asked) != 0 {
		t.Errorf("refused requests reached TMDB: %v", *asked)
	}
}

func TestNotFound(t *testing.T) {
	h, asked := server(t)
	for _, path := range []string{"/meta/tv/99", "/meta/tv/abc", "/meta/tv/1396/season/-1", "/meta/person/1", "/meta/search/tv"} {
		if rec := get(h, path, "same-site"); rec.Code != http.StatusNotFound {
			t.Errorf("%s = %d", path, rec.Code)
		}
	}
	if len(*asked) != 1 {
		t.Errorf("only the real lookup should reach TMDB: %v", *asked)
	}
}
