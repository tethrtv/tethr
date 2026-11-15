package core

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/tethrtv/tethr/internal/dbtest"
)

func TestKidsHiding(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	acct, owner := household(t, db, "ann")
	kid, err := AddProfile(ctx, db, acct, NewProfile{Name: "Ollie", Kind: "kids"})
	if err != nil {
		t.Fatal(err)
	}
	adult := catalogueTitle(t, db, "movie", WikidataTitle{QID: "Q1", Name: "Night Moves", Year: 1999, Sitelinks: 5})
	sexual := catalogueTitle(t, db, "movie", WikidataTitle{QID: "Q2", Name: "Dark Kiss", Year: 2000, Sitelinks: 4})
	film := catalogueTitle(t, db, "movie", WikidataTitle{QID: "Q3", Name: "Paddington", Year: 2014, Sitelinks: 50})
	sequel := catalogueTitle(t, db, "movie", WikidataTitle{QID: "Q4", Name: "Paddington 2", Year: 2017, Sitelinks: 40})
	for _, sql := range []string{
		fmt.Sprintf("UPDATE titles SET adult = true WHERE id = %d", adult),
		fmt.Sprintf("UPDATE titles SET sexual_content = true WHERE id = %d", sexual),
	} {
		if _, err := db.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}

	family, err := CreateList(ctx, db, owner, ListChanges{Name: ptr("Family"), Privacy: ptr("household")})
	if err != nil {
		t.Fatal(err)
	}
	var listings []Listing
	for _, id := range []int64{adult, sexual, film, sequel} {
		listings = append(listings, Listing{Ref: Ref{Kind: "movie", IDs: IDs{Tethr: id}}})
	}
	if _, err := AddToList(ctx, db, owner, family.ID, listings); err != nil {
		t.Fatal(err)
	}
	_, all, _, err := ListEntries(ctx, db, owner, family.ID, ListQuery{})
	if err != nil || len(all) != 4 {
		t.Fatalf("the owner's entries = %+v, %v", all, err)
	}
	if _, err := UpdateList(ctx, db, owner, family.ID, ListChanges{Cover: &all[0].ID}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []int64{owner, kid.ID} {
		if err := AddListToUpNext(ctx, db, p, family.ID); err != nil {
			t.Fatal(err)
		}
	}

	entryNames := func(entries []ListEntry) string {
		var items []Item
		for _, e := range entries {
			items = append(items, e.Item)
		}
		return names(items)
	}
	for _, c := range []struct {
		profile        int64
		count          int64
		cover, entries string
		next           string
	}{
		{owner, 4, "[Night Moves 1999]", "[Night Moves 1999 Dark Kiss 2000 Paddington 2014 Paddington 2 2017]", "[Night Moves 1999]"},
		{kid.ID, 2, "[Paddington 2014 Paddington 2 2017]", "[Paddington 2014 Paddington 2 2017]", "[Paddington 2014]"},
	} {
		l, err := GetList(ctx, db, c.profile, family.ID)
		if err != nil {
			t.Fatal(err)
		}
		if l.ItemCount != c.count || names(l.Cover) != c.cover {
			t.Errorf("profile %d sees %d items with cover %s, want %d and %s", c.profile, l.ItemCount, names(l.Cover), c.count, c.cover)
		}
		_, entries, total, err := ListEntries(ctx, db, c.profile, family.ID, ListQuery{})
		if err != nil || entryNames(entries) != c.entries || total != int(c.count) {
			t.Errorf("profile %d's entries = %s (%d), %v, want %s", c.profile, entryNames(entries), total, err, c.entries)
		}
		upNext, err := UpNextLists(ctx, db, c.profile)
		if err != nil || len(upNext) != 1 || names([]Item{upNext[0].Next}) != c.next {
			t.Errorf("profile %d's next from the list = %+v, %v, want %s", c.profile, upNext, err, c.next)
		}
	}

	ownerView, _ := GetList(ctx, db, owner, family.ID)
	kidView, _ := GetList(ctx, db, kid.ID, family.ID)
	if ownerView.Version == kidView.Version {
		t.Errorf("the owner and the kids profile share a version, %s", ownerView.Version)
	}
	_, seen, _, _ := ListEntries(ctx, db, kid.ID, family.ID, ListQuery{})
	if len(seen) != 2 || seen[0].Rank != 1 || seen[1].Rank != 2 {
		t.Errorf("the kids profile's ranks give hidden items away: %+v", seen)
	}
	if _, err := db.Exec(ctx, fmt.Sprintf("UPDATE titles SET sexual_content = true WHERE id = %d", sequel)); err != nil {
		t.Fatal(err)
	}
	if again, _ := GetList(ctx, db, kid.ID, family.ID); again.Version == kidView.Version || again.ItemCount != 1 {
		t.Errorf("flagging a title left the kids profile's version at %s with %d items", again.Version, again.ItemCount)
	}
	if _, err := db.Exec(ctx, fmt.Sprintf("UPDATE titles SET sexual_content = false WHERE id = %d", sequel)); err != nil {
		t.Fatal(err)
	}

	if err := ReorderList(ctx, db, kid.ID, family.ID, []int64{all[0].ID, all[1].ID, all[3].ID, all[2].ID}); !errors.Is(err, ErrListOrder) {
		t.Errorf("the kids profile reordering with hidden items = %v", err)
	}
	if err := ReorderList(ctx, db, kid.ID, family.ID, []int64{all[3].ID, all[2].ID}); err != nil {
		t.Errorf("the kids profile reordering what it sees = %v", err)
	}
	_, entries, _, _ := ListEntries(ctx, db, owner, family.ID, ListQuery{})
	if got := entryNames(entries); got != "[Night Moves 1999 Dark Kiss 2000 Paddington 2 2017 Paddington 2014]" {
		t.Errorf("after the kids profile's reorder the owner sees %s", got)
	}

	own, err := CreateList(ctx, db, kid.ID, ListChanges{Name: ptr("Ollie's")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddToList(ctx, db, kid.ID, own.ID, listings[1:3]); err != nil {
		t.Fatal(err)
	}
	out, err := Export(ctx, db, acct)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range out.Profiles {
		for _, l := range p.Lists {
			found = found || l.List.ID == own.ID
			if l.List.ID == own.ID && (l.List.ItemCount != 2 || entryNames(l.Entries) != "[Dark Kiss 2000 Paddington 2014]") {
				t.Errorf("the kids profile's export of its own list = %d items, %s", l.List.ItemCount, entryNames(l.Entries))
			}
		}
	}
	if !found {
		t.Error("the kids profile's own list is missing from the export")
	}
}
