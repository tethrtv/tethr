package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tethrtv/tethr/internal/dbtest"
	"github.com/tethrtv/tethr/internal/wikidata"
)

func TestFollowFeed(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	start := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	if _, err := db.Exec(ctx, `
INSERT INTO titles (id, kind, slug, wikidata_id) SELECT n, 'movie', 't-' || n, 'Q83495' FROM nextval('catalogue_id') AS n;
INSERT INTO titles (id, kind, slug, wikidata_id) SELECT n, 'show', 't-' || n, 'Q1079' FROM nextval('catalogue_id') AS n;
INSERT INTO wikidata_classes (qid, label) VALUES ('Q11424', 'film')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE wikidata_feed SET cursor = $1`, start); err != nil {
		t.Fatal(err)
	}

	at := func(d time.Duration) string { return start.Add(d).Format(time.RFC3339) }
	var starts []string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		starts = append(starts, q.Get("rcstart"))
		if q.Get("rccontinue") == "" {
			_, _ = fmt.Fprintf(w, `{"continue":{"rccontinue":"next"},"query":{"recentchanges":[
				{"type":"edit","title":"Q83495","timestamp":%q,"comment":"/* wbsetdescription-add:1|fr */ film"},
				{"type":"edit","title":"Q83495","timestamp":%q,"user":"~2026-1-1","temp":"","comment":"/* wbsetlabel-set:1|en */ The Matrix"},
				{"type":"edit","title":"Q999","timestamp":%q,"comment":"/* wbsetlabel-set:1|en */ Not ours"}
			]}}`, at(time.Minute), at(2*time.Minute), at(3*time.Minute))
			return
		}
		_, _ = fmt.Fprintf(w, `{"query":{"recentchanges":[
			{"type":"edit","title":"Q83495","timestamp":%q,"comment":"/* wbsetclaim-update:2||1 */ [[Property:P577]]: 1999"},
			{"type":"edit","title":"Q11424","timestamp":%q,"comment":"/* wbsetclaim-create:2||1 */ [[Property:P279]]: [[Q2431196]]"},
			{"type":"edit","title":"Q1079","timestamp":%q,"comment":"/* wbsetaliases-add:1|en */ BB"}
		]}}`, at(4*time.Minute), at(5*time.Minute), at(6*time.Minute))
	}))
	defer fake.Close()

	f := Feed{DB: db, Wikidata: &wikidata.Client{APIURL: fake.URL, BaseURL: fake.URL, QLeverURL: fake.URL}, Log: slog.New(slog.DiscardHandler)}
	if err := f.Follow(ctx); err != nil {
		t.Fatal(err)
	}
	pending := map[string]time.Time{}
	rows, err := db.Query(ctx, `SELECT qid, not_before FROM wikidata_pending`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var q string
		var nb time.Time
		if err := rows.Scan(&q, &nb); err != nil {
			t.Fatal(err)
		}
		pending[q] = nb
	}
	loggedOut := map[string]bool{}
	lrows, err := db.Query(ctx, `SELECT qid, logged_out FROM wikidata_pending`)
	if err != nil {
		t.Fatal(err)
	}
	for lrows.Next() {
		var q string
		var l bool
		if err := lrows.Scan(&q, &l); err != nil {
			t.Fatal(err)
		}
		loggedOut[q] = l
	}
	if !loggedOut["Q83495"] || loggedOut["Q11424"] {
		t.Errorf("logged out = %v, want The Matrix's relevant edit marked as made logged out", loggedOut)
	}
	if len(pending) != 2 || !pending["Q83495"].Equal(start.Add(2*time.Minute+24*time.Hour)) || !pending["Q11424"].Equal(start.Add(5*time.Minute+24*time.Hour)) {
		t.Errorf("pending = %v, want The Matrix a day after its first relevant edit and the film class", pending)
	}
	var cursor time.Time
	if err := db.QueryRow(ctx, `SELECT cursor FROM wikidata_feed`).Scan(&cursor); err != nil {
		t.Fatal(err)
	}
	if time.Since(cursor) > 6*time.Minute || time.Since(cursor) < 4*time.Minute {
		t.Errorf("cursor = %v, want it a few minutes behind now so late changes aren't missed", cursor)
	}

	starts = nil
	if err := f.Follow(ctx); err != nil {
		t.Fatal(err)
	}
	if len(starts) != 0 {
		t.Errorf("a second follow within the minute asked for %v", starts)
	}
}

func TestFollowFeedAfterAGap(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	if _, err := db.Exec(ctx, `
INSERT INTO titles (id, kind, slug, wikidata_id) SELECT n, 'movie', 't-' || n, 'Q83495' FROM nextval('catalogue_id') AS n;
INSERT INTO titles (id, kind, slug, wikidata_id) SELECT n, 'show', 't-' || n, 'Q1079' FROM nextval('catalogue_id') AS n;
INSERT INTO wikidata_classes (qid, label) VALUES ('Q11424', 'film');
UPDATE wikidata_feed SET cursor = now() - interval '40 days'`); err != nil {
		t.Fatal(err)
	}
	oldest := time.Now().Add(-30 * 24 * time.Hour).UTC().Format(time.RFC3339)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `{"query":{"recentchanges":[{"type":"edit","title":"Q5","timestamp":%q,"comment":""}]}}`, oldest)
	}))
	defer fake.Close()

	f := Feed{DB: db, Wikidata: &wikidata.Client{APIURL: fake.URL, BaseURL: fake.URL, QLeverURL: fake.URL}, Log: slog.New(slog.DiscardHandler)}
	if err := f.Follow(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM wikidata_pending WHERE not_before <= now()`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("%d items queued after the feed lost edits, want every title's item and class", n)
	}
}
