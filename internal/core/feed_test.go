package core

import (
	"context"
	"testing"

	"github.com/tethrtv/tethr/internal/dbtest"
	"github.com/tethrtv/tethr/internal/store"
)

func TestSuspiciousEdits(t *testing.T) {
	name := func(s string) *string { return &s }
	year := func(y int16) *int16 { return &y }
	for _, tc := range []struct {
		why    string
		before store.Title
		w      WikidataTitle
		want   Held
	}{
		{"a well-known title renamed completely", store.Title{Name: name("Karate kid leyends"), Year: year(2023), Sitelinks: 40}, WikidataTitle{Name: "American Born Chinese", Year: 2023}, Held{Name: "American Born Chinese"}},
		{"a well-known title losing a word", store.Title{Name: name("Dr. Seuss' How the Grinch Stole Christmas"), Year: year(2000), Sitelinks: 60}, WikidataTitle{Name: "How the Grinch Stole Christmas", Year: 2000}, Held{}},
		{"a well-known title gaining a comma", store.Title{Name: name("Six Days Seven Nights"), Year: year(1998), Sitelinks: 30}, WikidataTitle{Name: "Six Days, Seven Nights", Year: 1998}, Held{}},
		{"an obscure title renamed completely", store.Title{Name: name("Working Title"), Year: year(2026), Sitelinks: 3}, WikidataTitle{Name: "Something Else", Year: 2026}, Held{}},
		{"a well-known title losing its name", store.Title{Name: name("The Matrix"), Year: year(1999), Sitelinks: 90}, WikidataTitle{Year: 1999}, Held{}},
		{"a title with no name yet", store.Title{Sitelinks: 90}, WikidataTitle{Name: "The Matrix", Year: 1999}, Held{}},
		{"a year moving by one", store.Title{Name: name("Saw"), Year: year(2003)}, WikidataTitle{Name: "Saw", Year: 2004}, Held{}},
		{"a year moving by two", store.Title{Name: name("Saw"), Year: year(2004)}, WikidataTitle{Name: "Saw", Year: 2006}, Held{Year: 2006}},
		{"a first year", store.Title{Name: name("Saw")}, WikidataTitle{Name: "Saw", Year: 2004}, Held{}},
		{"a year going away", store.Title{Name: name("Saw"), Year: year(2004)}, WikidataTitle{Name: "Saw"}, Held{}},
		{"both at once", store.Title{Name: name("Heat"), Year: year(1995), Sitelinks: 50}, WikidataTitle{Name: "Cold", Year: 1985}, Held{Name: "Cold", Year: 1985}},
	} {
		if got := Suspicious(tc.before, tc.w); !got.Within(tc.want) || !tc.want.Within(got) {
			t.Errorf("%s: held %+v, want %+v", tc.why, got, tc.want)
		}
	}
}

func TestHeldWithin(t *testing.T) {
	for _, tc := range []struct {
		now, before Held
		want        bool
	}{
		{Held{}, Held{}, true},
		{Held{}, Held{Year: 2006}, true},
		{Held{Year: 2006}, Held{Year: 2006, Name: "Saw"}, true},
		{Held{Year: 2006}, Held{}, false},
		{Held{Year: 2007}, Held{Year: 2006}, false},
		{Held{Name: "Cold"}, Held{Name: "Hot"}, false},
		{Held{Kinds: []string{"show"}}, Held{Kinds: []string{"show"}}, true},
		{Held{Kinds: []string{"movie", "show"}}, Held{Kinds: []string{"show"}}, false},
	} {
		if got := tc.now.Within(tc.before); got != tc.want {
			t.Errorf("%+v within %+v = %v", tc.now, tc.before, got)
		}
	}
}

