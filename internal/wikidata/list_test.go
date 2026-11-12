package wikidata

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestListTitles(t *testing.T) {
	var asked []string
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, "wikidata")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer broken.Close()
	qlever := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		q := form.Get("query")
		asked = append(asked, "qlever")
		if !strings.HasPrefix(q, "PREFIX wd:") {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if strings.Contains(q, "VALUES ?class { wd:Q11424 wd:Q5398426 }") {
			_, _ = w.Write([]byte(`{"results":{"bindings":[{"item":{"value":"http://www.wikidata.org/entity/Q3"}},{"item":{"value":"http://www.wikidata.org/entity/Q1"}}]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"results":{"bindings":[{"item":{"value":"http://www.wikidata.org/entity/Q1"}},{"item":{"value":"http://www.wikidata.org/entity/Q2"}},{"item":{"value":"nonsense"}}]}}`))
	}))
	defer qlever.Close()

	c := &Client{BaseURL: broken.URL, QLeverURL: qlever.URL}
	got, fellBack, err := c.ListTitles(context.Background(), []string{"Q11424", "Q5398426", "not a qid"})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(got) != "[Q1 Q2 Q3]" || fellBack == nil {
		t.Errorf("listed %v, fell back %v", got, fellBack)
	}
	if fmt.Sprint(asked) != "[wikidata qlever wikidata qlever]" {
		t.Errorf("asked %v: each query should try wikidata once, then qlever", asked)
	}

	c.QLeverURL = broken.URL
	if _, _, err := c.ListTitles(context.Background(), nil); err == nil || errors.Is(err, ErrBusy) {
		t.Errorf("both services failing should be an error, not a busy one: %v", err)
	}

	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":{"bindings":[]}}`))
	}))
	defer empty.Close()
	asked = nil
	c = &Client{BaseURL: empty.URL, QLeverURL: qlever.URL}
	if got, fellBack, err := c.ListTitles(context.Background(), []string{"Q11424", "Q5398426"}); err != nil || fellBack == nil || fmt.Sprint(got) != "[Q1 Q2]" {
		t.Errorf("an empty list of titles should count as a failure and fall back: %v, %v, %v", got, fellBack, err)
	}
}

func TestFetchItems(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"entities":{
			"Q1":{"id":"Q2","labels":{"en":{"value":"Moved"}},"claims":{"P4947":[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":"7"}}}]}},
			"Q3":{"id":"Q3","missing":""},
			"Q4":{"id":"Q4","claims":{"P31":"not a list"}}}}`))
	}))
	defer fake.Close()
	c := &Client{APIURL: fake.URL}
	items, deleted, unreadable, err := c.FetchItems(context.Background(), []string{"Q1", "Q3", "Q4", "Q5"})
	if err != nil {
		t.Fatal(err)
	}
	if it, ok := items["Q1"]; !ok || it.QID() != "Q2" || fmt.Sprint(it.TMDBKinds()) != "[movie]" {
		t.Errorf("a redirected item should come back as its target: %+v", items)
	}
	if fmt.Sprint(deleted, unreadable) != "[Q3] [Q4 Q5]" {
		t.Errorf("deleted %v, unreadable or missing %v", deleted, unreadable)
	}
}
