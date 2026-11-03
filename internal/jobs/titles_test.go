package jobs

import (
	"context"
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

	var queries []string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		query := form.Get("query")
		queries = append(queries, query)
		w.Header().Set("Content-Type", "application/sparql-results+json")
		switch {
		case strings.Contains(query, `VALUES ?key { "603" }`):
			_, _ = w.Write([]byte(`{"results":{"bindings":[
				{"key":{"value":"603"},"item":{"value":"http://www.wikidata.org/entity/Q83495"},"name":{"value":"The Matrix"},"original":{"value":"The Matrix"},"year":{"value":"1999"},"tmdbID":{"value":"603"},"imdbID":{"value":"tt0133093"}},
				{"key":{"value":"603"},"item":{"value":"http://www.wikidata.org/entity/Q130000000"},"name":{"value":"The Matrix (duplicate)"}}]}}`))
		case strings.Contains(query, `VALUES ?key { "tt0245429" }`):
			_, _ = w.Write([]byte(`{"results":{"bindings":[
				{"key":{"value":"tt0245429"},"item":{"value":"http://www.wikidata.org/entity/Q155653"},"name":{"value":"Spirited Away"},"original":{"value":"千と千尋の神隠し"},"year":{"value":"2001"},"tmdbID":{"value":"129"},"imdbID":{"value":"tt0245429"}}]}}`))
		case strings.Contains(query, `VALUES ?key { "1396" "404" }`):
			_, _ = w.Write([]byte(`{"results":{"bindings":[
				{"key":{"value":"1396"},"item":{"value":"http://www.wikidata.org/entity/Q1079"},"name":{"value":"Breaking Bad"},"year":{"value":"2008"},"tmdbID":{"value":"1396"},"imdbID":{"value":"tt9999999"},"tvdbID":{"value":"81189"}}]}}`))
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer fake.Close()

	job := Titles{DB: db, Wikidata: &wikidata.Client{BaseURL: fake.URL}, Log: slog.New(slog.DiscardHandler)}
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
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
	if b := items[0].Title; *b.Name != "Breaking Bad" || b.OriginalName != nil || b.Imdb != "tt0903747" || b.Tvdb != "81189" {
		t.Errorf("an ID the app sent should win over Wikidata's: %+v", b)
	}

	items, err = core.LookUp(ctx, db, "show", core.IDs{TMDB: 404}, false)
	if err != nil || len(items) != 1 || items[0].Title.Name != nil {
		t.Errorf("a show Wikidata doesn't have should stay unnamed: %+v, %v", items, err)
	}

	queries = nil
	if err := job.Refresh(ctx); err != nil || len(queries) != 0 {
		t.Errorf("titles just checked shouldn't be asked for again: %d queries, %v", len(queries), err)
	}
}
