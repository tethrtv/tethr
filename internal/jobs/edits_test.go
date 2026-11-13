package jobs

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/dbtest"
	"github.com/tethrtv/tethr/internal/wikidata"
)

type fakeWikidata struct {
	mu      sync.Mutex
	items   map[string]string
	classes map[string]string
	fetched []string
}

func (f *fakeWikidata) set(qid, label string, claims ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items[qid] = fmt.Sprintf(`{"id":%q,"labels":{"en":{"value":%q}},"claims":{%s}}`, qid, label, strings.Join(claims, ","))
}

func (f *fakeWikidata) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ids := r.URL.Query().Get("ids")
	classes := r.URL.Query().Get("languages") == "en"
	if !classes {
		f.fetched = append(f.fetched, ids)
	}
	var out []string
	for _, q := range strings.Split(ids, "|") {
		e, ok := f.items[q]
		if classes {
			e, ok = f.classes[q]
			if !ok {
				e, ok = fmt.Sprintf(`{"id":%q,"claims":{}}`, q), true
			}
		}
		if !ok {
			e = fmt.Sprintf(`{"id":%q,"missing":""}`, q)
		}
		out = append(out, fmt.Sprintf("%q:%s", q, e))
	}
	_, _ = w.Write([]byte(`{"entities":{` + strings.Join(out, ",") + `}}`))
}

func editsSetup(t *testing.T) (*pgxpool.Pool, *fakeWikidata, Titles) {
	t.Helper()
	db := dbtest.New(t)
	fake := &fakeWikidata{items: map[string]string{}, classes: map[string]string{}}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	job := Titles{DB: db, Wikidata: &wikidata.Client{BaseURL: srv.URL, QLeverURL: srv.URL, APIURL: srv.URL}, Log: slog.New(slog.DiscardHandler)}
	if err := core.SaveClasses(context.Background(), db, []core.WikidataClass{{QID: "Q11424", Label: "film"}, {QID: "Q5398426", Label: "television series"}}); err != nil {
		t.Fatal(err)
	}
	if err := core.MarkJobRun(context.Background(), db, refreshJob); err != nil {
		t.Fatal(err)
	}
	return db, fake, job
}

