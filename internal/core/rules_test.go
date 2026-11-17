package core

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/dbtest"
)

const (
	pixar    = "Q127552"
	disney   = "Q1047410"
	netflix  = "Q907311"
	hbo      = "Q23633"
	japan    = "JP"
	animated = "animated"
)

type ruleTitle struct {
	kind, name    string
	year, runtime int
	sitelinks     int
	genres, types []string
	studios       []string
	networks      []string
	countries     []string
	adult         bool
}

func addRuleTitles(t *testing.T, db *pgxpool.Pool, titles []ruleTitle) map[string]int64 {
	t.Helper()
	ids := map[string]int64{}
	for _, r := range titles {
		var id int64
		err := db.QueryRow(context.Background(), `
			INSERT INTO titles (id, kind, slug, name, year, runtime_minutes, sitelinks, genres, type_flags, studios, networks, countries, adult)
			SELECT n, $1, 't-' || n, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11 FROM nextval('catalogue_id') AS n
			RETURNING id`,
			r.kind, r.name, r.year, r.runtime, r.sitelinks, orEmpty(r.genres), orEmpty(r.types), orEmpty(r.studios), orEmpty(r.networks), orEmpty(r.countries), r.adult).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		ids[r.name] = id
	}
	return ids
}

func ruleNames(entries []ListEntry) string {
	var names []string
	for _, e := range entries {
		names = append(names, *e.Item.Title.Name)
	}
	return fmt.Sprint(names)
}

