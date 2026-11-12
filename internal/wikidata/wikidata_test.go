package wikidata

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestFind(t *testing.T) {
	var sent string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		sent = string(body)
		if r.Header.Get("User-Agent") != "Tethr/test" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(`{"results":{"bindings":[
			{"key":{"value":"57243"},"item":{"value":"http://www.wikidata.org/entity/Q130000000"}},
			{"key":{"value":"57243"},"item":{"value":"http://www.wikidata.org/entity/Q39243484"}},
			{"key":{"value":"1"},"item":{"value":"http://www.wikidata.org/entity/Q2"}}]}}`))
	}))
	defer fake.Close()

	c := &Client{BaseURL: fake.URL, UserAgent: "Tethr/test"}
	found, err := c.Find(context.Background(), "show", "tmdb", []string{"57243", "1", `1" } ?x ?y ?z . { "`})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sent, "%3Fz") {
		t.Errorf("a value that isn't an ID reached the query: %s", sent)
	}
	if found["57243"] != "Q39243484" || found["1"] != "Q2" {
		t.Errorf("found = %v, want the older of two items claiming one ID", found)
	}
	if _, err := c.Find(context.Background(), "movie", "tvdb", []string{"1"}); err == nil {
		t.Error("films have no TVDB lookup")
	}
}

func claim(rank string, value any) map[string]any {
	return map[string]any{"rank": rank, "mainsnak": map[string]any{"snaktype": "value", "datavalue": map[string]any{"value": value}}}
}

func date(t string, precision int) map[string]any {
	return map[string]any{"time": t, "precision": precision}
}

