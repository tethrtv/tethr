package wikidata

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
			{"key":{"value":"57243"},"item":{"value":"http://www.wikidata.org/entity/Q39243484"},"name":{"value":"Doctor Who (2005)"},"original":{"value":"Doctor Who"},"year":{"value":"2005"},"runtime":{"value":"44.5"},"tvdbID":{"value":"78804"},"imdbID":{"value":"not an id"}},
			{"key":{"value":"1"},"item":{"value":"http://www.wikidata.org/entity/Q2"},"fallback":{"value":"Pride"}}]}}`))
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
	if w := found["57243"]; w.Name != "Doctor Who" || w.Year != 2005 || w.Runtime != 45 || w.TVDB != "78804" || w.IMDB != "" {
		t.Errorf("doctor who = %+v", w)
	}
	if w := found["1"]; w.Name != "Pride" || w.Year != 0 {
		t.Errorf("a title with no English label should use the default one: %+v", w)
	}
	if _, err := c.Find(context.Background(), "movie", "tvdb", []string{"1"}); err == nil {
		t.Error("films have no TVDB lookup")
	}
}
