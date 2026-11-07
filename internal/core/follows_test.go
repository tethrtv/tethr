package core

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tethrtv/tethr/internal/dbtest"
)

func TestFollowingLists(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	nathanAccount, _ := signedUpAccount(t, db)
	_, mine, _ := Me(ctx, db, nathanAccount)
	nathan := mine[0].ID
	sam, err := AddProfile(ctx, db, nathanAccount, NewProfile{Name: "Sam", Kind: "person"})
	if err != nil {
		t.Fatal(err)
	}
	kid, _ := AddProfile(ctx, db, nathanAccount, NewProfile{Name: "Ruby", Kind: "kids"})
	otherAccount, _ := otherAccount(t, db)
	_, theirs, _ := Me(ctx, db, otherAccount)
	other := theirs[0].ID

	films, _ := CreateList(ctx, db, nathan, ListChanges{Name: ptr("Slow films"), Privacy: ptr("public")})
	if _, err := AddToList(ctx, db, nathan, films.ID, []Listing{{Ref: inception()}}); err != nil {
		t.Fatal(err)
	}
	home, _ := CreateList(ctx, db, nathan, ListChanges{Name: ptr("Home"), Privacy: ptr("household")})

	seen, err := GetList(ctx, db, other, films.ID)
	if err != nil || seen.Owner.Username != "nathan" || seen.Owner.Name != "" || seen.Following || seen.Followers != 0 {
		t.Errorf("a private profile's public list from outside = %+v, %v", seen.Owner, err)
	}
	if mine, _ := GetList(ctx, db, sam.ID, films.ID); mine.Owner.Name != "Nathan" {
		t.Errorf("the household sees the name = %+v", mine.Owner)
	}

	followed, err := FollowList(ctx, db, other, films.ID)
	if err != nil || !followed.Following || followed.Followers != 1 {
		t.Errorf("follow = %+v, %v", followed, err)
	}
	if _, err := FollowList(ctx, db, other, films.ID); err != nil {
		t.Errorf("following twice = %v", err)
	}
	if _, err := FollowList(ctx, db, sam.ID, films.ID); !errors.Is(err, ErrFollowHousehold) {
		t.Errorf("following a household list = %v", err)
	}
	if _, err := FollowList(ctx, db, other, home.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("following a list you can't see = %v", err)
	}
	if _, err := UpdateProfile(ctx, db, otherAccount, other, ProfileChanges{Private: ptr(false)}); err != nil {
		t.Fatal(err)
	}

	lists, err := FollowedLists(ctx, db, other)
	if err != nil || len(lists) != 1 || lists[0].ID != films.ID {
		t.Errorf("followed lists = %+v, %v", lists, err)
	}
	followers, err := ListFollowers(ctx, db, nathan, films.ID)
	if err != nil || len(followers) != 1 || followers[0].Username != "other" || followers[0].Name != "Other" {
		t.Errorf("followers = %+v, %v", followers, err)
	}
	if _, err := ListFollowers(ctx, db, other, films.ID); !errors.Is(err, ErrFollowersOwner) {
		t.Errorf("someone else asking for followers = %v", err)
	}

	if _, err := UpdateList(ctx, db, nathan, films.ID, ListChanges{Privacy: ptr("private")}); err != nil {
		t.Fatal(err)
	}
	if lists, _ := FollowedLists(ctx, db, other); len(lists) != 0 {
		t.Errorf("a followed list that went private = %+v", lists)
	}
	if _, err := UpdateList(ctx, db, nathan, films.ID, ListChanges{Privacy: ptr("public")}); err != nil {
		t.Fatal(err)
	}
	if err := UnfollowList(ctx, db, other, films.ID); err != nil {
		t.Fatal(err)
	}
	if l, _ := GetList(ctx, db, other, films.ID); l.Following || l.Followers != 0 {
		t.Errorf("after unfollowing = %+v", l)
	}

	if _, _, err := BrowseLists(ctx, db, kid.ID, BrowseQuery{Page: Page{Page: 1, Limit: 20}}); !errors.Is(err, ErrOutsideHousehold) {
		t.Errorf("kids browsing = %v", err)
	}
	if _, err := ProfilePage(ctx, db, kid.ID, "other"); !errors.Is(err, ErrOutsideHousehold) {
		t.Errorf("kids opening a stranger's page = %v", err)
	}
}