func TestScript(t *testing.T) {
	for name, want := range map[string]string{
		"The Matrix":               "latin",
		"Pokémon":                  "latin",
		"Ça tourne à Manhattan":    "latin",
		"進撃の巨人":                    "cjk",
		"ドラゴンボール":                  "cjk",
		"기생충":                      "hangul",
		"Брат":                     "cyrillic",
		"Ζ":                        "greek",
		"2001":                     "",
		"Ghost in the Shell 攻殻機動隊": "latin",
	} {
		if got := script(name); got != want {
			t.Errorf("script(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestRefreshKeepsTheScript(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	items, err := LookUp(ctx, db, "show", IDs{TMDB: 1429}, true)
	if err != nil {
		t.Fatal(err)
	}
	id := items[0].Title.ID
	if _, err := SaveWikidata(ctx, db, id, "show", WikidataTitle{QID: "Q1", Name: "Attack on Titan", Year: 2013}); err != nil {
		t.Fatal(err)
	}
	got, err := RefreshWikidata(ctx, db, id, "show", WikidataTitle{QID: "Q1", Name: "進撃の巨人", Year: 2013, Runtime: 24}, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.KeptName != "進撃の巨人" {
		t.Errorf("kept name = %q, want the new name in another script reported", got.KeptName)
	}
	var name string
	var runtime int
	if err := db.QueryRow(ctx, `SELECT name, runtime_minutes FROM titles WHERE id = $1`, id).Scan(&name, &runtime); err != nil {
		t.Fatal(err)
	}
	if name != "Attack on Titan" || runtime != 24 {
		t.Errorf("after a name in another script: %q, %d minutes, want the old name and the rest applied", name, runtime)
	}
	got, err = RefreshWikidata(ctx, db, id, "show", WikidataTitle{QID: "Q1", Name: "Attack on Titan: The Series", Year: 2013}, false)
	if err != nil || got.KeptName != "" {
		t.Errorf("a rename in the same script = %+v, %v", got, err)
	}

	items, err = LookUp(ctx, db, "show", IDs{TMDB: 30991}, true)
	if err != nil {
		t.Fatal(err)
	}
	ikkoku := items[0].Title.ID
	if _, err := SaveWikidata(ctx, db, ikkoku, "show", WikidataTitle{QID: "Q2", Name: "めぞん一刻", Year: 1986}); err != nil {
		t.Fatal(err)
	}
	got, err = RefreshWikidata(ctx, db, ikkoku, "show", WikidataTitle{QID: "Q2", Name: "Maison Ikkoku", Year: 1986}, false)
	if err != nil || got.KeptName != "" {
		t.Errorf("a name moving into Latin should apply: %+v, %v", got, err)
	}
	if err := db.QueryRow(ctx, `SELECT name FROM titles WHERE id = $1`, ikkoku).Scan(&name); err != nil || name != "Maison Ikkoku" {
		t.Errorf("name = %q, %v", name, err)
	}
}

func TestRefetchEverythingKeepsHolds(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	if _, err := db.Exec(ctx, `INSERT INTO titles (id, kind, slug, wikidata_id) SELECT n, 'movie', 't-' || n, 'Q2' FROM nextval('catalogue_id') AS n`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO wikidata_pending (qid, edited_at, not_before) VALUES ('Q2', now(), now())`); err != nil {
		t.Fatal(err)
	}
	if err := HoldWikidata(ctx, db, "Q2", Held{Year: 2007}); err != nil {
		t.Fatal(err)
	}
	if _, err := QueueAllWikidata(ctx, db); err != nil {
		t.Fatal(err)
	}
	if due, err := DueWikidataEdits(ctx, db, 10); err != nil || len(due) != 0 {
		t.Errorf("refetching everything cut a week's hold short: %+v, %v", due, err)
	}
}

func TestClassMeaning(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	save := func(cs ...WikidataClass) {
		t.Helper()
		if err := SaveClasses(ctx, db, cs); err != nil {
			t.Fatal(err)
		}
	}
	save(WikidataClass{QID: "Q182015", Label: "thriller"}, WikidataClass{QID: "Q17537576", Label: "creative work", Parents: []string{"Q15621286"}},
		WikidataClass{QID: "Q15621286", Label: "intellectual work"}, WikidataClass{QID: "Q20", Label: "noir", Parents: []string{"Q17537576"}})
	meaning := func(q string) string {
		t.Helper()
		m, err := ClassMeaning(ctx, db, q)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	before := meaning("Q20")
	top := meaning("Q17537576")
	save(WikidataClass{QID: "Q17537576", Label: "creative work", Parents: []string{"Q15621286", "Q141524128"}}, WikidataClass{QID: "Q141524128", Label: "something abstract"})
	if meaning("Q17537576") != top {
		t.Error("a new parent that maps to nothing changed what a class means")
	}
	save(WikidataClass{QID: "Q20", Label: "noir", Parents: []string{"Q17537576", "Q182015"}})
	if meaning("Q20") == before {
		t.Error("gaining a thriller parent should change what a class means")
	}
	save(WikidataClass{QID: "Q20", Label: "noir", Parents: []string{"Q17537576", "Q999"}})
	if meaning("Q20") == before {
		t.Error("a parent Tethr hasn't fetched yet leaves the meaning unknown, so it should count as a change")
	}
}
