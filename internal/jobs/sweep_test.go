package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/dbtest"
	"github.com/tethrtv/tethr/internal/wikidata"
)

func TestSweep(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	if _, err := core.LookUp(ctx, db, "movie", core.IDs{TMDB: 100}, true); err != nil {
		t.Fatal(err)
	}
	var fetched []string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"results":{"bindings":[
				{"item":{"value":"http://www.wikidata.org/entity/Q1"}},
				{"item":{"value":"http://www.wikidata.org/entity/Q2"}},
				{"item":{"value":"http://www.wikidata.org/entity/Q3"}},
				{"item":{"value":"http://www.wikidata.org/entity/Q4"}}]}}`))
			return
		}
		ids := r.URL.Query().Get("ids")
		if r.URL.Query().Get("languages") == "en" {
			out := []string{}
			for _, q := range strings.Split(ids, "|") {
				out = append(out, fmt.Sprintf(`%q:{"id":%q,"claims":{}}`, q, q))
			}
			_, _ = w.Write([]byte(`{"entities":{` + strings.Join(out, ",") + `}}`))
			return
		}
		fetched = append(fetched, ids)
		item := func(qid, label string, claims ...string) string {
			return fmt.Sprintf(`%q:{"id":%q,"labels":{"en":{"value":%q}},"claims":{%s}}`, qid, qid, label, strings.Join(claims, ","))
		}
		_, _ = w.Write([]byte(`{"entities":{` + strings.Join([]string{
			item("Q1", "Already here", stringClaim("P4947", "100"), itemClaim("P31", "Q11424")),
			item("Q2", "New Film", stringClaim("P4947", "200"), itemClaim("P31", "Q11424")),
			item("Q3", "A Franchise", stringClaim("P4947", "300"), itemClaim("P31", "Q196600")),
			`"Q4":{"id":"Q4","missing":""}`,
		}, ",") + `}}`))
	}))
	defer fake.Close()

	s := Sweep{DB: db, Wikidata: &wikidata.Client{BaseURL: fake.URL, APIURL: fake.URL}, Log: slog.New(slog.DiscardHandler)}
	if err := s.Once(ctx); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(fetched) != "[Q1|Q2|Q3|Q4]" {
		t.Errorf("fetched %v: the title made by a play has no item yet, so all four are new", fetched)
	}
	var titles []string
	rows, err := db.Query(ctx, "SELECT coalesce(wikidata_id, '-') || ' ' || coalesce(tmdb_id::text, '-') FROM titles ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		titles = append(titles, s)
	}
	if fmt.Sprint(titles) != "[Q1 100 Q2 200]" {
		t.Errorf("titles = %v: the existing title should be linked, the new film created, the franchise left out", titles)
	}
	var left []string
	rows, err = db.Query(ctx, "SELECT qid || ' ' || reason FROM wikidata_left_out ORDER BY qid")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		left = append(left, s)
	}
	if fmt.Sprint(left) != "[Q3 not one film or show Q4 deleted]" {
		t.Errorf("left out = %v", left)
	}
	if ran, err := core.JobRanSince(ctx, db, sweepJob, time.Now().Add(-time.Hour)); err != nil || !ran {
		t.Errorf("the sweep should record that it ran: %v, %v", ran, err)
	}

	fetched = nil
	if err := s.Once(ctx); err != nil || len(fetched) != 0 {
		t.Errorf("a second sweep should fetch nothing: %v, %v", fetched, err)
	}

	if _, err := db.Exec(ctx, "DELETE FROM job_runs"); err != nil {
		t.Fatal(err)
	}
	listing := 0
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		listing++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failing.Close()
	s.Wikidata = &wikidata.Client{BaseURL: failing.URL, QLeverURL: failing.URL, APIURL: failing.URL}
	if err := s.due(ctx); err == nil || listing != 2 {
		t.Errorf("a failed sweep = %v after %d requests", err, listing)
	}
	if err := s.due(ctx); err != nil || listing != 2 {
		t.Errorf("a failed sweep should wait hours before trying again, not ask every hour: %v, %d requests", err, listing)
	}
}
