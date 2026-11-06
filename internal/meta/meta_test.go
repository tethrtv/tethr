package meta

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
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
			if r.URL.Query().Get("language") != "en-GB" {
				t.Errorf("language = %q", r.URL.Query().Get("language"))
			}
			_, _ = w.Write([]byte(`{"name":"Breaking Bad","first_air_date":"2008-01-20","poster_path":"/bb.jpg","overview":"A teacher","vote_average":8.94,"vote_count":16000,
				"genres":[{"id":18,"name":"Drama"}],"episode_run_time":[],"last_episode_to_air":{"runtime":47},"networks":[{"name":"AMC"}],"created_by":[{"name":"Vince Gilligan"}],
				"status":"Returning Series","seasons":[{"season_number":0,"episode_count":2},{"season_number":1,"episode_count":7}],"next_episode_to_air":{"season_number":6,"episode_number":1,"name":"Next","air_date":"2026-12-01"},
				"aggregate_credits":{"cast":[{"name":"A"},{"name":"B"},{"name":"C"},{"name":"D"},{"name":"E"},{"name":"F"}]},
				"content_ratings":{"results":[{"iso_3166_1":"US","rating":"TV-MA"},{"iso_3166_1":"GB","rating":"18"}]}}`))
		case "/3/movie/27205":
			_, _ = w.Write([]byte(`{"title":"Inception","release_date":"","poster_path":null,"tagline":"Your mind","runtime":148,"vote_average":9.5,"vote_count":4,
				"credits":{"cast":[{"name":"Leonardo DiCaprio"}],"crew":[{"name":"Christopher Nolan","job":"Director"},{"name":"Hans Zimmer","job":"Original Music Composer"}]},
				"release_dates":{"results":[{"iso_3166_1":"US","release_dates":[{"certification":"PG-13"}]},{"iso_3166_1":"FR","release_dates":[{"certification":"U"}]}]}}`))
		case "/3/search/multi":
			if r.URL.Query().Get("query") != "bad b" || r.URL.Query().Get("include_adult") != "false" {
				t.Errorf("search query = %s", r.URL.RawQuery)
			}
			var results []string
			for i := range 12 {
				results = append(results, fmt.Sprintf(`{"media_type":"tv","id":%d,"name":"Show %d","first_air_date":"2008-01-20","poster_path":"/p.jpg"}`, i+1, i+1))
			}
			_, _ = fmt.Fprintf(w, `{"results":[{"media_type":"person","id":9,"name":"Someone"},{"media_type":"movie","id":27205,"title":"Inception","release_date":""},%s]}`, strings.Join(results, ","))
		case "/3/tv/1396/season/1":
			_, _ = w.Write([]byte(`{"overview":"Season one","episodes":[{"episode_number":1,"name":"Pilot","air_date":"2008-01-20","overview":"x","runtime":58,"still_path":"/s.jpg"},{"episode_number":2,"name":"Cat's in the Bag...","air_date":""}]}`))
		case "/3/tv/1396/episode_groups":
			_, _ = w.Write([]byte(`{"results":[{"id":"69f50757054263b7bc87e32a","name":"DVD / PVOD","type":3,"group_count":6,"episode_count":62,"description":"x"}]}`))
		case "/3/tv/episode_group/69f50757054263b7bc87e32a":
			_, _ = w.Write([]byte(`{"id":"69f50757054263b7bc87e32a","name":"DVD / PVOD","type":3,"groups":[
				{"order":6,"name":"Season 6","episodes":[{"id":5,"show_id":1396,"season_number":5,"episode_number":10,"order":1,"name":"Buried","air_date":""},{"id":4,"show_id":1396,"season_number":5,"episode_number":9,"order":0,"name":"Blood Money","air_date":"2013-08-11","runtime":47}]},
				{"order":0,"name":"Specials","episodes":[]}]}`))
		case "/3/tv/episode_group/aaaaaaaaaaaaaaaaaaaaaaaa":
			_, _ = w.Write([]byte(`{"id":"aaaaaaaaaaaaaaaaaaaaaaaa","groups":[{"order":1,"episodes":[{"id":9,"show_id":1399,"season_number":1,"episode_number":1,"order":0}]}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(fake.Close)
	mux := http.NewServeMux()
	signedIn := func(next func(http.ResponseWriter, *http.Request, int64)) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, err := strconv.ParseInt(r.Header.Get("Account"), 10, 64)
			if err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			next(w, r, id)
		})
	}
	(&Server{TMDB: &tmdb.Client{Token: "secret", BaseURL: fake.URL}, Log: slog.New(slog.DiscardHandler), SignedIn: signedIn}).Routes(mux)
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
	want := `{"name":"Breaking Bad","year":2008,"poster":"/bb.jpg","overview":"A teacher","genres":["Drama"],"runtime":47,"certificate":{"country":"GB","rating":"18"},"score":8.9,` +
		`"creators":["Vince Gilligan"],"cast":["A","B","C","D","E"],"networks":["AMC"],"status":"Returning Series","next_episode":{"season":6,"number":1,"name":"Next","air_date":"2026-12-01"},` +
		`"seasons":[{"number":0,"episodes":2},{"number":1,"episodes":7}]}` + "\n"
	if rec.Code != http.StatusOK || rec.Body.String() != want {
		t.Errorf("show = %d %s", rec.Code, rec.Body)
	}
	if cache := rec.Header().Get("Cache-Control"); !strings.Contains(cache, "s-maxage=86400") || rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("headers = %v", rec.Header())
	}
	rec = get(h, "/meta/movie/27205", "same-origin")
	want = `{"name":"Inception","year":null,"poster":null,"tagline":"Your mind","runtime":148,"certificate":{"country":"US","rating":"PG-13"},"creators":["Christopher Nolan"],"cast":["Leonardo DiCaprio"]}` + "\n"
	if rec.Body.String() != want {
		t.Errorf("movie = %s", rec.Body)
	}
	if cache := rec.Header().Get("Cache-Control"); !strings.Contains(cache, "s-maxage=2592000") {
		t.Errorf("movie cache = %s", cache)
	}

	rec = get(h, "/meta/tv/1396/season/1", "same-site")
	var season struct {
		Overview string           `json:"overview"`
		Episodes []map[string]any `json:"episodes"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &season)
	if season.Overview != "Season one" || len(season.Episodes) != 2 || season.Episodes[0]["overview"] != "x" || season.Episodes[0]["runtime"] != 58.0 ||
		season.Episodes[1]["air_date"] != nil || len(season.Episodes[0]) != 5 {
		t.Errorf("season = %s", rec.Body)
	}
	if !strings.Contains(rec.Header().Get("Cache-Control"), "s-maxage=604800") {
		t.Errorf("season cache = %s", rec.Header().Get("Cache-Control"))
	}
}