func TestBrowsingLists(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	nathanAccount, _ := signedUpAccount(t, db)
	_, mine, _ := Me(ctx, db, nathanAccount)
	nathan := mine[0].ID
	otherAccount, _ := otherAccount(t, db)
	_, theirs, _ := Me(ctx, db, otherAccount)
	other := theirs[0].ID

	add := func(name, privacy string, items bool) List {
		l, err := CreateList(ctx, db, nathan, ListChanges{Name: ptr(name), Privacy: ptr(privacy)})
		if err != nil {
			t.Fatal(err)
		}
		if items {
			if _, err := AddToList(ctx, db, nathan, l.ID, []Listing{{Ref: inception()}}); err != nil {
				t.Fatal(err)
			}
		}
		return l
	}
	quiet := add("Quiet 100% films", "public", true)
	loud := add("Loud films", "public", true)
	add("Empty", "public", false)
	add("Ours", "household", true)
	if _, err := FollowList(ctx, db, other, quiet.ID); err != nil {
		t.Fatal(err)
	}

	names := func(q BrowseQuery) string {
		q.Page = Page{Page: 1, Limit: 20}
		lists, total, err := BrowseLists(ctx, db, other, q)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, l := range lists {
			out = append(out, l.Name)
		}
		if total != len(lists) {
			t.Errorf("total %d for %d lists", total, len(lists))
		}
		return strings.Join(out, ", ")
	}
	if got := names(BrowseQuery{Popular: true}); got != "Quiet 100% films, Loud films" {
		t.Errorf("most followed = %q", got)
	}
	if got := names(BrowseQuery{}); got != "Loud films, Quiet 100% films" {
		t.Errorf("newest = %q", got)
	}
	if got := names(BrowseQuery{Search: "100%"}); got != "Quiet 100% films" {
		t.Errorf("search with a percent sign = %q", got)
	}
	if got := names(BrowseQuery{Search: "LOUD"}); got != "Loud films" {
		t.Errorf("search ignoring case = %q", got)
	}

	r, isNew, err := ReportList(ctx, db, other, loud.ID, "spam", "  Links to a scam  ")
	if err != nil || !isNew || r.Reporter != "other" || *r.Note != "Links to a scam" {
		t.Fatalf("report = %+v, %v, %v", r, isNew, err)
	}
	if _, isNew, err := ReportList(ctx, db, other, loud.ID, "spam", ""); err != nil || isNew {
		t.Errorf("reporting twice = %v, %v", isNew, err)
	}
	if got := names(BrowseQuery{}); got != "Quiet 100% films" {
		t.Errorf("a reported list leaves browsing = %q", got)
	}
	if l, err := GetList(ctx, db, other, loud.ID); err != nil || l.Privacy != "public" {
		t.Errorf("a reported list still opens by link = %v", err)
	}
	for _, c := range []struct {
		reason, note string
		want         error
	}{
		{"rude", "", ErrReportReason},
		{"other", strings.Repeat("a", 501), ErrReportNote},
	} {
		if _, _, err := ReportList(ctx, db, other, quiet.ID, c.reason, c.note); !errors.Is(err, c.want) {
			t.Errorf("report %q = %v, want %v", c.reason, err, c.want)
		}
	}
	if _, _, err := ReportList(ctx, db, nathan, quiet.ID, "spam", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("reporting your own list = %v", err)
	}

	open, err := OpenReports(ctx, db)
	if err != nil || len(open) != 1 || open[0].ListID != loud.ID || *open[0].ReporterUsername != "other" {
		t.Errorf("open reports = %+v, %v", open, err)
	}
	if err := KeepList(ctx, db, loud.ID); err != nil {
		t.Fatal(err)
	}
	if got := names(BrowseQuery{}); got != "Loud films, Quiet 100% films" {
		t.Errorf("a kept list is back = %q", got)
	}
	if err := KeepList(ctx, db, loud.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("keeping a list with no open reports = %v", err)
	}

	if _, _, err := ReportList(ctx, db, other, loud.ID, "harassment", ""); err != nil {
		t.Fatal(err)
	}
	removed, err := RemoveList(ctx, db, loud.ID)
	if err != nil || removed.Email != "nathan@example.com" || removed.Name != "Loud films" {
		t.Errorf("remove = %+v, %v", removed, err)
	}
	if _, err := GetList(ctx, db, other, loud.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("a removed list is private = %v", err)
	}
	if open, _ := OpenReports(ctx, db); len(open) != 0 {
		t.Errorf("reports left open = %+v", open)
	}

	page, err := ProfilePage(ctx, db, other, "nathan")
	if err != nil || page.Name != "" || !page.Private || len(page.Lists) != 2 {
		t.Errorf("profile page from outside = %+v, %v", page, err)
	}
	own, err := ProfilePage(ctx, db, nathan, "nathan")
	if err != nil || own.Name != "Nathan" || len(own.Lists) != 4 {
		t.Errorf("your own profile page = %d lists, %v", len(own.Lists), err)
	}
	if _, err := ProfilePage(ctx, db, other, "nobody"); !errors.Is(err, ErrNotFound) {
		t.Errorf("no such username = %v", err)
	}
}
