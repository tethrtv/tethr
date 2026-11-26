package jobs

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/migrations"
)

func watch(t *testing.T, db *pgxpool.Pool, ref core.Ref) {
	t.Helper()
	ctx := context.Background()
	account, _, err := core.CreateAccount(ctx, db, core.SignUp{Email: fmt.Sprintf("viewer%d@example.com", ref.IDs.TMDB+ref.Show.TMDB), Name: "Viewer", Username: fmt.Sprintf("viewer%d", ref.IDs.TMDB+ref.Show.TMDB)})
	if err != nil {
		t.Fatal(err)
	}
	caller, err := core.AccountCaller(ctx, db, account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := core.ScrobbleStop(ctx, db, caller, core.Scrobble{Ref: ref, Progress: 100}); err != nil {
		t.Fatal(err)
	}
}

func titleExists(t *testing.T, db *pgxpool.Pool, id int64) bool {
	t.Helper()
	var n int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM titles WHERE id = $1`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1
}

func tmdbOf(t *testing.T, db *pgxpool.Pool, id int64) int32 {
	t.Helper()
	var tmdb *int32
	if err := db.QueryRow(context.Background(), `SELECT tmdb_id FROM titles WHERE id = $1`, id).Scan(&tmdb); err != nil {
		t.Fatal(err)
	}
	if tmdb == nil {
		return 0
	}
	return *tmdb
}

func TestTwinsGoAfterTheirWeek(t *testing.T) {
	db, fake, job := editsSetup(t)
	ctx := context.Background()
	twins := func(qid, name string, movie, show int32, classes ...string) (int64, int64) {
		w := core.WikidataTitle{QID: qid, Name: name, Year: 1997, InstanceOf: classes}
		w.TMDB = movie
		film := seedTitle(t, db, "movie", w)
		w.TMDB = show
		series := seedTitle(t, db, "show", w)
		var p31 []string
		for _, c := range classes {
			p31 = append(p31, fmt.Sprintf(`{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":{"id":%q}}}}`, c))
		}
		fake.set(qid, name, stringClaim("P4947", strconv.Itoa(int(movie))), stringClaim("P4983", strconv.Itoa(int(show))), `"P31":[`+strings.Join(p31, ",")+`]`)
		return film, series
	}
	wallace, wallaceShow := twins("Q30", "George Wallace", 30, 3030, "Q11424")
	_, physicianShow := twins("Q31", "The Physician", 31, 3131, "Q11424")
	bagpussFilm, bagpuss := twins("Q32", "Bagpuss", 32, 3232, "Q5398426")
	madeFilm, madeShow := twins("Q33", "Made in America", 33, 3333, "Q11424", "Q5398426")
	season, number := int32(1), int32(1)
	watch(t, db, core.Ref{Kind: "episode", Show: core.IDs{TMDB: 3131}, Season: &season, Number: &number})

	if err := core.CheckCatalogue(ctx, db); err != nil {
		t.Fatal(err)
	}
	for _, e := range reviewEntries(t, db) {
		if _, err := core.DismissReview(ctx, db, strconv.FormatInt(e.ID, 10), "triaged"); err != nil {
			t.Fatal(err)
		}
	}

	edited(t, db, "Q30", "Q31", "Q32", "Q33")
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if p := pendingItems(t, db); fmt.Sprint(p) != "[Q30 Q31 Q32]" {
		t.Errorf("held = %v, want the twins whose classes give one kind held for a week", p)
	}
	if !titleExists(t, db, wallaceShow) || !titleExists(t, db, bagpussFilm) {
		t.Error("a twin went before its week")
	}

	aWeekLater(t, db)
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		id   int64
		want bool
	}{
		"the film the classes give": {wallace, true}, "its unused show": {wallaceShow, false},
		"a watched show":            {physicianShow, true},
		"the show the classes give": {bagpuss, true}, "its unused film": {bagpussFilm, false},
		"a film that's also a show": {madeFilm, true}, "a show that's also a film": {madeShow, true},
	} {
		if got := titleExists(t, db, c.id); got != c.want {
			t.Errorf("%s exists = %v, want %v", name, got, c.want)
		}
	}
	if got := fmt.Sprint(reviewNow(t, db)); got != "[kind_change The Physician |show>movie]" {
		t.Errorf("review = %s, want only the watched twin left for a person", got)
	}
	var dismissed int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM review_entries WHERE qid IN ('Q30', 'Q32') AND outcome = 'dismissed'`).Scan(&dismissed); err != nil || dismissed != 2 {
		t.Errorf("dismissed entries kept = %d, %v, want the history kept", dismissed, err)
	}
	if p := pendingItems(t, db); len(p) != 0 {
		t.Errorf("still pending: %v", p)
	}

	edited(t, db, "Q30")
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	var shows int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM titles WHERE wikidata_id = 'Q30' AND kind = 'show'`).Scan(&shows); err != nil || shows != 0 {
		t.Errorf("shows for Q30 after another edit = %d, %v, want the twin to stay gone", shows, err)
	}
	if err := core.CheckCatalogue(ctx, db); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(reviewNow(t, db)); got != "[kind_change The Physician |show>movie]" {
		t.Errorf("review after the daily check = %s", got)
	}
}

