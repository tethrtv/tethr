package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/dbtest"
	"github.com/tethrtv/tethr/internal/wikidata"
)

func TestRefreshTitles(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	for _, ref := range []struct {
		kind string
		ids  core.IDs
	}{
		{"movie", core.IDs{TMDB: 603}},
		{"movie", core.IDs{IMDB: "tt0245429"}},
		{"show", core.IDs{TMDB: 1396, IMDB: "tt0903747"}},
		{"show", core.IDs{TMDB: 404}},
	} {
		if _, err := core.LookUp(ctx, db, ref.kind, ref.ids, true); err != nil {
			t.Fatal(err)
		}
	}

	var queries, fetches, classFetches []string
	parent := func(q string) []any {
		return []any{map[string]any{"rank": "normal", "mainsnak": map[string]any{"snaktype": "value", "datavalue": map[string]any{"value": map[string]string{"id": q}}}}}
	}
	classClaims := map[string]any{
		"Q188473":  map[string]any{"P279": parent("Q1762165")},
		"Q30":      map[string]any{"P297": []any{map[string]any{"rank": "normal", "mainsnak": map[string]any{"snaktype": "value", "datavalue": map[string]any{"value": "US"}}}}},
		"Q1762165": map[string]any{"P279": parent("Q7725634")},
	}
	matrix := "Q83495"
	busy := true
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if busy {
			queries = append(queries, "busy")
			w.Header().Set("Retry-After", "600")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		if r.Method == http.MethodGet && r.URL.Query().Get("languages") == "en" {
			ids := r.URL.Query().Get("ids")
			classFetches = append(classFetches, ids)
			out := map[string]any{}
			for _, q := range strings.Split(ids, "|") {
				out[q] = map[string]any{"id": q, "claims": classClaims[q]}
			}
			body, _ := json.Marshal(map[string]any{"entities": out})
			_, _ = w.Write(body)
			return
		}
		if r.Method == http.MethodGet {
			ids := r.URL.Query().Get("ids")
			fetches = append(fetches, ids)
			w.Header().Set("Content-Type", "application/json")
			switch ids {
			case "Q83495|Q155653":
				_, _ = w.Write([]byte(`{"entities":{
					"Q83495":{"id":"Q83495","labels":{"mul":{"value":"The Matrix"}},"claims":{
						"P1476":[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":{"text":"The Matrix","language":"en"}}}}],
						"P577":[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":{"time":"+1999-03-31T00:00:00Z","precision":11}}}}],
						"P4947":[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":"603"}}}],
						"P345":[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":"tt0133093"}}}],
						"P31":[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":{"id":"Q11424"}}}}],
						"P136":[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":{"id":"Q188473"}}}}],
						"P495":[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":{"id":"Q30"}}}}]}},
					"Q155653":{"id":"Q155653","labels":{"en":{"value":"Spirited Away"}},"claims":{
						"P1476":[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":{"text":"千と千尋の神隠し","language":"ja"}}}}],
						"P577":[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":{"time":"+2001-07-20T00:00:00Z","precision":11}}}}],
						"P4947":[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":"129"}}}],
						"P345":[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":"tt0245429"}}}]}}}}`))
			case "Q1079":
				_, _ = w.Write([]byte(`{"entities":{
					"Q1079":{"id":"Q1079","labels":{"en":{"value":"Breaking Bad"}},"claims":{
						"P580":[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":{"time":"+2008-01-20T00:00:00Z","precision":11}}}}],
						"P4983":[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":"1396"}}}],
						"P345":[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":"tt9999999"}}}],
						"P4835":[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":"81189"}}}]}}}}`))
			case "Q83495":
				_, _ = w.Write([]byte(`{"entities":{"Q83495":{"id":"Q83495","missing":""}}}`))
			case "Q999":
				_, _ = w.Write([]byte(`{"entities":{"Q999":{"id":"Q999","labels":{"en":{"value":"The Matrix"}},"claims":{}}}}`))
			default:
				w.WriteHeader(http.StatusBadRequest)
			}
			return
		}
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		query := form.Get("query")
		queries = append(queries, query)
		w.Header().Set("Content-Type", "application/sparql-results+json")
		switch {
		case strings.Contains(query, `VALUES ?key { "603" }`):
			_, _ = w.Write([]byte(`{"results":{"bindings":[
				{"key":{"value":"603"},"item":{"value":"http://www.wikidata.org/entity/` + matrix + `"}},
				{"key":{"value":"603"},"item":{"value":"http://www.wikidata.org/entity/Q130000000"}}]}}`))
		case strings.Contains(query, `VALUES ?key { "tt0245429" }`):
			_, _ = w.Write([]byte(`{"results":{"bindings":[{"key":{"value":"tt0245429"},"item":{"value":"http://www.wikidata.org/entity/Q155653"}}]}}`))
		case strings.Contains(query, `VALUES ?key { "1396" "404" }`):
			_, _ = w.Write([]byte(`{"results":{"bindings":[{"key":{"value":"1396"},"item":{"value":"http://www.wikidata.org/entity/Q1079"}}]}}`))
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer fake.Close()

	job := Titles{DB: db, Wikidata: &wikidata.Client{BaseURL: fake.URL, APIURL: fake.URL}, Log: slog.New(slog.DiscardHandler)}
	if err := job.Refresh(ctx); !errors.Is(err, wikidata.ErrBusy) || len(queries) != 1 {
		t.Errorf("a busy Wikidata should stop the run after one request: %v, %v", queries, err)
	}
	busy, queries = false, nil
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if len(fetches) != 2 {
		t.Errorf("one fetch per kind, got %v", fetches)
	}
	if fmt.Sprint(classFetches) != "[Q11424|Q188473|Q30 Q1762165 Q7725634]" {
		t.Errorf("classes should be fetched once each, parents after children: %v", classFetches)
	}
	var genres, countries []string
	var version *string
	if err := db.QueryRow(ctx, "SELECT genres, countries, facts_version FROM titles WHERE tmdb_id = 603").Scan(&genres, &countries, &version); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(genres, countries) != "[action] [US]" || version == nil {
		t.Errorf("the matrix's facts = %v %v, version %v", genres, countries, version)
	}
	if len(queries) != 3 {
		t.Errorf("one query per kind and ID type, got %d", len(queries))
	}

	items, err := core.LookUp(ctx, db, "movie", core.IDs{TMDB: 603}, false)
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
	m := items[0].Title
	if *m.Name != "The Matrix" || *m.OriginalName != "The Matrix" || *m.Year != 1999 || *m.WikidataID != "Q83495" || m.Imdb != "tt0133093" {
		t.Errorf("the matrix = %+v", m)
	}

	items, err = core.LookUp(ctx, db, "movie", core.IDs{TMDB: 129}, false)
	if err != nil || len(items) != 1 {
		t.Fatalf("a title found by IMDb should learn its TMDB ID: %v, %v", items, err)
	}
	if s := items[0].Title; *s.Name != "Spirited Away" || *s.OriginalName != "千と千尋の神隠し" {
		t.Errorf("spirited away = %+v", s)
	}

	items, err = core.LookUp(ctx, db, "show", core.IDs{TMDB: 1396}, false)
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
	if b := items[0].Title; *b.Name != "Breaking Bad" || b.OriginalName != nil || b.Imdb != "tt9999999" || b.Tvdb != "81189" {
		t.Errorf("Wikidata's ID should win over one an app sent: %+v", b)
	}

	items, err = core.LookUp(ctx, db, "show", core.IDs{TMDB: 404}, false)
	if err != nil || len(items) != 1 || items[0].Title.Name != nil {
		t.Errorf("a show Wikidata doesn't have should stay unnamed: %+v, %v", items, err)
	}

	queries, fetches = nil, nil
	if err := job.Refresh(ctx); err != nil || len(queries) != 0 || len(fetches) != 0 {
		t.Errorf("titles just checked shouldn't be asked for again: %d queries, %d fetches, %v", len(queries), len(fetches), err)
	}

	if _, err := db.Exec(ctx, `UPDATE titles SET wikidata_checked_at = now() - interval '91 days' WHERE wikidata_id = 'Q83495'`); err != nil {
		t.Fatal(err)
	}
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if len(queries) != 0 || len(fetches) != 1 || fetches[0] != "Q83495" {
		t.Errorf("a title with an item should be fetched by it, not looked up again: %v, %v", queries, fetches)
	}
	items, err = core.LookUp(ctx, db, "movie", core.IDs{TMDB: 603}, false)
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
	var flagged bool
	if err := db.QueryRow(ctx, `SELECT wikidata_deleted_at IS NOT NULL FROM titles WHERE id = $1`, items[0].Title.ID).Scan(&flagged); err != nil {
		t.Fatal(err)
	}
	if m := items[0].Title; !flagged || *m.Name != "The Matrix" || *m.WikidataID != "Q83495" {
		t.Errorf("a deleted item should be flagged and its title kept as it was: %v, %+v", flagged, m)
	}

	matrix = "Q999"
	queries, fetches = nil, nil
	if _, err := db.Exec(ctx, `UPDATE titles SET wikidata_checked_at = now() - interval '91 days' WHERE wikidata_id = 'Q83495'`); err != nil {
		t.Fatal(err)
	}
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if len(queries) != 1 || len(fetches) != 1 || fetches[0] != "Q999" {
		t.Errorf("a title whose item was deleted should be matched again: %v, %v", queries, fetches)
	}
	if err := db.QueryRow(ctx, `SELECT wikidata_deleted_at IS NOT NULL FROM titles WHERE id = $1`, items[0].Title.ID).Scan(&flagged); err != nil || flagged {
		t.Errorf("a new item should clear the flag: %v", err)
	}
}
