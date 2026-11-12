package jobs

import (
	"compress/gzip"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/dbtest"
)

func gzipFile(t *testing.T, name string, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := gzip.NewWriter(f)
	if _, err := zw.Write([]byte(strings.Join(lines, "\n"))); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func snapshotItem(qid, label string, sitelinks int, modified string, claims ...string) string {
	return fmt.Sprintf(`{"id":%q,"labels":{"en":%q},"sitelinks":%d,"modified":%q,"claims":{%s}}`, qid, label, sitelinks, modified, strings.Join(claims, ","))
}

func stringClaim(p, v string) string {
	return fmt.Sprintf(`%q:[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":%q}}}]`, p, v)
}

func itemClaim(p, q string) string {
	return fmt.Sprintf(`%q:[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":{"id":%q}}}}]`, p, q)
}

func dateClaim(p, d string) string {
	return fmt.Sprintf(`%q:[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":{"time":%q,"precision":11}}}}]`, p, d)
}

func TestSeed(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	old, recent := "2026-10-01T00:00:00Z", "2026-11-09T00:00:00Z"
	items := gzipFile(t, "items.gz",
		snapshotItem("Q1", "Big Film", 40, old, stringClaim("P4947", "100"), itemClaim("P31", "Q11424"), dateClaim("P577", "+2001-05-01T00:00:00Z")),
		snapshotItem("Q2", "Its Remake", 10, old, stringClaim("P4947", "100"), itemClaim("P31", "Q11424")),
		snapshotItem("Q3", "Close Call", 12, old, stringClaim("P4947", "200"), itemClaim("P31", "Q11424")),
		snapshotItem("Q4", "Close Call", 8, old, stringClaim("P4947", "200"), itemClaim("P31", "Q11424")),
		snapshotItem("Q5", "Renamed", 30, recent, stringClaim("P4947", "300"), itemClaim("P31", "Q11424"), dateClaim("P577", "+2010-01-01T00:00:00Z")),
		snapshotItem("Q6", "Kept", 30, recent, stringClaim("P4947", "400"), itemClaim("P31", "Q11424"), dateClaim("P577", "+2012-01-01T00:00:00Z")),
		snapshotItem("Q7", "A Franchise", 90, old, stringClaim("P4947", "500"), itemClaim("P31", "Q196600")),
	)
	extra := gzipFile(t, "extra.gz",
		snapshotItem("Q8", "Webisodes", 3, old, stringClaim("P345", "tt0000008"), itemClaim("P31", "Q526877")),
	)
	classes := gzipFile(t, "classes.gz", `{"id":"Q11424","labels":{"en":"film"},"claims":{}}`, `{"id":"Q196600","labels":{"en":"media franchise"},"claims":{}}`, `{"id":"Q526877","labels":{"en":"web series"},"claims":{}}`)
	names := filepath.Join(t.TempDir(), "names.jsonl")
	if err := os.WriteFile(names, []byte(`{"qid":"Q5","en":"Old Name","mul":null}`+"\n"+`{"qid":"Q6","en":"Kept","mul":null}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	s := Seed{DB: db, Log: slog.New(slog.DiscardHandler), Items: []string{items, extra}, Classes: []string{classes}, Names: names, Taken: time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)}
	if err := s.Run(ctx); err != nil {
		t.Fatal(err)
	}

	rows, err := db.Query(ctx, "SELECT wikidata_id, kind, coalesce(tmdb_id::text, '-'), slug LIKE 't-%' FROM titles ORDER BY wikidata_id")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for rows.Next() {
		var qid, kind, tmdb string
		var waiting bool
		if err := rows.Scan(&qid, &kind, &tmdb, &waiting); err != nil {
			t.Fatal(err)
		}
		slug := "slug"
		if waiting {
			slug = "waits"
		}
		got = append(got, strings.Join([]string{qid, kind, tmdb, slug}, " "))
	}
	want := []string{
		"Q1 movie 100 slug",
		"Q2 movie - waits",
		"Q3 movie - waits",
		"Q4 movie - waits",
		"Q5 movie 300 waits",
		"Q6 movie 400 slug",
		"Q8 show - waits",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("seeded:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if again, err := core.ClassesNeeded(ctx, db, []string{"Q11424"}); err != nil || len(again) != 0 {
		t.Errorf("snapshot classes should be loaded: %v, %v", again, err)
	}
}
