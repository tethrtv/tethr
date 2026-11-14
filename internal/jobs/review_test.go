package jobs

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/core"
)

func editedInBatch(t *testing.T, db *pgxpool.Pool, batch string, qids ...string) {
	t.Helper()
	edits := map[string]core.Edits{}
	at := time.Now().Add(-25 * time.Hour)
	for _, q := range qids {
		edits[q] = core.Edits{First: at, Last: at, Batch: batch}
	}
	cursor, err := core.FeedCursor(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := core.QueueWikidataEdits(context.Background(), db, edits, cursor); err != nil {
		t.Fatal(err)
	}
}

func reviewNow(t *testing.T, db *pgxpool.Pool) []string {
	t.Helper()
	open, err := core.OpenReview(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range open {
		s := fmt.Sprintf("%s %s %s|%s>%s", e.Kind, e.TitleName, e.Subject, e.Tethr, e.Wikidata)
		if e.Batch != "" {
			s += " in " + e.Batch
		}
		if e.OtherName != "" {
			s += " with " + e.OtherName
		}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func TestKeptNamesGoOnReview(t *testing.T) {
	db, fake, job := editsSetup(t)
	ctx := context.Background()
	film := itemClaim("P31", "Q11424")
	seedTitle(t, db, "movie", core.WikidataTitle{QID: "Q1", Name: "Akira", Year: 1988, TMDB: 149, InstanceOf: []string{"Q11424"}})
	fake.set("Q1", "アキラ", film, stringClaim("P4947", "149"), dateClaim("P577", "+1988-07-16T00:00:00Z"))

	editedInBatch(t, db, "QSv2/294922", "Q1")
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(reviewNow(t, db)); got != "[kept_name Akira |Akira>アキラ in QSv2/294922]" {
		t.Errorf("review = %s, want the kept name with its batch", got)
	}
	edited(t, db, "Q1")
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if got := reviewNow(t, db); len(got) != 1 {
		t.Errorf("review = %v, want one entry however often the name is kept", got)
	}

	fake.set("Q1", "Akira", film, stringClaim("P4947", "149"), dateClaim("P577", "+1988-07-16T00:00:00Z"))
	edited(t, db, "Q1")
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if got := reviewNow(t, db); len(got) != 0 {
		t.Errorf("review = %v, want it cleared once Wikidata is fixed", got)
	}
}

func TestHoldsGoOnReviewAfterTheirWeek(t *testing.T) {
	db, fake, job := editsSetup(t)
	ctx := context.Background()
	film := itemClaim("P31", "Q11424")
	seedTitle(t, db, "movie", core.WikidataTitle{QID: "Q2", Name: "Saw", Year: 2004, TMDB: 176, InstanceOf: []string{"Q11424"}})
	seedTitle(t, db, "show", core.WikidataTitle{QID: "Q3", Name: "Karate kid leyends", Year: 2023, Sitelinks: 40, InstanceOf: []string{"Q5398426"}})
	seedTitle(t, db, "movie", core.WikidataTitle{QID: "Q4", Name: "Swapped", Year: 2010, TMDB: 44, InstanceOf: []string{"Q11424"}})
	seedTitle(t, db, "movie", core.WikidataTitle{QID: "Q15", Name: "Newsreel 68", Year: 1944, InstanceOf: []string{"Q11424"}})
	seedTitle(t, db, "movie", core.WikidataTitle{QID: "Q16", Name: "Newsreel 69", Year: 1944, InstanceOf: []string{"Q11424"}})
	fake.set("Q2", "Saw", film, stringClaim("P4947", "176"), dateClaim("P577", "+2007-01-01T00:00:00Z"))
	fake.set("Q3", "American Born Chinese", itemClaim("P31", "Q5398426"), dateClaim("P580", "+2023-05-24T00:00:00Z"))
	fake.set("Q4", "Swapped", itemClaim("P31", "Q5398426"), stringClaim("P4983", "4444"), dateClaim("P580", "+2010-01-01T00:00:00Z"))
	fake.set("Q15", "Newsreel 68", itemClaim("P31", "Q21191270"))
	fake.set("Q16", "Newsreel 69", itemClaim("P31", "Q21191270"))

	edited(t, db, "Q2", "Q3", "Q4")
	editedInBatch(t, db, "QSv2/300001", "Q15", "Q16")
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if got := reviewNow(t, db); len(got) != 0 {
		t.Errorf("review = %v, want nothing while edits are held", got)
	}

	fake.set("Q3", "Karate kid leyends", itemClaim("P31", "Q5398426"), dateClaim("P580", "+2023-05-24T00:00:00Z"))
	aWeekLater(t, db)
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	want := "[held_change Saw year|2004>2007 kind_change Newsreel 68 |movie>none in QSv2/300001 kind_change Newsreel 69 |movie>none in QSv2/300001 kind_change Swapped |movie>show]"
	if got := fmt.Sprint(reviewNow(t, db)); got != want {
		t.Errorf("review =\n%s\nwant\n%s", got, want)
	}

	fake.set("Q2", "Saw", film, stringClaim("P4947", "176"), dateClaim("P577", "+2004-01-01T00:00:00Z"))
	edited(t, db, "Q2", "Q15", "Q16")
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(reviewNow(t, db)); got != want {
		t.Errorf("review = %s, want entries kept while their items are held again", got)
	}
	aWeekLater(t, db)
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(reviewNow(t, db)); got != "[kind_change Newsreel 68 |movie>none in QSv2/300001 kind_change Newsreel 69 |movie>none in QSv2/300001 kind_change Swapped |movie>show]" {
		t.Errorf("review = %s, want the year cleared once Wikidata's revert applied, with no entry for the revert", got)
	}
}

func TestSharedIDsGoOnReviewAfterTheirWeek(t *testing.T) {
	db, fake, job := editsSetup(t)
	ctx := context.Background()
	film := itemClaim("P31", "Q11424")
	seedTitle(t, db, "movie", core.WikidataTitle{QID: "Q5", Name: "One", Year: 2001, TMDB: 5, InstanceOf: []string{"Q11424"}})
	seedTitle(t, db, "movie", core.WikidataTitle{QID: "Q6", Name: "Other", Year: 2002, TMDB: 6, IDs: map[string][]string{"imdb": {"tt0000006"}}, InstanceOf: []string{"Q11424"}})
	fake.set("Q5", "One", film, stringClaim("P4947", "5"), stringClaim("P345", "tt0000006"))
	fake.set("Q6", "Other", film, stringClaim("P4947", "6"), stringClaim("P345", "tt0000006"))

	edited(t, db, "Q5")
	for range 2 {
		if err := job.Refresh(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if got := reviewNow(t, db); len(got) != 0 {
		t.Errorf("review = %v, want nothing within the week", got)
	}
	aWeekLater(t, db)
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(reviewNow(t, db)); got != "[shared_id One imdb tt0000006|> with Other]" {
		t.Errorf("review = %s, want an ID both items still claim after the week", got)
	}
	edited(t, db, "Q5")
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if got := reviewNow(t, db); len(got) != 1 {
		t.Errorf("review = %v, want the entry kept rather than held again", got)
	}

	fake.set("Q6", "Other", film, stringClaim("P4947", "6"))
	edited(t, db, "Q5", "Q6")
	for range 2 {
		if err := job.Refresh(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if got := reviewNow(t, db); len(got) != 0 {
		t.Errorf("review = %v, want it cleared once only one item claims the ID", got)
	}
}

func TestDeletedItemsAndUnreadableClassesGoOnReview(t *testing.T) {
	db, fake, job := editsSetup(t)
	ctx := context.Background()
	film := itemClaim("P31", "Q11424")
	seedTitle(t, db, "movie", core.WikidataTitle{QID: "Q7", Name: "Gone", Year: 2001, TMDB: 7, InstanceOf: []string{"Q11424"}})
	seedTitle(t, db, "movie", core.WikidataTitle{QID: "Q8", Name: "Odd genre", Year: 2002, TMDB: 8, InstanceOf: []string{"Q11424"}})
	fake.set("Q8", "Odd genre", film, stringClaim("P4947", "8"), itemClaim("P136", "Q21"))
	fake.classes["Q21"] = `{"id":"Q21","claims":{"P279":"not a list"}}`

	edited(t, db, "Q7", "Q8")
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(reviewNow(t, db)); got != "[deleted_item Gone |> unreadable_class  |>]" {
		t.Errorf("review = %s", got)
	}

	fake.set("Q7", "Gone", film, stringClaim("P4947", "7"))
	fake.classes["Q21"] = `{"id":"Q21","labels":{"en":{"value":"odd"}},"claims":{}}`
	edited(t, db, "Q7", "Q21")
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if got := reviewNow(t, db); len(got) != 0 {
		t.Errorf("review = %v, want both cleared once Wikidata answers", got)
	}
}

func TestKindChangeLeavesOtherEntriesAlone(t *testing.T) {
	db, fake, job := editsSetup(t)
	ctx := context.Background()
	reel := seedTitle(t, db, "movie", core.WikidataTitle{QID: "Q15", Name: "Newsreel 68", Year: 1944, InstanceOf: []string{"Q11424"}})
	if err := core.FlagForReview(ctx, db, []core.ReviewFlag{{Kind: core.KeptName, QID: "Q15", Title: reel, Tethr: "Newsreel 68", Wikidata: "Wochenschau 68", Why: "renamed by a logged-out editor"}}); err != nil {
		t.Fatal(err)
	}
	fake.set("Q15", "Wochenschau 68", itemClaim("P31", "Q21191270"))
	edited(t, db, "Q15")
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	aWeekLater(t, db)
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	want := "[kept_name Newsreel 68 |Newsreel 68>Wochenschau 68 kind_change Newsreel 68 |movie>none]"
	if got := fmt.Sprint(reviewNow(t, db)); got != want {
		t.Errorf("review = %s, want %s: a title kept as it was can't show its other problems are fixed", got, want)
	}
}