func TestRuleLists(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	accountID, _ := signedUpAccount(t, db)
	_, profiles, err := Me(ctx, db, accountID)
	if err != nil {
		t.Fatal(err)
	}
	owner := profiles[0].ID
	partner, err := AddProfile(ctx, db, accountID, NewProfile{Name: "Sam", Kind: "person"})
	if err != nil {
		t.Fatal(err)
	}
	kid, err := AddProfile(ctx, db, accountID, NewProfile{Name: "Kit", Kind: "kids"})
	if err != nil {
		t.Fatal(err)
	}

	ids := addRuleTitles(t, db, []ruleTitle{
		{kind: "movie", name: "Toy Story", year: 1995, runtime: 81, sitelinks: 120, genres: []string{"comedy", "family"}, types: []string{animated}, studios: []string{pixar, "Q191224"}, countries: []string{"US"}},
		{kind: "movie", name: "Aladdin", year: 1992, runtime: 90, sitelinks: 100, genres: []string{"comedy", "family"}, types: []string{animated}, studios: []string{disney}, countries: []string{"US"}},
		{kind: "movie", name: "Up", year: 2009, runtime: 96, sitelinks: 90, genres: []string{"adventure"}, types: []string{animated}, studios: []string{pixar}, countries: []string{"US"}},
		{kind: "movie", name: "Cars Uncut", year: 2006, runtime: 117, sitelinks: 1, genres: []string{"comedy"}, studios: []string{pixar}, adult: true},
		{kind: "show", name: "Stranger Things", year: 2016, sitelinks: 110, genres: []string{"drama"}, networks: []string{netflix}, countries: []string{"US"}},
		{kind: "show", name: "Midnight Diner", year: 2016, sitelinks: 20, genres: []string{"drama"}, networks: []string{netflix}, countries: []string{japan}},
		{kind: "show", name: "The Wire", year: 2002, sitelinks: 105, genres: []string{"crime", "drama"}, networks: []string{hbo}},
		{kind: "movie", name: "Toy Story (copy)", year: 1995, sitelinks: 500, studios: []string{pixar}},
	})
	if _, err := db.Exec(ctx, "UPDATE titles SET merged_into_id = $1 WHERE id = $2", ids["Toy Story"], ids["Toy Story (copy)"]); err != nil {
		t.Fatal(err)
	}

	pixarRule := Rule{Studios: Values{Any: []string{pixar}}}
	everything, err := CreateList(ctx, db, owner, ListChanges{Name: ptr("Everything from Pixar"), Rule: &pixarRule})
	if err != nil {
		t.Fatal(err)
	}
	if everything.Rule == nil || everything.ItemCount != 3 || everything.Matched != 3 || len(everything.Cover) != 3 {
		t.Errorf("new rule list = %+v", everything)
	}
	_, entries, total, err := ListEntries(ctx, db, owner, everything.ID, ListQuery{})
	if err != nil || total != 3 || ruleNames(entries) != "[Toy Story Up Cars Uncut]" {
		t.Errorf("pixar = %s, %d, %v", ruleNames(entries), total, err)
	}
	if entries[0].ID != ids["Toy Story"] || entries[0].Rank != 1 || entries[1].Rank != 2 {
		t.Errorf("rule entries are numbered by title, ranked best known first: %+v", entries[0].Entry)
	}
	if _, entries, _, err := ListEntries(ctx, db, owner, everything.ID, ListQuery{SortBy: "year"}); err != nil || ruleNames(entries) != "[Toy Story Cars Uncut Up]" {
		t.Errorf("by year = %s, %v", ruleNames(entries), err)
	}

	for _, c := range []struct {
		name string
		rule Rule
		want string
	}{
		{"disney's animated films", Rule{Kind: "movie", Studios: Values{Any: []string{pixar, disney}}, Types: Values{Any: []string{animated}}, Year: Between{From: ptr(1989), To: ptr(1999)}}, "[Toy Story Aladdin]"},
		{"netflix originals", Rule{Kind: "show", Networks: Values{Any: []string{netflix}}}, "[Stranger Things Midnight Diner]"},
		{"not from japan", Rule{Networks: Values{Any: []string{netflix}}, Countries: Values{None: []string{japan}}}, "[Stranger Things]"},
		{"comedies under 100 minutes", Rule{Genres: Values{Any: []string{"comedy"}}, Runtime: Between{To: ptr(99)}}, "[Toy Story Aladdin]"},
		{"dramas but not crime", Rule{Genres: Values{Any: []string{"drama"}, None: []string{"crime"}}}, "[Stranger Things Midnight Diner]"},
		{"no runtime never matches a runtime", Rule{Kind: "show", Runtime: Between{From: ptr(1)}}, "[]"},
	} {
		p, err := PreviewRule(ctx, db, owner, c.rule, 0, ListQuery{})
		if err != nil || ruleNames(p.Entries) != c.want || p.Matched != len(p.Entries) {
			t.Errorf("%s = %s (%d matched), %v; want %s", c.name, ruleNames(p.Entries), p.Matched, err, c.want)
		}
	}

	if _, err := db.Exec(ctx, "INSERT INTO watch_events (profile_id, title_id, watched_at, action) VALUES ($1, $2, now(), 'watch')", owner, ids["Toy Story"]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "INSERT INTO ratings (profile_id, title_id, rating, rated_at) VALUES ($1, $2, 8, now())", partner.ID, ids["Up"]); err != nil {
		t.Fatal(err)
	}
	unseen, err := CreateList(ctx, db, owner, ListChanges{Name: ptr("Pixar I haven't seen"), Privacy: ptr("household"), Rule: &Rule{Studios: Values{Any: []string{pixar}}, Watched: ptr(false)}})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		who    string
		viewer int64
		want   string
	}{
		{"the owner", owner, "[Up Cars Uncut]"},
		{"the rest of the household, by their own history", partner.ID, "[Toy Story Up Cars Uncut]"},
		{"a kids profile, without adult titles", kid.ID, "[Toy Story Up]"},
	} {
		if _, entries, _, err := ListEntries(ctx, db, c.viewer, unseen.ID, ListQuery{}); err != nil || ruleNames(entries) != c.want {
			t.Errorf("%s sees %s, %v; want %s", c.who, ruleNames(entries), err, c.want)
		}
	}
	for _, c := range []struct {
		who    string
		viewer int64
		want   []InList
	}{
		{"the owner, who has watched it", owner, []InList{{ListID: everything.ID, ItemID: ids["Toy Story"]}}},
		{"the rest of the household, who can't see the private list", partner.ID, []InList{{ListID: unseen.ID, ItemID: ids["Toy Story"]}}},
	} {
		if title, err := Title(ctx, db, c.viewer, fmt.Sprint(ids["Toy Story"]), "movie"); err != nil || fmt.Sprint(title.Lists) != fmt.Sprint(c.want) {
			t.Errorf("title page for %s is on %v, %v; want %v", c.who, title.Lists, err, c.want)
		}
	}
	if p, err := PreviewRule(ctx, db, partner.ID, Rule{Studios: Values{Any: []string{pixar}}, Rated: ptr(true)}, 0, ListQuery{}); err != nil || ruleNames(p.Entries) != "[Up]" {
		t.Errorf("rated = %s, %v", ruleNames(p.Entries), err)
	}
	if p, err := PreviewRule(ctx, db, 0, Rule{Studios: Values{Any: []string{pixar}}, Watched: ptr(true)}, 0, ListQuery{}); err != nil || ruleNames(p.Entries) != "[Toy Story Up Cars Uncut]" {
		t.Errorf("signed out, personal rules are left out: %s, %v", ruleNames(p.Entries), err)
	}
	if err := SaveClasses(ctx, db, []WikidataClass{{QID: pixar, Label: "Pixar"}}); err != nil {
		t.Fatal(err)
	}
	if p, err := PreviewRule(ctx, db, 0, Rule{Studios: Values{Any: []string{pixar}, None: []string{disney}}}, 0, ListQuery{}); err != nil || fmt.Sprint(p.Names) != "map[Q127552:Pixar]" {
		t.Errorf("names = %v, %v", p.Names, err)
	}
	if l, err := GetList(ctx, db, owner, everything.ID); err != nil || l.RuleNames[pixar] != "Pixar" {
		t.Errorf("a rule list's names = %v, %v", l.RuleNames, err)
	}

	if err := RemoveFromList(ctx, db, partner.ID, unseen.ID, ids["Cars Uncut"]); err != nil {
		t.Fatal(err)
	}
	l, entries, _, err := ListEntries(ctx, db, owner, unseen.ID, ListQuery{})
	if err != nil || ruleNames(entries) != "[Up]" || fmt.Sprint(l.Rule.LeftOut) != fmt.Sprint([]int64{ids["Cars Uncut"]}) {
		t.Errorf("after removing a title = %s, %+v, %v", ruleNames(entries), l.Rule, err)
	}
	if p, err := PreviewRule(ctx, db, owner, Rule{Studios: Values{Any: []string{pixar}}}, unseen.ID, ListQuery{}); err != nil || ruleNames(p.Entries) != "[Toy Story Up]" {
		t.Errorf("a preview for a rule list leaves out what was taken out of it: %s, %v", ruleNames(p.Entries), err)
	}
	if _, err := PreviewRule(ctx, db, owner, pixarRule, 999999, ListQuery{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("a preview for a list that isn't there = %v", err)
	}
	if n, err := RemoveManyFromList(ctx, db, owner, unseen.ID, []int64{ids["Up"], ids["The Wire"]}); err != nil || n != 1 {
		t.Errorf("removing several = %d, %v", n, err)
	}
	if err := RemoveFromList(ctx, db, owner, unseen.ID, ids["The Wire"]); !errors.Is(err, ErrNotFound) {
		t.Errorf("removing a title that doesn't match = %v", err)
	}
	if _, err := UpdateList(ctx, db, owner, unseen.ID, ListChanges{Rule: &Rule{Studios: Values{Any: []string{pixar}}, Watched: ptr(false)}}); err != nil {
		t.Fatal(err)
	}
	if _, entries, _, err := ListEntries(ctx, db, owner, unseen.ID, ListQuery{}); err != nil || ruleNames(entries) != "[]" {
		t.Errorf("saving a rule without left_out keeps them out: %s, %v", ruleNames(entries), err)
	}
	if _, err := UpdateList(ctx, db, owner, unseen.ID, ListChanges{Rule: &Rule{Studios: Values{Any: []string{pixar}}, Watched: ptr(false), LeftOut: []int64{}}}); err != nil {
		t.Fatal(err)
	}
	if _, entries, _, err := ListEntries(ctx, db, owner, unseen.ID, ListQuery{}); err != nil || ruleNames(entries) != "[Up Cars Uncut]" {
		t.Errorf("saving it with none puts them back: %s, %v", ruleNames(entries), err)
	}
	dup := addRuleTitles(t, db, []ruleTitle{{kind: "movie", name: "Up again", sitelinks: 2, studios: []string{pixar}}})["Up again"]
	if err := RemoveFromList(ctx, db, owner, unseen.ID, dup); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "UPDATE titles SET merged_into_id = $1 WHERE id = $2", ids["Up"], dup); err != nil {
		t.Fatal(err)
	}
	if _, entries, _, err := ListEntries(ctx, db, owner, unseen.ID, ListQuery{}); err != nil || ruleNames(entries) != "[Cars Uncut]" {
		t.Errorf("a title taken out stays out when merged into another: %s, %v", ruleNames(entries), err)
	}
	if _, err := UpdateList(ctx, db, owner, unseen.ID, ListChanges{Rule: &Rule{Studios: Values{Any: []string{pixar}}, Watched: ptr(false), LeftOut: []int64{}}}); err != nil {
		t.Fatal(err)
	}

	if _, err := AddToList(ctx, db, owner, everything.ID, []Listing{{Ref: Ref{Kind: "movie", IDs: IDs{TMDB: 1}}}}); !errors.Is(err, ErrRuleList) {
		t.Errorf("adding by hand = %v", err)
	}
	if err := ReorderList(ctx, db, owner, everything.ID, []int64{ids["Up"], ids["Toy Story"], ids["Cars Uncut"]}); !errors.Is(err, ErrRuleList) {
		t.Errorf("reordering = %v", err)
	}
	if err := SetListNotes(ctx, db, owner, everything.ID, ids["Up"], "balloons"); !errors.Is(err, ErrRuleList) {
		t.Errorf("notes = %v", err)
	}
	if _, err := UpdateList(ctx, db, owner, everything.ID, ListChanges{Cover: ptr(ids["Up"])}); !errors.Is(err, ErrRuleList) {
		t.Errorf("picking a cover = %v", err)
	}
	if _, err := UpdateList(ctx, db, owner, everything.ID, ListChanges{SortBy: ptr("added")}); !errors.Is(err, ErrRuleSort) {
		t.Errorf("sorting by date added = %v", err)
	}
	byHand, err := CreateList(ctx, db, owner, ListChanges{Name: ptr("By hand")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateList(ctx, db, owner, byHand.ID, ListChanges{Rule: &pixarRule}); !errors.Is(err, ErrRuleList) {
		t.Errorf("giving a hand-made list a rule = %v", err)
	}

	for _, c := range []struct {
		name string
		rule Rule
	}{
		{"an unknown genre", Rule{Genres: Values{Any: []string{"cowboy"}}}},
		{"an unknown type", Rule{Types: Values{None: []string{"cartoon"}}}},
		{"a studio that isn't a Wikidata ID", Rule{Studios: Values{Any: []string{"Pixar"}}}},
		{"a lowercase country", Rule{Countries: Values{Any: []string{"us"}}}},
		{"a backwards year range", Rule{Year: Between{From: ptr(2000), To: ptr(1990)}}},
		{"a negative runtime", Rule{Runtime: Between{From: ptr(-1)}}},
		{"too many taken out", Rule{LeftOut: make([]int64, 1001)}},
		{"an unknown kind", Rule{Kind: "episode"}},
		{"too many values", Rule{Studios: Values{Any: make([]string, 51)}}},
	} {
		if _, err := PreviewRule(ctx, db, owner, c.rule, 0, ListQuery{}); !errors.Is(err, ErrRule) {
			t.Errorf("%s = %v", c.name, err)
		}
	}

	if err := AddListToUpNext(ctx, db, partner.ID, unseen.ID); err != nil {
		t.Fatal(err)
	}
	up, err := UpNextLists(ctx, db, partner.ID)
	if err != nil || len(up) != 1 || *up[0].Next.Title.Name != "Toy Story" {
		t.Errorf("up next follows a rule list = %+v, %v", up, err)
	}

	if _, err := UpdateList(ctx, db, owner, everything.ID, ListChanges{Privacy: ptr("public")}); err != nil {
		t.Fatal(err)
	}
	browsed, _, err := BrowseLists(ctx, db, partner.ID, BrowseQuery{Page: Page{Page: 1, Limit: 10}})
	if err != nil || len(browsed) != 1 || browsed[0].ID != everything.ID {
		t.Errorf("a public rule list can be browsed: %+v, %v", browsed, err)
	}

	copied, err := CopyRuleList(ctx, db, partner.ID, everything.ID, "Pixar, my order")
	if err != nil {
		t.Fatal(err)
	}
	if _, entries, _, err := ListEntries(ctx, db, partner.ID, copied.ID, ListQuery{}); err != nil || copied.Rule != nil || ruleNames(entries) != "[Toy Story Up Cars Uncut]" {
		t.Errorf("copy = %+v, %s, %v", copied, ruleNames(entries), err)
	}
	if _, err := CopyRuleList(ctx, db, owner, byHand.ID, "Again"); !errors.Is(err, ErrNotFound) {
		t.Errorf("copying a hand-made list = %v", err)
	}
}

func TestCompanies(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	addRuleTitles(t, db, []ruleTitle{
		{kind: "movie", name: "Toy Story", sitelinks: 1, studios: []string{pixar}},
		{kind: "movie", name: "Up", sitelinks: 1, studios: []string{pixar}},
		{kind: "show", name: "Stranger Things", sitelinks: 1, networks: []string{netflix}},
	})
	if err := SaveClasses(ctx, db, []WikidataClass{{QID: pixar, Label: "Pixar"}, {QID: "Q1", Label: "Pixar Canada"}, {QID: netflix, Label: "Netflix"}, {QID: "Q2", Label: "Pixar fan", Human: true}}); err != nil {
		t.Fatal(err)
	}
	found, err := Companies(ctx, db, "pix")
	if err != nil || fmt.Sprint(found) != "[{Q127552 Pixar 2 0}]" {
		t.Errorf("companies = %v, %v", found, err)
	}
	if found, err := Companies(ctx, db, "flix"); err != nil || len(found) != 0 {
		t.Errorf("matches start a word: %v, %v", found, err)
	}
	if found, err := Companies(ctx, db, "100%_"); err != nil || len(found) != 0 {
		t.Errorf("wildcards are plain text: %v, %v", found, err)
	}
}