func seedTitle(t *testing.T, db *pgxpool.Pool, kind string, w core.WikidataTitle) int64 {
	t.Helper()
	res, err := core.SeedTitle(context.Background(), db, kind, w, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	return res.Title
}

func edited(t *testing.T, db *pgxpool.Pool, qids ...string) {
	t.Helper()
	edits := map[string]core.Edits{}
	at := time.Now().Add(-25 * time.Hour)
	for _, q := range qids {
		edits[q] = core.Edits{First: at, Last: at}
	}
	cursor, err := core.FeedCursor(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := core.QueueWikidataEdits(context.Background(), db, edits, cursor); err != nil {
		t.Fatal(err)
	}
}

func aWeekLater(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	if _, err := db.Exec(context.Background(), `UPDATE wikidata_pending SET not_before = not_before - interval '7 days', edited_at = edited_at - interval '7 days', last_edited_at = last_edited_at - interval '7 days'`); err != nil {
		t.Fatal(err)
	}
}

func pendingItems(t *testing.T, db *pgxpool.Pool) []string {
	t.Helper()
	rows, err := db.Query(context.Background(), `SELECT qid FROM wikidata_pending ORDER BY qid`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var q string
		if err := rows.Scan(&q); err != nil {
			t.Fatal(err)
		}
		out = append(out, q)
	}
	return out
}

type titleRow struct {
	Name    string
	Year    int
	Runtime *int
	Deleted bool
}

func titleNow(t *testing.T, db *pgxpool.Pool, id int64) titleRow {
	t.Helper()
	var r titleRow
	if err := db.QueryRow(context.Background(), `SELECT coalesce(name, ''), coalesce(year, 0), runtime_minutes, wikidata_deleted_at IS NOT NULL FROM titles WHERE id = $1`, id).
		Scan(&r.Name, &r.Year, &r.Runtime, &r.Deleted); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRefreshEditedItems(t *testing.T) {
	db, fake, job := editsSetup(t)
	ctx := context.Background()
	film := itemClaim("P31", "Q11424")
	heat := seedTitle(t, db, "movie", core.WikidataTitle{QID: "Q1", Name: "Heat", Year: 1995, TMDB: 949, Sitelinks: 50, InstanceOf: []string{"Q11424"}})
	gone := seedTitle(t, db, "movie", core.WikidataTitle{QID: "Q7", Name: "Gone", Year: 2001, TMDB: 7, InstanceOf: []string{"Q11424"}})
	later := seedTitle(t, db, "movie", core.WikidataTitle{QID: "Q8", Name: "Later", Year: 2002, TMDB: 8, InstanceOf: []string{"Q11424"}})
	fake.set("Q1", "Heat", film, stringClaim("P4947", "949"), dateClaim("P577", "+1995-12-15T00:00:00Z"), `"P2047":[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":{"amount":"+170","unit":"http://www.wikidata.org/entity/Q7727"}}}}]`)
	fake.set("Q8", "Later", film, stringClaim("P4947", "8"))
	odd := seedTitle(t, db, "movie", core.WikidataTitle{QID: "Q11", Name: "Odd", Year: 2003, TMDB: 11, InstanceOf: []string{"Q11424"}})
	fake.items["Q11"] = `{"id":"Q11","labels":{"en":{"value":"Odd"}},"claims":{"P31":"not a list"}}`
	if _, err := db.Exec(ctx, `UPDATE titles SET wikidata_checked_at = now() - interval '100 days' WHERE id = $1`, odd); err != nil {
		t.Fatal(err)
	}

	edited(t, db, "Q1", "Q7", "Q11")
	if _, err := core.QueueWikidataEdits(ctx, db, map[string]core.Edits{"Q8": {First: time.Now(), Last: time.Now()}}, time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(fake.fetched) != "[Q1|Q11|Q7]" {
		t.Errorf("fetched %v, want only the items whose day has passed", fake.fetched)
	}
	if h := titleNow(t, db, heat); h.Runtime == nil || *h.Runtime != 170 || h.Name != "Heat" {
		t.Errorf("heat after its edit = %+v", h)
	}
	if g := titleNow(t, db, gone); !g.Deleted || g.Name != "Gone" {
		t.Errorf("a deleted item's title should be flagged and kept: %+v", g)
	}
	if l := titleNow(t, db, later); l.Name != "Later" || l.Runtime != nil {
		t.Errorf("an item still in its day shouldn't change: %+v", l)
	}
	var checked time.Time
	if err := db.QueryRow(ctx, `SELECT wikidata_checked_at FROM titles WHERE id = $1`, odd).Scan(&checked); err != nil || time.Since(checked) > time.Minute {
		t.Errorf("an unreadable item's title should count as checked, or the safety pass keeps picking it: %v, %v", checked, err)
	}
	if p := pendingItems(t, db); fmt.Sprint(p) != "[Q8]" {
		t.Errorf("pending = %v", p)
	}
}

func TestFilmAndShowItemIsNotAMove(t *testing.T) {
	db, fake, job := editsSetup(t)
	ctx := context.Background()
	w := core.WikidataTitle{QID: "Q12", Name: "War and Peace", Year: 1965, IDs: map[string][]string{"imdb": {"tt0063794"}}, InstanceOf: []string{"Q11424"}}
	w.TMDB = 12
	seedTitle(t, db, "movie", w)
	w.TMDB = 1212
	seedTitle(t, db, "show", w)
	fake.set("Q12", "War and Peace", itemClaim("P31", "Q11424"), stringClaim("P4947", "12"), stringClaim("P4983", "1212"), stringClaim("P345", "tt0063794"))
	edited(t, db, "Q12")
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if p := pendingItems(t, db); len(p) != 0 {
		t.Errorf("an ID shared by one item's film and show was held as a move: %v", p)
	}
}

func TestOutsideIDHoldsAddUp(t *testing.T) {
	db, fake, job := editsSetup(t)
	ctx := context.Background()
	film := itemClaim("P31", "Q11424")
	seedTitle(t, db, "movie", core.WikidataTitle{QID: "Q13", Name: "Taker", Year: 2001, TMDB: 13, InstanceOf: []string{"Q11424"}})
	seedTitle(t, db, "movie", core.WikidataTitle{QID: "Q14", Name: "Keeper", Year: 2002, TMDB: 14, IDs: map[string][]string{"imdb": {"tt0000014"}, "letterboxd": {"keeper"}}, InstanceOf: []string{"Q11424"}})
	fake.set("Q13", "Taker", film, stringClaim("P4947", "13"), stringClaim("P345", "tt0000014"))
	fake.set("Q14", "Keeper", film, stringClaim("P4947", "14"), stringClaim("P345", "tt0000014"), stringClaim("P6127", "keeper"))
	edited(t, db, "Q13")
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	fake.set("Q13", "Taker", film, stringClaim("P4947", "13"), stringClaim("P345", "tt0000014"), stringClaim("P6127", "keeper"))
	aWeekLater(t, db)
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	var held string
	if err := db.QueryRow(ctx, `SELECT held::text FROM wikidata_pending WHERE qid = 'Q13'`).Scan(&held); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(held, "imdb tt0000014") || !strings.Contains(held, "letterboxd keeper") {
		t.Errorf("held = %s, want both IDs so the first isn't held again next week", held)
	}
	aWeekLater(t, db)
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if p := pendingItems(t, db); slices.Contains(p, "Q13") {
		t.Errorf("IDs both items still claim after their week should stop holding the item: %v", p)
	}
}

func TestEditDuringARefetch(t *testing.T) {
	db, fake, job := editsSetup(t)
	ctx := context.Background()
	seedTitle(t, db, "movie", core.WikidataTitle{QID: "Q1", Name: "Heat", Year: 1995, TMDB: 949, InstanceOf: []string{"Q11424"}})
	fake.set("Q1", "Heat", itemClaim("P31", "Q11424"), stringClaim("P4947", "949"))
	edited(t, db, "Q1")
	if _, err := db.Exec(ctx, `UPDATE wikidata_pending SET last_edited_at = now() + interval '1 minute'`); err != nil {
		t.Fatal(err)
	}
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	var due time.Time
	if err := db.QueryRow(ctx, `SELECT not_before FROM wikidata_pending WHERE qid = 'Q1'`).Scan(&due); err != nil {
		t.Fatalf("an edit made after the fetch should stay queued: %v", err)
	}
	if time.Until(due) < 23*time.Hour {
		t.Errorf("the later edit is due %v, want a day after it", due)
	}
}

func TestSuspiciousEditsWait(t *testing.T) {
	db, fake, job := editsSetup(t)
	ctx := context.Background()
	film := itemClaim("P31", "Q11424")
	saw := seedTitle(t, db, "movie", core.WikidataTitle{QID: "Q2", Name: "Saw", Year: 2004, TMDB: 176, InstanceOf: []string{"Q11424"}})
	karate := seedTitle(t, db, "show", core.WikidataTitle{QID: "Q3", Name: "Karate kid leyends", Year: 2023, Sitelinks: 40, InstanceOf: []string{"Q5398426"}})
	swap := seedTitle(t, db, "movie", core.WikidataTitle{QID: "Q4", Name: "Swapped", Year: 2010, TMDB: 44, InstanceOf: []string{"Q11424"}})
	fake.set("Q2", "Saw", film, stringClaim("P4947", "176"), dateClaim("P577", "+2007-01-01T00:00:00Z"))
	fake.set("Q3", "American Born Chinese", itemClaim("P31", "Q5398426"), dateClaim("P580", "+2023-05-24T00:00:00Z"))
	fake.set("Q4", "Swapped", itemClaim("P31", "Q5398426"), stringClaim("P4983", "4444"), dateClaim("P580", "+2010-01-01T00:00:00Z"))
	seedTitle(t, db, "movie", core.WikidataTitle{QID: "Q15", Name: "Newsreel 68", Year: 1944, InstanceOf: []string{"Q11424"}})
	fake.set("Q15", "Newsreel 68", itemClaim("P31", "Q21191270"))

	edited(t, db, "Q2", "Q3", "Q4", "Q15")
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if s, k := titleNow(t, db, saw), titleNow(t, db, karate); s.Year != 2004 || k.Name != "Karate kid leyends" {
		t.Errorf("suspicious edits applied at once: %+v, %+v", s, k)
	}
	if p := pendingItems(t, db); fmt.Sprint(p) != "[Q15 Q2 Q3 Q4]" {
		t.Errorf("held = %v", p)
	}
	fake.fetched = nil
	if err := job.Refresh(ctx); err != nil || len(fake.fetched) != 0 {
		t.Errorf("held items were fetched again within the week: %v, %v", fake.fetched, err)
	}

	fake.set("Q3", "Karate kid leyends", itemClaim("P31", "Q5398426"), dateClaim("P580", "+2023-05-24T00:00:00Z"))
	aWeekLater(t, db)
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if s := titleNow(t, db, saw); s.Year != 2007 {
		t.Errorf("a year Wikidata still gives after a week should apply: %+v", s)
	}
	if k := titleNow(t, db, karate); k.Name != "Karate kid leyends" {
		t.Errorf("a rename undone within the week: %+v", k)
	}
	if s := titleNow(t, db, swap); s.Name != "Swapped" {
		t.Errorf("the old film should be kept as it was: %+v", s)
	}
	var shows int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM titles WHERE kind = 'show' AND wikidata_id = 'Q4' AND tmdb_id = 4444`).Scan(&shows); err != nil || shows != 1 {
		t.Errorf("a film that became a show should gain a show title: %d, %v", shows, err)
	}
	if p := pendingItems(t, db); len(p) != 0 {
		t.Errorf("still pending after the week: %v", p)
	}
}

func TestOutsideIDMovesBetweenTitles(t *testing.T) {
	db, fake, job := editsSetup(t)
	ctx := context.Background()
	film := itemClaim("P31", "Q11424")
	one := seedTitle(t, db, "movie", core.WikidataTitle{QID: "Q5", Name: "One", Year: 2001, TMDB: 5, IDs: map[string][]string{"imdb": {"tt0000005"}}, InstanceOf: []string{"Q11424"}})
	other := seedTitle(t, db, "movie", core.WikidataTitle{QID: "Q6", Name: "Other", Year: 2002, TMDB: 6, IDs: map[string][]string{"imdb": {"tt0000006"}}, InstanceOf: []string{"Q11424"}})
	fake.set("Q5", "One", film, stringClaim("P4947", "5"), stringClaim("P345", "tt0000006"))
	fake.set("Q6", "Other", film, stringClaim("P4947", "6"))
	owner := func() int64 {
		t.Helper()
		var id int64
		if err := db.QueryRow(ctx, `SELECT title_id FROM external_ids WHERE provider = 'imdb' AND value = 'tt0000006'`).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}

	edited(t, db, "Q5")
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if owner() != other {
		t.Error("an outside ID moved between titles at once")
	}
	if p := pendingItems(t, db); fmt.Sprint(p) != "[Q5 Q6]" {
		t.Errorf("pending = %v, want the other title's item queued too", p)
	}
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(fake.fetched) != "[Q5 Q6]" {
		t.Errorf("fetched %v, want the other title's item looked at next", fake.fetched)
	}
	if p := pendingItems(t, db); fmt.Sprint(p) != "[Q5]" {
		t.Errorf("pending = %v", p)
	}
	aWeekLater(t, db)
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if owner() != one {
		t.Error("an outside ID Wikidata still moves after a week should move")
	}
}

func TestClassEdits(t *testing.T) {
	db, fake, job := editsSetup(t)
	ctx := context.Background()
	if err := core.SaveClasses(ctx, db, []core.WikidataClass{{QID: "Q20", Label: "noir thriller"}}); err != nil {
		t.Fatal(err)
	}
	noir := seedTitle(t, db, "movie", core.WikidataTitle{QID: "Q9", Name: "Noir", Year: 1950, TMDB: 9, InstanceOf: []string{"Q11424"}, Genres: []string{"Q20"}})
	plain := seedTitle(t, db, "movie", core.WikidataTitle{QID: "Q10", Name: "Plain", Year: 1951, TMDB: 10, InstanceOf: []string{"Q11424"}})
	fake.classes["Q20"] = `{"id":"Q20","labels":{"en":{"value":"noir thriller"}},"claims":{` + itemClaim("P279", "Q182015") + `}}`
	fake.classes["Q182015"] = `{"id":"Q182015","labels":{"en":{"value":"thriller film"}},"claims":{}}`

	fake.classes["Q21"] = `{"id":"Q21","claims":{"P279":"not a list"}}`
	if _, err := db.Exec(ctx, `INSERT INTO wikidata_classes (qid, label, checked_at) VALUES ('Q21', 'odd', now() - interval '100 days')`); err != nil {
		t.Fatal(err)
	}

	fake.classes["Q11424"] = `{"id":"Q11424","labels":{"en":{"value":"film"}},"claims":{` + itemClaim("P279", "Q141524128") + `}}`

	var logged bytes.Buffer
	job.Log = slog.New(slog.NewTextHandler(&logged, nil))
	edited(t, db, "Q20", "Q21", "Q11424")
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(logged.String(), "class=Q11424") || !strings.Contains(logged.String(), "class=Q20") {
		t.Errorf("only the class whose meaning changed should have its titles worked out again:\n%s", logged.String())
	}
	var label string
	var checked time.Time
	if err := db.QueryRow(ctx, `SELECT label, checked_at FROM wikidata_classes WHERE qid = 'Q21'`).Scan(&label, &checked); err != nil || label != "odd" || time.Since(checked) > time.Minute {
		t.Errorf("an unreadable class should be kept and count as checked: %q, %v, %v", label, checked, err)
	}
	var genres []string
	if err := db.QueryRow(ctx, `SELECT genres FROM titles WHERE id = $1`, noir).Scan(&genres); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(genres, "thriller") {
		t.Errorf("a class gaining a parent should rework its titles' genres: %v", genres)
	}
	var version *string
	if err := db.QueryRow(ctx, `SELECT facts_version FROM titles WHERE id = $1`, plain).Scan(&version); err != nil || version == nil {
		t.Errorf("a title not using the class shouldn't be worked out again: %v", err)
	}
	var parents []string
	if err := db.QueryRow(ctx, `SELECT parents FROM wikidata_classes WHERE qid = 'Q11424'`).Scan(&parents); err != nil || fmt.Sprint(parents) != "[Q141524128]" {
		t.Errorf("the film class should still be saved with its new parent: %v, %v", parents, err)
	}
	if p := pendingItems(t, db); len(p) != 0 {
		t.Errorf("pending = %v", p)
	}
}

func TestQueueRefreshes(t *testing.T) {
	db, _, job := editsSetup(t)
	ctx := context.Background()
	for i := range 3 {
		seedTitle(t, db, "movie", core.WikidataTitle{QID: fmt.Sprintf("Q%d", 100+i), Name: fmt.Sprintf("Film %d", i), Year: 2000 + i, TMDB: int32(100 + i), InstanceOf: []string{"Q11424"}})
	}
	waiting := seedTitle(t, db, "movie", core.WikidataTitle{QID: "Q200", Name: "Waiting", Year: 2020, TMDB: 200, InstanceOf: []string{"Q11424"}})
	if _, err := db.Exec(ctx, `UPDATE titles SET wikidata_checked_at = now() - interval '8 days', slug = 't-' || id WHERE id = $1`, waiting); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE titles SET wikidata_checked_at = now() - interval '20 days' WHERE wikidata_id = 'Q101'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `DELETE FROM job_runs`); err != nil {
		t.Fatal(err)
	}
	if err := job.queue(ctx); err != nil {
		t.Fatal(err)
	}
	if p := pendingItems(t, db); fmt.Sprint(p) != "[Q101 Q11424 Q200]" {
		t.Errorf("queued %v, want the oldest title, the oldest class and the title waiting for its slug", p)
	}
	if err := job.queue(ctx); err != nil {
		t.Fatal(err)
	}
	if p := pendingItems(t, db); len(p) != 3 {
		t.Errorf("queued again within the hour: %v", p)
	}
}

func TestLoggedOutRenames(t *testing.T) {
	db, fake, job := editsSetup(t)
	ctx := context.Background()
	show := itemClaim("P31", "Q5398426")
	weapon := seedTitle(t, db, "show", core.WikidataTitle{QID: "Q16", Name: "Lethal Weapon", Year: 2016, Sitelinks: 27, InstanceOf: []string{"Q5398426"}})
	famous := seedTitle(t, db, "show", core.WikidataTitle{QID: "Q17", Name: "Friends", Year: 1994, Sitelinks: 90, InstanceOf: []string{"Q5398426"}})
	small := seedTitle(t, db, "show", core.WikidataTitle{QID: "Q18", Name: "Dirty angel", Year: 2026, InstanceOf: []string{"Q5398426"}})
	oktobar := seedTitle(t, db, "show", core.WikidataTitle{QID: "Q19", Name: "Октобар", Year: 1967, InstanceOf: []string{"Q5398426"}})
	fake.set("Q16", "n'juWeapon", show, dateClaim("P580", "+2016-09-21T00:00:00Z"))
	fake.set("Q17", "Enemies", show, dateClaim("P580", "+1994-09-22T00:00:00Z"))
	fake.set("Q18", "Dirty Angel", show, dateClaim("P580", "+2026-01-01T00:00:00Z"))
	fake.set("Q19", "Oktobar", show, dateClaim("P580", "+1967-01-01T00:00:00Z"))
	at := time.Now().Add(-25 * time.Hour)
	edits := map[string]core.Edits{}
	for _, q := range []string{"Q16", "Q17", "Q18", "Q19"} {
		edits[q] = core.Edits{First: at, Last: at, LoggedOut: true}
	}
	cursor, err := core.FeedCursor(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := core.QueueWikidataEdits(ctx, db, edits, cursor); err != nil {
		t.Fatal(err)
	}
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if w := titleNow(t, db, weapon); w.Name != "Lethal Weapon" || w.Year != 2016 {
		t.Errorf("a logged-out editor's complete rename should keep the old name: %+v", w)
	}
	if f := titleNow(t, db, famous); f.Name != "Friends" {
		t.Errorf("a well-known title renamed logged out should keep its name, not wait a week: %+v", f)
	}
	if s := titleNow(t, db, small); s.Name != "Dirty Angel" {
		t.Errorf("a logged-out editor's small fix should apply: %+v", s)
	}
	if o := titleNow(t, db, oktobar); o.Name != "Oktobar" {
		t.Errorf("a logged-out editor's move into Latin is a change of script, not a rename, and should apply: %+v", o)
	}
	if p := pendingItems(t, db); len(p) != 0 {
		t.Errorf("pending = %v, want nothing held", p)
	}
}