func TestOrders(t *testing.T) {
	h, _ := server(t)
	rec := get(h, "/meta/tv/1396/orders", "same-site")
	if want := `{"orders":[{"id":"69f50757054263b7bc87e32a","name":"DVD / PVOD","type":3,"seasons":6,"episodes":62}]}` + "\n"; rec.Code != http.StatusOK || rec.Body.String() != want {
		t.Errorf("orders = %d %s", rec.Code, rec.Body)
	}
	rec = get(h, "/meta/tv/1396/order/69f50757054263b7bc87e32a", "same-site")
	want := `{"name":"DVD / PVOD","seasons":[{"number":0,"name":"Specials","episodes":[]},{"number":6,"name":"Season 6","episodes":[` +
		`{"number":1,"aired":{"season":5,"number":9},"name":"Blood Money","air_date":"2013-08-11","runtime":47},` +
		`{"number":2,"aired":{"season":5,"number":10},"name":"Buried","air_date":null}]}],"type":3}` + "\n"
	if rec.Code != http.StatusOK || rec.Body.String() != want {
		t.Errorf("order = %d %s", rec.Code, rec.Body)
	}
	for _, path := range []string{"/meta/tv/1396/order/aaaaaaaaaaaaaaaaaaaaaaaa", "/meta/tv/1396/order/..%2Fsecret"} {
		if rec := get(h, path, "same-site"); rec.Code != http.StatusNotFound {
			t.Errorf("%s = %d", path, rec.Code)
		}
	}
}

func TestSearch(t *testing.T) {
	h, asked := server(t)
	search := func(q, account string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/meta/search?q="+url.QueryEscape(q), nil)
		if account != "" {
			req.Header.Set("Account", account)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	if rec := search("bad b", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("signed out = %d", rec.Code)
	}
	if rec := search(" b ", "1"); rec.Code != http.StatusOK || rec.Body.String() != `{"results":[]}`+"\n" || len(*asked) != 0 {
		t.Errorf("one letter = %d %s, asked %v", rec.Code, rec.Body, *asked)
	}

	rec := search(" bad b ", "1")
	var found struct {
		Results []map[string]any `json:"results"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &found)
	if rec.Code != http.StatusOK || len(found.Results) != 10 {
		t.Fatalf("search = %d %s", rec.Code, rec.Body)
	}
	if r := found.Results[0]; r["type"] != "movie" || r["tmdb"] != 27205.0 || r["year"] != nil || r["poster"] != nil {
		t.Errorf("film = %v", r)
	}
	if r := found.Results[1]; r["type"] != "show" || r["name"] != "Show 1" || r["year"] != 2008.0 || r["poster"] != "/p.jpg" {
		t.Errorf("show = %v", r)
	}
	if cache := rec.Header().Get("Cache-Control"); !strings.HasPrefix(cache, "private") {
		t.Errorf("search cache = %s", cache)
	}

	for range searchLimit.Burst - 1 {
		search("bad b", "1")
	}
	before := len(*asked)
	if rec := search("bad b", "1"); rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "2" || len(*asked) != before {
		t.Errorf("over the limit = %d %v", rec.Code, rec.Header())
	}
	if rec := search("bad b", "2"); rec.Code != http.StatusOK {
		t.Errorf("another account = %d", rec.Code)
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