func TestDroppedTMDBIDsGoAfterTheirWeek(t *testing.T) {
	db, fake, job := editsSetup(t)
	ctx := context.Background()
	film := itemClaim("P31", "Q11424")
	wrong := seedTitle(t, db, "movie", core.WikidataTitle{QID: "Q40", Name: "Wrong Holder", Year: 2010, TMDB: 40, InstanceOf: []string{"Q11424"}})
	right := seedTitle(t, db, "movie", core.WikidataTitle{QID: "Q41", Name: "Right One", Year: 2011, InstanceOf: []string{"Q11424"}})
	watched := seedTitle(t, db, "movie", core.WikidataTitle{QID: "Q42", Name: "Watched", Year: 2012, TMDB: 42, InstanceOf: []string{"Q11424"}})
	watch(t, db, core.Ref{Kind: "movie", IDs: core.IDs{TMDB: 42}})
	fake.set("Q40", "Wrong Holder", film, stringClaim("P4947", "40"))
	fake.set("Q41", "Right One", film, stringClaim("P4947", "40"))
	fake.set("Q42", "Watched", film)

	edited(t, db, "Q40", "Q41")
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	aWeekLater(t, db)
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(reviewNow(t, db)); got != "[shared_id Right One tmdb 40|> with Wrong Holder]" {
		t.Fatalf("review = %s", got)
	}

	fake.set("Q40", "Wrong Holder", film)
	edited(t, db, "Q40", "Q42")
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if tmdbOf(t, db, wrong) != 40 {
		t.Error("a TMDB ID went before its week")
	}
	if p := pendingItems(t, db); fmt.Sprint(p) != "[Q40]" {
		t.Errorf("held = %v, want only the unused title's item", p)
	}
	aWeekLater(t, db)
	for range 2 {
		if err := job.Refresh(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if got := tmdbOf(t, db, wrong); got != 0 {
		t.Errorf("unused title's TMDB ID after its week = %d, want it gone with Wikidata's", got)
	}
	if got := tmdbOf(t, db, right); got != 40 {
		t.Errorf("the right title's TMDB ID = %d, want it to take the freed ID", got)
	}
	if got := tmdbOf(t, db, watched); got != 42 {
		t.Errorf("a watched title's TMDB ID = %d, want it kept for the apps that use it", got)
	}
	if got := reviewNow(t, db); len(got) != 0 {
		t.Errorf("review = %v, want the shared ID cleared", got)
	}
	if p := pendingItems(t, db); len(p) != 0 {
		t.Errorf("still pending: %v", p)
	}
}

func reviewEntries(t *testing.T, db *pgxpool.Pool) []core.ReviewEntry {
	t.Helper()
	open, err := core.OpenReview(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	return open
}

func TestTwinsNobodyEditsGoAfterTheirWeek(t *testing.T) {
	db, fake, job := editsSetup(t)
	ctx := context.Background()
	twins := func(qid, name string, movie, show int32, classes ...string) (int64, int64) {
		w := core.WikidataTitle{QID: qid, Name: name, Year: 1997, InstanceOf: classes}
		w.TMDB = movie
		film := seedTitle(t, db, "movie", w)
		w.TMDB = show
		series := seedTitle(t, db, "show", w)
		var p31 []string
		for _, c := range classes {
			p31 = append(p31, fmt.Sprintf(`{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":{"id":%q}}}}`, c))
		}
		fake.set(qid, name, stringClaim("P4947", strconv.Itoa(int(movie))), stringClaim("P4983", strconv.Itoa(int(show))), `"P31":[`+strings.Join(p31, ",")+`]`)
		return film, series
	}
	wallace, wallaceShow := twins("Q30", "George Wallace", 30, 3030, "Q11424")
	bagpussFilm, bagpuss := twins("Q32", "Bagpuss", 32, 3232, "Q5398426")
	madeFilm, madeShow := twins("Q33", "Made in America", 33, 3333, "Q11424", "Q5398426")
	seedTitle(t, db, "movie", core.WikidataTitle{QID: "Q34", Name: "Just a Film", Year: 1997, TMDB: 34, InstanceOf: []string{"Q11424"}})

	upMigration(t, db, "00042_queue_film_and_show_items.sql")
	if p := pendingItems(t, db); fmt.Sprint(p) != "[Q30 Q32 Q33]" {
		t.Errorf("queued = %v, want every item with a film and a show", p)
	}

	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if p := pendingItems(t, db); fmt.Sprint(p) != "[Q30 Q32]" {
		t.Errorf("held = %v, want the twins whose classes give one kind held for a week", p)
	}
	if !titleExists(t, db, wallaceShow) || !titleExists(t, db, bagpussFilm) {
		t.Error("a twin went before its week")
	}

	aWeekLater(t, db)
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		id   int64
		want bool
	}{
		"the film the classes give": {wallace, true}, "its unused show": {wallaceShow, false},
		"the show the classes give": {bagpuss, true}, "its unused film": {bagpussFilm, false},
		"a film that's also a show": {madeFilm, true}, "a show that's also a film": {madeShow, true},
	} {
		if got := titleExists(t, db, c.id); got != c.want {
			t.Errorf("%s exists = %v, want %v", name, got, c.want)
		}
	}
	if p := pendingItems(t, db); len(p) != 0 {
		t.Errorf("still pending: %v", p)
	}
}

func upMigration(t *testing.T, db *pgxpool.Pool, name string) {
	t.Helper()
	raw, err := migrations.FS.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	up, _, _ := strings.Cut(string(raw), "-- +goose Down")
	if _, err := db.Exec(context.Background(), up); err != nil {
		t.Fatal(err)
	}
}
