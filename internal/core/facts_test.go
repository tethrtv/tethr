package core

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/tethrtv/tethr/internal/dbtest"
	"github.com/tethrtv/tethr/internal/store"
)

func TestMaps(t *testing.T) {
	for _, name := range []string{"genres", "types", "adult", "sexual", "countries", "languages"} {
		m, err := parseMap(name)
		if err != nil || len(m) == 0 {
			t.Errorf("%s: %d rows, %v", name, len(m), err)
		}
	}
	for _, tags := range genreMap {
		for _, g := range tags {
			if !contains(genreOrder, g) {
				t.Errorf("genre %q isn't in the order list", g)
			}
		}
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func testClasses() classes {
	cs := classes{}
	add := func(qid, country, language string, parents ...string) {
		cs[qid] = store.ClassAncestryRow{Qid: qid, Country: country, Language: language, Parents: parents}
	}
	add("Q100", "", "", "Q40831", "Q19765983")
	add("Q101", "", "", "Q100")
	add("Q102", "", "", "Q5937792", "Q182015", "Q1196408")
	add("Q103", "", "", "Q21010853")
	add("Q200", "", "", "Q185529")
	add("Q201", "", "", "Q181001")
	add("Q17", "JP", "")
	add("Q30", "US", "")
	add("Q33946", "", "")
	add("Q5287", "", "ja")
	add("Q1860", "", "en")
	add("Q9186", "", "")
	add("Q11424", "", "")
	add("Q24862", "", "")
	add("Q20650540", "", "")
	add("Q40831", "", "")
	add("Q19765983", "", "")
	add("Q5937792", "", "")
	add("Q182015", "", "")
	add("Q1196408", "", "")
	add("Q21010853", "", "")
	add("Q185529", "", "")
	add("Q181001", "", "")
	add("Q202", "", "", "Q599558")
	add("Q599558", "", "")
	return cs
}

func TestDeriveFacts(t *testing.T) {
	cs := testClasses()
	for _, c := range []struct {
		name string
		raw  rawFacts
		want string
	}{
		{"romantic comedy gives both, and a long film loses short", rawFacts{Kind: "movie", Runtime: 95, InstanceOf: []string{"Q11424", "Q24862"}, Genres: []string{"Q101"}, Countries: []string{"Q30"}, Languages: []string{"Q1860"}},
			"genres [comedy romance] flags [] adult false countries [US] languages [en] complete true"},
		{"a short runtime makes a film short", rawFacts{Kind: "movie", Runtime: 12, InstanceOf: []string{"Q11424"}},
			"genres [] flags [short] adult false countries [] languages [] complete true"},
		{"anime needs Japan", rawFacts{Kind: "movie", Runtime: 100, InstanceOf: []string{"Q20650540"}, Countries: []string{"Q30"}},
			"genres [] flags [animated] adult false countries [US] languages [] complete true"},
		{"anime from Japan keeps the flag, in its own language", rawFacts{Kind: "movie", Runtime: 125, InstanceOf: []string{"Q20650540"}, Countries: []string{"Q17"}, Languages: []string{"Q5287"}},
			"genres [] flags [animated anime] adult false countries [JP] languages [ja] complete true"},
		{"more than four genres keep the most voted, ties to the commoner", rawFacts{Kind: "movie", Genres: []string{"Q102", "Q103", "Q103", "Q100"}},
			"genres [drama comedy crime romance] flags [] adult false countries [] languages [] complete true"},
		{"hand codes fill what Wikidata leaves blank", rawFacts{Kind: "show", Countries: []string{"Q33946"}, Languages: []string{"Q9186"}},
			"genres [] flags [] adult false countries [CZ] languages [yue] complete true"},
		{"pornography is adult, and sexual content too", rawFacts{Kind: "movie", Genres: []string{"Q200"}},
			"genres [] flags [] adult true countries [] languages [] complete true sexual"},
		{"an erotic film is sexual content but not adult", rawFacts{Kind: "movie", Genres: []string{"Q202"}},
			"genres [] flags [] adult false countries [] languages [] complete true sexual"},
		{"erotica isn't adult", rawFacts{Kind: "movie", Genres: []string{"Q201"}},
			"genres [] flags [] adult false countries [] languages [] complete true"},
		{"a class not fetched yet leaves the facts incomplete", rawFacts{Kind: "movie", Genres: []string{"Q999"}},
			"genres [] flags [] adult false countries [] languages [] complete false"},
	} {
		f := deriveFacts(cs, c.raw)
		got := fmt.Sprintf("genres %v flags %v adult %v countries %v languages %v complete %v", f.Genres, f.TypeFlags, f.Adult, f.Countries, f.Languages, f.Complete)
		if f.SexualContent {
			got += " sexual"
		}
		if got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
}

func TestOwnLanguageName(t *testing.T) {
	names := map[string]string{"zh": "无间道", "fr": "Infernal Affairs (fr)"}
	if got := ownLanguageName(names, []string{"yue"}); got != "无间道" {
		t.Errorf("Cantonese should find the Chinese label: %q", got)
	}
	if got := ownLanguageName(names, []string{"ko"}); got != "" {
		t.Errorf("no label in the title's own language = %q", got)
	}
}

func TestSaveWikidataFacts(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	items, err := LookUp(ctx, db, "movie", IDs{TMDB: 11647}, true)
	if err != nil {
		t.Fatal(err)
	}
	id := items[0].Title.ID
	w := WikidataTitle{QID: "Q699530", Name: "Infernal Affairs (fr)", Year: 2002, Runtime: 101, TMDB: 11647,
		InstanceOf: []string{"Q11424"}, Genres: []string{"Q102"}, Countries: []string{"Q17"}, Languages: []string{"Q9186"},
		Sitelinks: 48, OtherNames: map[string]string{"zh": "无间道", "fr": "Infernal Affairs (fr)"}}

	var classes []WikidataClass
	for _, c := range testClasses() {
		if c.Qid == "Q5937792" {
			continue
		}
		classes = append(classes, WikidataClass{QID: c.Qid, Country: c.Country, Language: c.Language, Parents: c.Parents})
	}
	if err := SaveClasses(ctx, db, classes); err != nil {
		t.Fatal(err)
	}
	if missing, err := ClassesNeeded(ctx, db, []string{"Q102", "Q17"}); err != nil || fmt.Sprint(missing) != "[Q5937792]" {
		t.Errorf("classes needed = %v, %v", missing, err)
	}

	if _, err := SaveWikidata(ctx, db, id, "movie", w); err != nil {
		t.Fatal(err)
	}
	read := func() (string, *string) {
		t.Helper()
		var name string
		var genres, flags, countries, languages []string
		var sitelinks int
		var version *string
		err := db.QueryRow(ctx, "SELECT name, genres, type_flags, countries, languages, sitelinks, facts_version FROM titles WHERE id = $1", id).
			Scan(&name, &genres, &flags, &countries, &languages, &sitelinks, &version)
		if err != nil {
			t.Fatal(err)
		}
		return fmt.Sprintf("%s %v %v %v %v %d", name, genres, flags, countries, languages, sitelinks), version
	}
	if got, version := read(); got != "无间道 [crime thriller history] [] [JP] [yue] 48" || version != nil {
		t.Errorf("before every class is known = %s, version %v", got, version)
	}

	if err := SaveClasses(ctx, db, []WikidataClass{{QID: "Q5937792"}}); err != nil {
		t.Fatal(err)
	}
	outdated, err := OutdatedFacts(ctx, db, 10)
	if err != nil || len(outdated) != 1 {
		t.Fatalf("outdated = %v, %v", outdated, err)
	}
	if err := RederiveFacts(ctx, db, outdated[0]); err != nil {
		t.Fatal(err)
	}
	if got, version := read(); got != "无间道 [crime thriller history] [] [JP] [yue] 48" || version == nil || *version != factsVersion {
		t.Errorf("after the missing class arrives = %s, version %v", got, version)
	}
	if outdated, err := OutdatedFacts(ctx, db, 10); err != nil || len(outdated) != 0 {
		t.Errorf("nothing should be outdated now: %v, %v", outdated, err)
	}

	aWeekPasses(t, db)
	w.Released = time.Date(2002, 12, 12, 0, 0, 0, 0, time.UTC)
	if _, err := SaveWikidata(ctx, db, id, "movie", w); err != nil {
		t.Fatal(err)
	}
	var slug string
	if err := db.QueryRow(ctx, "SELECT slug FROM titles WHERE id = $1", id).Scan(&slug); err != nil || slug != "infernal-affairs-fr-2002" {
		t.Errorf("a name in a script slugs can't use should take its slug from the fallback name: %q, %v", slug, err)
	}
}
