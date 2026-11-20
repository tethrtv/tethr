package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/dbtest"
)

func movie(tmdb int64) Ref {
	return Ref{Kind: "movie", IDs: IDs{TMDB: tmdb}}
}

func listsByName(t *testing.T, db *pgxpool.Pool, profile int64) map[string]List {
	t.Helper()
	lists, err := Lists(context.Background(), db, profile)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]List{}
	for _, l := range lists {
		if l.Owner.ID == profile {
			out[l.Slug] = l
		}
	}
	return out
}

func listTMDB(t *testing.T, db *pgxpool.Pool, profile, id int64) string {
	t.Helper()
	_, entries, _, err := ListEntries(context.Background(), db, profile, id, ListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		s := fmt.Sprint(*e.Item.Title.TmdbID)
		if e.Notes != nil {
			s += " " + *e.Notes
		}
		out = append(out, s)
	}
	return strings.Join(out, ", ")
}

func TestImportLists(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	caller, _, _ := connectedProfile(t, db)
	profile := caller.ProfileID

	favourites, err := CreateList(ctx, db, profile, ListChanges{Name: ptr("Favourites")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddToList(ctx, db, profile, favourites.ID, []Listing{{Ref: movie(10)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateList(ctx, db, profile, ListChanges{Name: ptr("Horror"), Rule: &Rule{Kind: "movie"}}); err != nil {
		t.Fatal(err)
	}

	file := ImportFile{Lists: []ImportList{
		{Name: "favourites", Items: []ImportItem{
			{Ref: movie(10), Label: "Ten (2001)"},
			{Ref: movie(11), Label: "Eleven (2002)"},
			{Ref: Ref{Kind: "movie"}, Label: "Lost film (1930)"},
		}},
		{Name: "  Best of 2019 ", Description: "<p>My <a href=\"https://boxd.it/x\">picks</a> &amp; more</p><p>Second</p>", Items: []ImportItem{
			{Ref: movie(21), Label: "Twenty-one (2019)", Notes: "  rewatch  "},
			{Ref: movie(11), Label: "Eleven (2002)"},
		}},
		{Name: "Horror", Items: []ImportItem{{Ref: movie(31), Label: "Thirty-one (1980)"}}},
		{Name: "Not this one", Items: []ImportItem{{Ref: movie(41), Label: "Forty-one (1990)"}}},
		{Name: "Nothing matched", Items: []ImportItem{{Ref: Ref{Kind: "movie"}, Label: "Home video (1998)"}}},
	}}
	parsed, err := CreateImport(ctx, db, profile, "letterboxd", "letterboxd.zip", file)
	if err != nil {
		t.Fatal(err)
	}
	s := parsed.Summary
	if s.ListEntries.Total != 8 || s.ListEntries.Unmatched != 2 || len(s.Lists) != 5 || s.ListRoom != maxLists-2 {
		t.Fatalf("parsed = %+v", s)
	}
	if l := s.Lists[0]; l.Name != "favourites" || !l.Existing || l.Total != 3 || l.Unmatched != 1 || !l.Chosen {
		t.Errorf("favourites = %+v", l)
	}
	if l := s.Lists[1]; l.Name != "Best of 2019" || l.Existing || l.Total != 2 {
		t.Errorf("best of 2019 = %+v", l)
	}
	if s.Lists[2].Existing {
		t.Error("a rule list was picked to add to")
	}

	if _, err := QueueImport(ctx, db, profile, parsed.ID, []int{0, 7}); !errors.Is(err, ErrImportLists) {
		t.Errorf("choosing a list that isn't there: %v", err)
	}
	if _, err := QueueImport(ctx, db, profile, parsed.ID, []int{0, 1, 2, 4}); err != nil {
		t.Fatal(err)
	}
	if found, _, err := ApplyNextImport(ctx, db); !found || err != nil {
		t.Fatalf("apply = %v, %v", found, err)
	}
	applied, err := ImportByID(ctx, db, profile, parsed.ID)
	if err != nil {
		t.Fatal(err)
	}
	s = applied.Summary
	if s.ListEntries.Added != 4 || s.ListEntries.Skipped != 1 || s.ListEntries.Unmatched != 2 {
		t.Errorf("list entries = %+v", s.ListEntries)
	}
	if l := s.Lists[0]; l.Added != 1 || l.Skipped != 1 || l.Unmatched != 1 || l.Created {
		t.Errorf("favourites after = %+v", l)
	}
	if l := s.Lists[1]; l.Added != 2 || !l.Created {
		t.Errorf("best of 2019 after = %+v", l)
	}
	if l := s.Lists[3]; l.Chosen || l.Added != 0 || l.Created {
		t.Errorf("left out list = %+v", l)
	}
	if l := s.Lists[4]; l.Created || l.Unmatched != 1 {
		t.Errorf("unmatched list = %+v", l)
	}

	lists := listsByName(t, db, profile)
	if len(lists) != 4 {
		t.Errorf("lists = %v", lists)
	}
	if _, ok := lists["not-this-one"]; ok {
		t.Error("a list that wasn't chosen came across")
	}
	if got := listTMDB(t, db, profile, favourites.ID); got != "10, 11" {
		t.Errorf("favourites = %s", got)
	}
	best := lists["best-of-2019"]
	if best.Name != "Best of 2019" || best.Privacy != "private" || best.SortBy != "rank" || best.Description == nil || *best.Description != "My picks & more\nSecond" {
		t.Errorf("best of 2019 = %+v", best)
	}
	if got := listTMDB(t, db, profile, best.ID); got != "21 rewatch, 11" {
		t.Errorf("best of 2019 = %s", got)
	}
	if got := listTMDB(t, db, profile, lists["horror-2"].ID); got != "31" {
		t.Errorf("horror = %s", got)
	}

	if _, err := AddToList(ctx, db, profile, best.ID, []Listing{{Ref: movie(50)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := UndoImport(ctx, db, profile, parsed.ID); err != nil {
		t.Fatal(err)
	}
	lists = listsByName(t, db, profile)
	if _, ok := lists["horror-2"]; ok || len(lists) != 3 {
		t.Errorf("lists after undo = %v", lists)
	}
	if got := listTMDB(t, db, profile, favourites.ID); got != "10" {
		t.Errorf("favourites after undo = %s", got)
	}
	if got := listTMDB(t, db, profile, best.ID); got != "50" {
		t.Errorf("a list with your own films in it after undo = %s", got)
	}
}

func TestImportListLimits(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	caller, _, _ := connectedProfile(t, db)
	profile := caller.ProfileID

	for i := range maxLists - 2 {
		if _, err := CreateList(ctx, db, profile, ListChanges{Name: ptr(fmt.Sprint("List ", i))}); err != nil {
			t.Fatal(err)
		}
	}
	big := ImportList{Name: strings.Repeat("Long name ", 20)}
	for i := range maxListItems + 3 {
		big.Items = append(big.Items, ImportItem{Ref: movie(int64(i + 1)), Label: fmt.Sprint(i + 1)})
	}
	file := ImportFile{Lists: []ImportList{
		big,
		{Name: "One more", Items: []ImportItem{{Ref: movie(1), Label: "1"}}},
		{Name: "One too many", Items: []ImportItem{{Ref: movie(1), Label: "1"}}},
		{Name: "List 3", Items: []ImportItem{{Ref: movie(1), Label: "1"}}},
	}}
	parsed, err := CreateImport(ctx, db, profile, "letterboxd", "letterboxd.zip", file)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Summary.ListRoom != 2 || len([]rune(parsed.Summary.Lists[0].Name)) != 100 {
		t.Fatalf("parsed = %+v", parsed.Summary.Lists[0].Name)
	}
	if _, err := QueueImport(ctx, db, profile, parsed.ID, nil); !errors.Is(err, ErrTooManyLists) {
		t.Errorf("all four lists with room for two: %v", err)
	}
	if _, err := QueueImport(ctx, db, profile, parsed.ID, []int{0, 1, 3}); err != nil {
		t.Fatal(err)
	}
	if found, _, err := ApplyNextImport(ctx, db); !found || err != nil {
		t.Fatalf("apply = %v, %v", found, err)
	}
	applied, _ := ImportByID(ctx, db, profile, parsed.ID)
	if l := applied.Summary.Lists[0]; l.Added != maxListItems || l.Full != 3 || !l.Created {
		t.Errorf("big list = %+v", l)
	}
	if l := applied.Summary.Lists[3]; l.Added != 1 || l.Created {
		t.Errorf("list 3 = %+v", l)
	}
	if n := len(listsByName(t, db, profile)); n != maxLists {
		t.Errorf("lists = %d", n)
	}
}

func TestImportListsWithoutRoomAtApply(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	caller, _, _ := connectedProfile(t, db)
	profile := caller.ProfileID

	for i := range maxLists - 1 {
		if _, err := CreateList(ctx, db, profile, ListChanges{Name: ptr(fmt.Sprint("List ", i))}); err != nil {
			t.Fatal(err)
		}
	}
	file := ImportFile{Lists: []ImportList{{Name: "Last one", Items: []ImportItem{{Ref: movie(1), Label: "1"}, {Ref: movie(2), Label: "2"}}}}}
	parsed, err := CreateImport(ctx, db, profile, "letterboxd", "letterboxd.zip", file)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := QueueImport(ctx, db, profile, parsed.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateList(ctx, db, profile, ListChanges{Name: ptr("Made meanwhile")}); err != nil {
		t.Fatal(err)
	}
	if found, _, err := ApplyNextImport(ctx, db); !found || err != nil {
		t.Fatalf("apply = %v, %v", found, err)
	}
	applied, _ := ImportByID(ctx, db, profile, parsed.ID)
	if l := applied.Summary.Lists[0]; applied.Status != "applied" || l.Created || l.Full != 2 {
		t.Errorf("list with no room = %s %+v", applied.Status, l)
	}
}

func TestImportListsCheckedWhenChosen(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	caller, _, _ := connectedProfile(t, db)
	profile := caller.ProfileID

	for i := range maxLists - 1 {
		if _, err := CreateList(ctx, db, profile, ListChanges{Name: ptr(fmt.Sprint("List ", i))}); err != nil {
			t.Fatal(err)
		}
	}
	file := ImportFile{Lists: []ImportList{
		{Name: "Twice", Items: []ImportItem{{Ref: movie(1), Label: "1"}}},
		{Name: "twice", Items: []ImportItem{{Ref: movie(2), Label: "2"}}},
		{Name: "List 0", Items: []ImportItem{{Ref: movie(3), Label: "3"}, {Ref: Ref{Kind: "movie"}, Label: "Lost"}}},
		{Name: "Left out", Items: []ImportItem{{Ref: movie(4), Label: "4"}}},
	}}
	parsed, err := CreateImport(ctx, db, profile, "letterboxd", "letterboxd.zip", file)
	if err != nil {
		t.Fatal(err)
	}
	if l := parsed.Summary.Lists; l[0].Existing || l[1].Existing || !l[2].Existing {
		t.Fatalf("lists = %+v", l)
	}

	lists := listsByName(t, db, profile)
	if err := DeleteList(ctx, db, profile, lists["list-0"].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateList(ctx, db, profile, ListChanges{Name: ptr("Made meanwhile")}); err != nil {
		t.Fatal(err)
	}
	if _, err := QueueImport(ctx, db, profile, parsed.ID, []int{0, 1, 2}); !errors.Is(err, ErrTooManyLists) {
		t.Errorf("a list deleted since the file was read still counted as there: %v", err)
	}
	queued, err := QueueImport(ctx, db, profile, parsed.ID, []int{0, 1})
	if err != nil {
		t.Fatal(err)
	}
	if s := queued.Summary; s.ListEntries.Total != 2 || s.ListEntries.Unmatched != 0 || !s.Lists[1].Existing || s.Lists[2].Existing {
		t.Errorf("queued = %+v", s)
	}
	if found, _, err := ApplyNextImport(ctx, db); !found || err != nil {
		t.Fatalf("apply = %v, %v", found, err)
	}
	applied, _ := ImportByID(ctx, db, profile, parsed.ID)
	if l := applied.Summary.Lists; !l[0].Created || l[1].Created || l[1].Added != 1 {
		t.Errorf("applied = %+v", l)
	}
	if got := listTMDB(t, db, profile, listsByName(t, db, profile)["twice"].ID); got != "1, 2" {
		t.Errorf("twice = %s", got)
	}
}

func TestImportListsKeepPrivacyAndSort(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	caller, _, _ := connectedProfile(t, db)
	profile := caller.ProfileID
	kid, err := AddProfile(ctx, db, caller.AccountID, NewProfile{Name: "Ollie", Kind: "kids"})
	if err != nil {
		t.Fatal(err)
	}

	file := ImportFile{Lists: []ImportList{
		{Name: "Shared", Privacy: "public", SortBy: "added", SortHow: "desc", Items: []ImportItem{{Ref: movie(1), Label: "1"}}},
		{Name: "Friends only", Privacy: "friends", SortBy: "released", Items: []ImportItem{{Ref: movie(2), Label: "2"}}},
		{Name: "Odd sort", Privacy: "private", SortBy: "percentage", SortHow: "desc", Items: []ImportItem{{Ref: movie(3), Label: "3"}}},
	}}
	parsed, err := CreateImport(ctx, db, profile, "trakt", "trakt.zip", file)
	if err != nil {
		t.Fatal(err)
	}
	if l := parsed.Summary.Lists; l[0].Privacy != "public" || l[1].Privacy != "private" || l[2].Privacy != "private" {
		t.Fatalf("privacy = %+v", l)
	}
	if _, err := QueueImport(ctx, db, profile, parsed.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ApplyNextImport(ctx, db); err != nil {
		t.Fatal(err)
	}
	lists := listsByName(t, db, profile)
	if l := lists["shared"]; l.Privacy != "public" || l.SortBy != "added" || l.SortHow != "desc" {
		t.Errorf("shared = %+v", l)
	}
	if l := lists["friends-only"]; l.Privacy != "private" || l.SortBy != "year" || l.SortHow != "asc" {
		t.Errorf("friends only = %+v", l)
	}
	if l := lists["odd-sort"]; l.SortBy != "rank" || l.SortHow != "asc" {
		t.Errorf("odd sort = %+v", l)
	}

	kids, err := CreateImport(ctx, db, kid.ID, "trakt", "trakt.zip", file)
	if err != nil {
		t.Fatal(err)
	}
	if kids.Summary.Lists[0].Privacy != "private" {
		t.Errorf("a kids profile was offered a public list: %+v", kids.Summary.Lists[0])
	}
	if _, err := QueueImport(ctx, db, kid.ID, kids.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ApplyNextImport(ctx, db); err != nil {
		t.Fatal(err)
	}
	if l := listsByName(t, db, kid.ID)["shared"]; l.Privacy != "private" {
		t.Errorf("kids list = %+v", l)
	}
}

func TestImportListsSavedBeforePrivacy(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	caller, _, _ := connectedProfile(t, db)
	profile := caller.ProfileID

	file := ImportFile{Lists: []ImportList{{Name: "Old", Items: []ImportItem{{Ref: movie(1), Label: "1"}}}}}
	parsed, err := CreateImport(ctx, db, profile, "letterboxd", "letterboxd.zip", file)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `UPDATE imports SET
		payload = convert_to((convert_from(payload, 'UTF8')::jsonb #- '{Lists,0,Privacy}' #- '{Lists,0,SortBy}' #- '{Lists,0,SortHow}')::text, 'UTF8'),
		summary = summary #- '{lists,0,privacy}'
		WHERE id = $1`, parsed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := QueueImport(ctx, db, profile, parsed.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ApplyNextImport(ctx, db); err != nil {
		t.Fatal(err)
	}
	applied, _ := ImportByID(ctx, db, profile, parsed.ID)
	if l := listsByName(t, db, profile)["old"]; applied.Status != "applied" || l.Privacy != "private" || l.SortBy != "rank" {
		t.Errorf("an import saved before lists kept privacy = %s, %+v", applied.Status, l)
	}
}

func TestPlainText(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{`<p>My <a href="https://boxd.it/x">picks</a> &amp; more</p><p>Second</p>`, "My picks & more\nSecond"},
		{"Films that **twist**, see [the list](https://trakt.tv/x) and _more_.", "Films that twist, see the list and more."},
		{"## Ranked\n> worth it\n![poster](https://x/p.jpg) `code` *really*", "Ranked\nworth it\nposter code really"},
		{"5 * 3 = 15 and snake_case_name stay", "5 * 3 = 15 and snake_case_name stay"},
	} {
		if got := plainText(c.in); got != c.want {
			t.Errorf("plainText(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestImportListMissingItemsListedOnce(t *testing.T) {
	db := dbtest.New(t)
	caller, _, _ := connectedProfile(t, db)
	items := []ImportItem{{Label: "Lost: its items are missing from the export"}, {}, {}}
	file := ImportFile{Lists: []ImportList{{Name: "Lost", Items: items}, {Name: "Kept", Items: []ImportItem{{Ref: movie(1), Label: "1"}}}}}
	parsed, err := CreateImport(context.Background(), db, caller.ProfileID, "trakt", "trakt.zip", file)
	if err != nil {
		t.Fatal(err)
	}
	if s := parsed.Summary; s.Lists[0].Total != 3 || s.Lists[0].Unmatched != 3 || len(s.Unmatched) != 1 {
		t.Errorf("summary = %+v", s)
	}
}
