package core

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tethrtv/tethr/internal/dbtest"
)

func TestListBasics(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	accountID, _ := signedUpAccount(t, db)
	_, profiles, err := Me(ctx, db, accountID)
	if err != nil {
		t.Fatal(err)
	}
	owner := profiles[0].ID

	friday, err := CreateList(ctx, db, owner, ListChanges{Name: ptr("  Friday films  ")})
	if err != nil {
		t.Fatal(err)
	}
	if friday.ID < 1_000_000_000 || friday.Name != "Friday films" || friday.Slug != "friday-films" || friday.Privacy != "private" ||
		friday.SortBy != "rank" || friday.SortHow != "asc" || friday.Access != "mine" || friday.ItemCount != 0 || friday.Watched == nil {
		t.Errorf("new list = %+v", friday)
	}
	again, err := CreateList(ctx, db, owner, ListChanges{Name: ptr("Friday films!")})
	if err != nil || again.Slug != "friday-films-2" {
		t.Errorf("second slug = %q, %v", again.Slug, err)
	}
	named, err := CreateList(ctx, db, owner, ListChanges{Name: ptr("Watchlist")})
	if err != nil || named.Slug != "watchlist-2" {
		t.Errorf("a list called Watchlist = %q, %v", named.Slug, err)
	}

	for _, c := range []struct {
		changes ListChanges
		want    error
	}{
		{ListChanges{Name: ptr("  ")}, ErrListName},
		{ListChanges{Name: ptr(strings.Repeat("a", 101))}, ErrListName},
		{ListChanges{Name: ptr("Ok"), Description: ptr(strings.Repeat("a", 1001))}, ErrListDescription},
		{ListChanges{Name: ptr("Ok"), Privacy: ptr("friends")}, ErrListPrivacy},
		{ListChanges{Name: ptr("Ok"), SortBy: ptr("rating")}, ErrListSort},
		{ListChanges{Name: ptr("Ok"), SortHow: ptr("up")}, ErrListSort},
	} {
		if _, err := CreateList(ctx, db, owner, c.changes); !errors.Is(err, c.want) {
			t.Errorf("create %+v = %v, want %v", c.changes, err, c.want)
		}
	}

	renamed, err := UpdateList(ctx, db, owner, again.ID, ListChanges{Name: ptr("Sunday films"), Privacy: ptr("public"), Description: ptr("Slow ones")})
	if err != nil || renamed.Slug != "sunday-films" || renamed.Privacy != "public" || *renamed.Description != "Slow ones" {
		t.Errorf("rename = %+v, %v", renamed, err)
	}
	cleared, err := UpdateList(ctx, db, owner, again.ID, ListChanges{Description: ptr("")})
	if err != nil || cleared.Description != nil || cleared.Name != "Sunday films" {
		t.Errorf("clearing the description = %+v, %v", cleared, err)
	}

	if err := DeleteList(ctx, db, owner, named.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := GetList(ctx, db, owner, named.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted list = %v", err)
	}
	mine, err := Lists(ctx, db, owner)
	if err != nil || len(mine) != 2 || mine[0].Name != "Friday films" || mine[1].Name != "Sunday films" {
		t.Errorf("lists = %+v, %v", mine, err)
	}

	for i := len(mine); i < maxLists; i++ {
		if _, err := CreateList(ctx, db, owner, ListChanges{Name: ptr("More")}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := CreateList(ctx, db, owner, ListChanges{Name: ptr("One too many")}); !errors.Is(err, ErrTooManyLists) {
		t.Errorf("list %d = %v", maxLists+1, err)
	}
}

func TestListItems(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	caller, _, _ := connectedProfile(t, db)
	owner := caller.ProfileID

	list, err := CreateList(ctx, db, owner, ListChanges{Name: ptr("Mixed")})
	if err != nil {
		t.Fatal(err)
	}
	show := Ref{Kind: "show", IDs: IDs{TMDB: 1396}}
	added, err := AddToList(ctx, db, owner, list.ID, []Listing{
		{Ref: inception(), Notes: "Twice"},
		{Ref: show},
		{Ref: episode(1, 2)},
		{Ref: Ref{Kind: "movie", IDs: IDs{TMDB: 27205}}},
	})
	if err != nil || added.Added != (Counts{Movies: 1, Shows: 1, Episodes: 1}) || added.Existing.Movies != 1 || added.Listed != 3 {
		t.Fatalf("add = %+v, %v", added, err)
	}
	if _, err := AddToList(ctx, db, owner, list.ID, []Listing{{Ref: show, Notes: strings.Repeat("a", 501)}}); !errors.Is(err, ErrListNotes) {
		t.Errorf("long notes = %v", err)
	}

	_, entries, total, err := ListEntries(ctx, db, owner, list.ID, ListQuery{})
	if err != nil || total != 3 || len(entries) != 3 {
		t.Fatalf("entries = %+v, %d, %v", entries, total, err)
	}
	movie, series, ep := entries[0], entries[1], entries[2]
	if movie.Item.Kind() != "movie" || *movie.Notes != "Twice" || series.Item.Kind() != "show" || ep.Item.Kind() != "episode" || movie.Rank != 1 || ep.Rank != 3 {
		t.Errorf("entries in rank order = %+v", entries)
	}
	if movie.Progress == nil || movie.Progress.Total != 1 || movie.Progress.Watched != 0 {
		t.Errorf("unwatched film progress = %+v", movie.Progress)
	}

	showID := series.Item.Title.ID
	aired := time.Now().AddDate(-1, 0, 0)
	later := time.Now().AddDate(1, 0, 0)
	if err := SaveOutline(ctx, db, showID, []OutlineEpisode{
		{TMDBID: 62085, Season: 0, Number: 1, FirstAired: &aired},
		{TMDBID: 62086, Season: 1, Number: 1, FirstAired: &aired},
		{TMDBID: 62087, Season: 1, Number: 2, FirstAired: &aired},
		{TMDBID: 62088, Season: 1, Number: 3, FirstAired: &later},
	}); err != nil {
		t.Fatal(err)
	}
	before, err := GetList(ctx, db, owner, list.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ScrobbleStop(ctx, db, caller, Scrobble{Ref: inception(), Progress: 95}); err != nil {
		t.Fatal(err)
	}
	if _, err := ScrobbleStop(ctx, db, caller, Scrobble{Ref: episode(1, 2), Progress: 95}); err != nil {
		t.Fatal(err)
	}

	got, entries, _, err := ListEntries(ctx, db, owner, list.ID, ListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if *got.Watched != 2 || got.Version == before.Version {
		t.Errorf("after watching: watched %d, version %q then %q", *got.Watched, before.Version, got.Version)
	}
	if p := entries[0].Progress; p.Watched != 1 || !p.Done() || p.LastWatchedAt == nil {
		t.Errorf("watched film = %+v", p)
	}
	if p := entries[1].Progress; p.Watched != 1 || p.Total != 2 || p.Done() {
		t.Errorf("show with one of two aired episodes watched = %+v", p)
	}
	if p := entries[2].Progress; !p.Done() {
		t.Errorf("watched episode = %+v", p)
	}
	if len(entries) != 3 {
		t.Errorf("watching took something off the list: %+v", entries)
	}

	_, left, total, err := ListEntries(ctx, db, owner, list.ID, ListQuery{HideWatched: true})
	if err != nil || total != 1 || left[0].Item.Kind() != "show" {
		t.Errorf("hiding watched = %+v, %d, %v", left, total, err)
	}
	_, shows, _, err := ListEntries(ctx, db, owner, list.ID, ListQuery{Kind: "show"})
	if err != nil || len(shows) != 1 {
		t.Errorf("only shows = %+v, %v", shows, err)
	}
	_, paged, total, err := ListEntries(ctx, db, owner, list.ID, ListQuery{SortBy: "added", SortHow: "desc", Page: Page{Page: 2, Limit: 2}})
	if err != nil || total != 3 || len(paged) != 1 || paged[0].ID != movie.ID {
		t.Errorf("second page newest first = %+v, %d, %v", paged, total, err)
	}

	if err := ReorderList(ctx, db, owner, list.ID, []int64{ep.ID, movie.ID}); !errors.Is(err, ErrListOrder) {
		t.Errorf("order missing an item = %v", err)
	}
	if err := ReorderList(ctx, db, owner, list.ID, []int64{ep.ID, movie.ID, movie.ID}); !errors.Is(err, ErrListOrder) {
		t.Errorf("order naming an item twice = %v", err)
	}
	if err := ReorderList(ctx, db, owner, list.ID, []int64{ep.ID, movie.ID, series.ID}); err != nil {
		t.Fatal(err)
	}
	if err := SetListNotes(ctx, db, owner, list.ID, series.ID, "  From the start "); err != nil {
		t.Fatal(err)
	}
	_, entries, _, _ = ListEntries(ctx, db, owner, list.ID, ListQuery{})
	if entries[0].ID != ep.ID || entries[1].ID != movie.ID || entries[2].Rank != 3 || *entries[2].Notes != "From the start" {
		t.Errorf("after reordering = %+v", entries)
	}

	if err := RemoveListing(ctx, db, owner, movie.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("removing a list item through the watchlist = %v", err)
	}
	if err := RemoveFromList(ctx, db, owner, list.ID, movie.ID); err != nil {
		t.Fatal(err)
	}
	if err := RemoveFromList(ctx, db, owner, list.ID, movie.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("removing twice = %v", err)
	}
	if got, _ := GetList(ctx, db, owner, list.ID); got.ItemCount != 2 {
		t.Errorf("item count = %d", got.ItemCount)
	}
}

func TestListPrivacy(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	accountID, _ := signedUpAccount(t, db)
	_, profiles, err := Me(ctx, db, accountID)
	if err != nil {
		t.Fatal(err)
	}
	owner := profiles[0].ID
	kid, err := AddProfile(ctx, db, accountID, NewProfile{Name: "Ruby", Kind: "kids"})
	if err != nil {
		t.Fatal(err)
	}
	otherAccount, _ := signedUpOther(t, db)
	_, others, err := Me(ctx, db, otherAccount)
	if err != nil {
		t.Fatal(err)
	}
	stranger := others[0].ID

	private, _ := CreateList(ctx, db, owner, ListChanges{Name: ptr("Mine")})
	family, _ := CreateList(ctx, db, owner, ListChanges{Name: ptr("Family"), Privacy: ptr("household")})
	public, _ := CreateList(ctx, db, owner, ListChanges{Name: ptr("Everyone"), Privacy: ptr("public")})
	if _, err := AddToList(ctx, db, owner, public.ID, []Listing{{Ref: inception()}}); err != nil {
		t.Fatal(err)
	}

	if _, err := CreateList(ctx, db, kid.ID, ListChanges{Name: ptr("Ruby's"), Privacy: ptr("public")}); !errors.Is(err, ErrPublicList) {
		t.Errorf("kids making a public list = %v", err)
	}
	rubys, err := CreateList(ctx, db, kid.ID, ListChanges{Name: ptr("Ruby's"), Privacy: ptr("household")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateList(ctx, db, kid.ID, rubys.ID, ListChanges{Privacy: ptr("public")}); !errors.Is(err, ErrPublicList) {
		t.Errorf("kids making a list public later = %v", err)
	}

	seen, err := Lists(ctx, db, kid.ID)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, l := range seen {
		names = append(names, l.Name+":"+l.Access)
	}
	if strings.Join(names, ",") != "Ruby's:mine,Everyone:household,Family:household" {
		t.Errorf("what the kids profile sees = %v", names)
	}
	if _, err := GetList(ctx, db, kid.ID, private.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("someone else's private list = %v", err)
	}
	if _, err := UpdateList(ctx, db, kid.ID, family.ID, ListChanges{Name: ptr("Taken over")}); !errors.Is(err, ErrNotFound) {
		t.Errorf("changing someone else's household list = %v", err)
	}
	if _, err := AddToList(ctx, db, kid.ID, family.ID, []Listing{{Ref: inception()}}); !errors.Is(err, ErrNotFound) {
		t.Errorf("adding to someone else's household list = %v", err)
	}

	if seen, _ := Lists(ctx, db, stranger); len(seen) != 0 {
		t.Errorf("another household's lists leaked: %+v", seen)
	}
	if _, err := GetList(ctx, db, stranger, family.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("a stranger reading a household list = %v", err)
	}
	got, err := GetList(ctx, db, stranger, public.ID)
	if err != nil || got.Access != "public" || got.Owner.Username != profiles[0].Username || got.ItemCount != 1 {
		t.Errorf("a stranger reading a public list = %+v, %v", got, err)
	}

	anyone, entries, _, err := ListEntries(ctx, db, 0, public.ID, ListQuery{})
	if err != nil || anyone.Watched != nil || len(entries) != 1 || entries[0].Progress != nil {
		t.Errorf("signed out = %+v, %+v, %v", anyone, entries, err)
	}
	if _, err := GetList(ctx, db, 0, family.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("signed out reading a household list = %v", err)
	}

	byName, err := ProfileLists(ctx, db, stranger, profiles[0].Username)
	if err != nil || len(byName) != 1 || byName[0].ID != public.ID {
		t.Errorf("a stranger's view of the owner's lists = %+v, %v", byName, err)
	}
	if _, err := ProfileLists(ctx, db, stranger, "nobody-here"); !errors.Is(err, ErrNotFound) {
		t.Errorf("lists for an unknown username = %v", err)
	}

	activity, err := LastActivities(ctx, db, kid.ID)
	if err != nil || activity.Marks["lists.updated_at"].IsZero() {
		t.Errorf("household activity = %+v, %v", activity, err)
	}
}