func entities(t *testing.T) []byte {
	labels := func(pairs ...string) map[string]any {
		out := map[string]any{}
		for i := 0; i < len(pairs); i += 2 {
			out[pairs[i]] = map[string]string{"language": pairs[i], "value": pairs[i+1]}
		}
		return out
	}
	body, err := json.Marshal(map[string]any{"entities": map[string]any{
		"Q39243484": map[string]any{"id": "Q39243484", "labels": labels("en", "Doctor Who (2005 TV series)", "fr", "Doctor Who"), "claims": map[string]any{
			"P1476": []any{claim("normal", map[string]string{"text": "Doctor Who", "language": "en"})},
			"P580":  []any{claim("normal", date("+2005-03-26T00:00:00Z", 11))},
			"P577":  []any{claim("deprecated", date("+1999-01-01T00:00:00Z", 11))},
			"P2047": []any{claim("normal", map[string]string{"amount": "+44.5", "unit": "http://www.wikidata.org/entity/Q7727"}), claim("normal", map[string]string{"amount": "+3", "unit": "http://www.wikidata.org/entity/Q1"})},
			"P4983": []any{claim("normal", "57243")},
			"P4835": []any{claim("normal", "78804")},
			"P345":  []any{claim("normal", "not an id")},
			"P9751": []any{claim("normal", "umc.cmc.4iygvvnqsxa8knd287saief60")},
			"P9586": []any{claim("normal", "umc.cmc.1zcw5qvtqaplltm533oeemyd7")},
		}},
		"Q83495": map[string]any{"id": "Q83495", "labels": labels("mul", "The Matrix", "de", "Matrix"), "claims": map[string]any{
			"P577":   []any{claim("normal", date("+1999-03-31T00:00:00Z", 11)), claim("normal", date("+1999-06-17T00:00:00Z", 11))},
			"P4947":  []any{claim("normal", "999"), claim("preferred", "603")},
			"P345":   []any{claim("normal", "tt0133093")},
			"P6127":  []any{claim("normal", "the-matrix")},
			"P8013":  []any{claim("normal", "movies/the-matrix-1999")},
			"P11460": []any{claim("normal", "5d776825880197001ec967c6"), claim("normal", "5d776825880197001ec967c5"), claim("normal", "5d776825880197001ec967c5")},
			"P2704":  []any{claim("normal", "10.5240/0000-0000-0000-0000-0000-X"), claim("preferred", "10.5240/B1D4-4D0E-9E3B-6D12-1B4C-Q")},
			"P1712":  []any{claim("normal", "movie/the-matrix"), claim("normal", "tv/the-matrix/season-1/episode-1")},
			"P8600":  []any{claim("normal", "123")},
		}},
		"Q1": map[string]any{"id": "Q2", "redirects": map[string]string{"from": "Q1", "to": "Q2"}, "labels": labels("fr", "Pride"), "claims": map[string]any{
			"P577": []any{claim("normal", date("+2014-00-00T00:00:00Z", 9))},
		}},
		"Q3":         map[string]any{"id": "Q3", "labels": labels("ja", "千と千尋の神隠し", "zh", "千与千寻"), "claims": map[string]any{"P577": []any{claim("normal", date("+2001-07-00T00:00:00Z", 10))}}},
		"Q137784704": map[string]any{"id": "Q137784704", "missing": ""},
		"Q5":         map[string]any{"id": "Q5", "labels": labels("en", "Broken"), "claims": map[string]any{"P577": "not a list"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestFetch(t *testing.T) {
	calls := 0
	var asked []string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		asked = append(asked, r.URL.Query().Get("ids"))
		if r.URL.Query().Get("maxlag") != "60" || r.Header.Get("User-Agent") != "Tethr/test" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if calls == 1 {
			w.Header().Set("Retry-After", "1")
			_, _ = w.Write([]byte(`{"error":{"code":"maxlag","info":"Waiting for a database server"}}`))
			return
		}
		_, _ = w.Write(entities(t))
	}))
	defer fake.Close()

	c := &Client{APIURL: fake.URL, UserAgent: "Tethr/test"}
	ctx := context.Background()
	found, deleted, err := c.Fetch(ctx, "show", []string{"Q39243484", "Q1", "Q3", "Q137784704", "Q5", "Q39243484", "nonsense"})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("a lagged answer should be asked again once, got %d calls", calls)
	}
	if asked[1] != "Q39243484|Q1|Q3|Q137784704|Q5" {
		t.Errorf("asked for %q", asked[1])
	}
	if len(deleted) != 1 || deleted[0] != "Q137784704" {
		t.Errorf("deleted = %v", deleted)
	}
	if _, ok := found["Q5"]; ok {
		t.Error("an item whose claims can't be read should be left out, not saved empty")
	}
	if w := found["Q39243484"]; w.Name != "Doctor Who" || w.Original != "Doctor Who" || w.Year != 2005 || !w.Released.Equal(time.Date(2005, 3, 26, 0, 0, 0, 0, time.UTC)) || w.Runtime != 45 || w.TMDB != 57243 || !reflect.DeepEqual(w.IDs, map[string][]string{"tvdb": {"78804"}, "appletv": {"umc.cmc.4iygvvnqsxa8knd287saief60"}}) {
		t.Errorf("doctor who = %+v", w)
	}
	if w := found["Q1"]; w.QID != "Q2" || w.Name != "Pride" || w.Year != 2014 || !w.Released.Equal(time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("a redirect should come back as its target, released once its year is over: %+v", w)
	}
	if w := found["Q3"]; w.Name != "千と千尋の神隠し" || !w.Released.Equal(time.Date(2001, 8, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("with no English, mul or original title, any language will do, released once its month is over: %+v", w)
	}

	calls = 1
	found, _, err = c.Fetch(ctx, "movie", []string{"Q83495"})
	if err != nil {
		t.Fatal(err)
	}
	if w := found["Q83495"]; w.Name != "The Matrix" || w.Year != 1999 || !w.Released.Equal(time.Date(1999, 3, 31, 0, 0, 0, 0, time.UTC)) || w.TMDB != 603 {
		t.Errorf("the matrix = %+v", w)
	}
	if ids := found["Q83495"].IDs; !reflect.DeepEqual(ids, map[string][]string{
		"imdb":       {"tt0133093"},
		"letterboxd": {"the-matrix"},
		"trakt_slug": {"movies/the-matrix-1999"},
		"plex":       {"5d776825880197001ec967c5", "5d776825880197001ec967c6"},
		"eidr":       {"10.5240/B1D4-4D0E-9E3B-6D12-1B4C-Q"},
		"metacritic": {"movie/the-matrix"},
	}) {
		t.Errorf("every outside ID, best rank only, no episode pages and no show IDs on a film: %v", ids)
	}

	found, deleted, err = c.Fetch(ctx, "movie", []string{"Q404"})
	if err != nil || len(found) != 0 || len(deleted) != 0 {
		t.Errorf("an item left out of the answer is neither found nor deleted: %v, %v, %v", found, deleted, err)
	}
}

func TestBusy(t *testing.T) {
	calls := 0
	retryAfter := "1"
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Retry-After", retryAfter)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer fake.Close()

	c := &Client{APIURL: fake.URL, BaseURL: fake.URL}
	ctx := context.Background()
	if _, _, err := c.Fetch(ctx, "movie", []string{"Q83495"}); !errors.Is(err, ErrBusy) || calls != 3 {
		t.Errorf("should give up after three tries: %d calls, %v", calls, err)
	}

	calls = 0
	if _, err := c.Find(ctx, "movie", "tmdb", []string{"603"}); !errors.Is(err, ErrBusy) || calls != 3 {
		t.Errorf("queries should be asked again too: %d calls, %v", calls, err)
	}

	calls, retryAfter = 0, "600"
	if _, _, err := c.Fetch(ctx, "movie", []string{"Q83495"}); !errors.Is(err, ErrBusy) || calls != 1 {
		t.Errorf("a long Retry-After should give up straight away rather than ask early: %d calls, %v", calls, err)
	}
}

func TestReleaseDate(t *testing.T) {
	for _, c := range []struct {
		time      string
		precision int
		year      int
		released  time.Time
		ok        bool
	}{
		{"+2027-00-00T00:00:00Z", 9, 2027, time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC), true},
		{"+2026-12-00T00:00:00Z", 10, 2026, time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), true},
		{"+2026-11-11T00:00:00Z", 11, 2026, time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC), true},
		{"+2026-11-00T00:00:00Z", 11, 2026, time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), true},
		{"+1990-00-00T00:00:00Z", 8, 0, time.Time{}, false},
		{"-0044-03-15T00:00:00Z", 11, 0, time.Time{}, false},
		{"+1700-01-01T00:00:00Z", 11, 0, time.Time{}, false},
		{"nonsense", 11, 0, time.Time{}, false},
	} {
		raw, _ := json.Marshal(date(c.time, c.precision))
		year, released, ok := releaseDate(raw)
		if year != c.year || !released.Equal(c.released) || ok != c.ok {
			t.Errorf("%s at %d = %d, %s, %v", c.time, c.precision, year, released, ok)
		}
	}
}
