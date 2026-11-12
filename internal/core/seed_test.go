package core

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/tethrtv/tethr/internal/dbtest"
)

func TestSeed(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	if err := SaveClasses(ctx, db, []WikidataClass{{QID: "Q11424"}, {QID: "Q5398426"}, {QID: "Q24856"}, {QID: "Q99", Parents: []string{"Q11424"}}}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		tmdb, p31 []string
		want      string
	}{
		{[]string{"movie"}, []string{"Q11424"}, "[movie]"},
		{[]string{"movie"}, []string{"Q24856"}, "[]"},
		{[]string{"show"}, nil, "[show]"},
		{nil, []string{"Q99"}, "[movie]"},
		{nil, []string{"Q11424", "Q5398426"}, "[]"},
		{nil, []string{"Q11424", "Q24856"}, "[movie]"},
		{nil, nil, "[]"},
	} {
		got, _, err := SeedKinds(ctx, db, c.tmdb, c.p31)
		if err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("kinds for tmdb %v, instance of %v = %v, %v, want %s", c.tmdb, c.p31, got, err, c.want)
		}
	}

	items, err := LookUp(ctx, db, "movie", IDs{TMDB: 603}, true)
	if err != nil {
		t.Fatal(err)
	}
	existing := items[0].Title
	released := time.Date(1999, 3, 31, 0, 0, 0, 0, time.UTC)
	matrix := WikidataTitle{QID: "Q83495", Name: "The Matrix", Year: 1999, Released: released, TMDB: 603, IDs: map[string][]string{"imdb": {"tt0133093"}}}
	if got, err := SeedTitle(ctx, db, "movie", matrix, 0, true); err != nil || got.Outcome != SeedUpdated {
		t.Fatalf("seeding a title that already exists = %v, %v", got, err)
	}
	items, _ = LookUp(ctx, db, "movie", IDs{TMDB: 603}, false)
	if m := items[0].Title; m.ID != existing.ID || m.Slug != "the-matrix-1999" || *m.WikidataID != "Q83495" {
		t.Errorf("an existing title should keep its ID and get its slug: %+v", m)
	}

	other := WikidataTitle{QID: "Q1", Name: "Another Matrix", Year: 1999, Released: released, TMDB: 603}
	if got, err := SeedTitle(ctx, db, "movie", other, 0, true); err != nil || got.Outcome != SeedSkipped {
		t.Errorf("an item whose TMDB ID belongs to another item's title = %v, %v", got, err)
	}

	fresh := WikidataTitle{QID: "Q2", Name: "Inception", Year: 2010, Released: time.Date(2010, 7, 16, 0, 0, 0, 0, time.UTC), TMDB: 27205}
	if got, err := SeedTitle(ctx, db, "movie", fresh, 0, false); err != nil || got.Outcome != SeedCreated {
		t.Fatalf("a new item = %v, %v", got, err)
	}
	items, _ = LookUp(ctx, db, "movie", IDs{TMDB: 27205}, false)
	if s := items[0].Title.Slug; s != fmt.Sprintf("t-%d", items[0].Title.ID) {
		t.Errorf("a name edited in the week before the snapshot should wait for its slug: %q", s)
	}
	if got, err := SeedTitle(ctx, db, "movie", fresh, 0, false); err != nil || got.Outcome != SeedUpdated {
		t.Errorf("seeding the same item again = %v, %v", got, err)
	}

	if _, err := LookUp(ctx, db, "movie", IDs{TMDB: 155}, true); err != nil {
		t.Fatal(err)
	}
	unowned := WikidataTitle{QID: "Q3", Name: "One of two", Year: 2008}
	if got, err := SeedTitle(ctx, db, "movie", unowned, 155, true); err != nil || got.Outcome != SeedSkipped {
		t.Errorf("an item sharing an unowned TMDB ID that an unlinked title holds should be skipped: %v, %v", got, err)
	}
	remake := WikidataTitle{QID: "Q4", Name: "The Matrix remake", Year: 2030, IDs: map[string][]string{"imdb": {"tt9999990", "tt0133093"}}}
	if got, err := SeedTitle(ctx, db, "movie", remake, 603, false); err != nil || got.Outcome != SeedSkipped {
		t.Errorf("a later IMDb value should still find the title it belongs to: %v, %v", got, err)
	}
	if got, err := SeedTitle(ctx, db, "movie", WikidataTitle{QID: "Q5", Name: "Lost the TMDB ID"}, 603, false); err != nil || got.Outcome != SeedCreated {
		t.Errorf("an item that lost a shared TMDB ID to its owner should still get a title: %v, %v", got, err)
	}
}

func TestDueWikidataUnmatchedMonthly(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	unmatched, _ := LookUp(ctx, db, "movie", IDs{TMDB: 1}, true)
	waiting, _ := LookUp(ctx, db, "movie", IDs{TMDB: 2}, true)
	if err := WikidataMissing(ctx, db, unmatched[0].Title.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveWikidata(ctx, db, waiting[0].Title.ID, "movie", WikidataTitle{QID: "Q2", Name: "Not Out Yet"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "UPDATE titles SET wikidata_checked_at = now() - interval '8 days'"); err != nil {
		t.Fatal(err)
	}
	due, err := DueWikidata(ctx, db, 10)
	if err != nil || len(due) != 1 || due[0].ID != waiting[0].Title.ID {
		t.Errorf("after a week only the title waiting for its slug should be due: %+v, %v", due, err)
	}
	if _, err := db.Exec(ctx, "UPDATE titles SET wikidata_checked_at = now() - interval '31 days' WHERE id = $1", unmatched[0].Title.ID); err != nil {
		t.Fatal(err)
	}
	if due, err := DueWikidata(ctx, db, 10); err != nil || len(due) != 2 {
		t.Errorf("a title Wikidata didn't know should be asked again after a month: %+v, %v", due, err)
	}
}
