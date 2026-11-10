package core

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/dbtest"
	"github.com/tethrtv/tethr/internal/store"
)

func household(t *testing.T, db *pgxpool.Pool, username string) (accountID, profileID int64) {
	t.Helper()
	ctx := context.Background()
	email, err := StartSignUp(ctx, db, SignUp{Email: username + "@example.com", Name: username, Username: username}, false)
	if err != nil {
		t.Fatal(err)
	}
	session, err := Verify(ctx, db, Verification{Token: email.Token}, "test")
	if err != nil {
		t.Fatal(err)
	}
	accountID, err = SessionAccount(ctx, db, session.Token)
	if err != nil {
		t.Fatal(err)
	}
	_, profiles, err := Me(ctx, db, accountID)
	if err != nil {
		t.Fatal(err)
	}
	return accountID, profiles[0].ID
}

func TestReportAbuse(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	_, owner := household(t, db, "owner")
	_, viewer := household(t, db, "viewer")
	ann, annie := household(t, db, "ann")
	_, bob := household(t, db, "bob")
	_, cat := household(t, db, "cat")

	list := func(name string) List {
		l, err := CreateList(ctx, db, owner, ListChanges{Name: ptr(name), Privacy: ptr("public")})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := AddToList(ctx, db, owner, l.ID, []Listing{{Ref: inception()}}); err != nil {
			t.Fatal(err)
		}
		return l
	}
	browsable := func(l List) bool {
		t.Helper()
		lists, _, err := BrowseLists(ctx, db, viewer, BrowseQuery{Page: Page{Page: 1, Limit: 100}})
		if err != nil {
			t.Fatal(err)
		}
		return slices.ContainsFunc(lists, func(b List) bool { return b.ID == l.ID })
	}
	report := func(from int64, l List, reason string) Report {
		t.Helper()
		r, _, err := ReportList(ctx, db, from, l.ID, reason, "")
		if err != nil {
			t.Fatal(err)
		}
		return r
	}

	spam := list("Spam")
	if r := report(annie, spam, "spam"); r.Hid || !browsable(spam) {
		t.Error("one spam report hid the list")
	}
	annSecond, err := AddProfile(ctx, db, ann, NewProfile{Name: "Second", Kind: "person"})
	if err != nil {
		t.Fatal(err)
	}
	if _, isNew, err := ReportList(ctx, db, annSecond.ID, spam.ID, "harassment", ""); err != nil || isNew {
		t.Errorf("a second profile in the same household reporting again = %v, %v", isNew, err)
	}
	if r := report(bob, spam, "other"); r.Hid || !browsable(spam) {
		t.Error("two households hid the list")
	}
	if r := report(cat, spam, "spam"); !r.Hid || browsable(spam) {
		t.Error("three households didn't hide the list")
	}

	personal := list("Personal")
	if r := report(annie, personal, "personal"); !r.Hid || browsable(personal) {
		t.Error("a report of personal details didn't hide the list")
	}
	if r := report(bob, personal, "illegal"); r.Hid {
		t.Error("a report on a list that's already hidden says it hid it")
	}
	leaver, gone := household(t, db, "leaver")
	doxxed := list("Doxxed")
	report(gone, doxxed, "personal")
	if err := DeleteAccount(ctx, db, leaver, "leaver"); err != nil {
		t.Fatal(err)
	}
	if browsable(doxxed) {
		t.Error("deleting the reporter's account brought the list back")
	}
	if err := KeepList(ctx, db, personal.ID); err != nil {
		t.Fatal(err)
	}
	if r := report(cat, personal, "illegal"); r.Hid || !browsable(personal) {
		t.Error("a kept list was hidden again before it changed")
	}
	if open, _ := OpenReports(ctx, db); !slices.ContainsFunc(open, func(o store.OpenReportsRow) bool { return o.ListID == personal.ID }) {
		t.Error("a report on a kept list didn't reach the queue")
	}
	if _, err := UpdateList(ctx, db, owner, personal.ID, ListChanges{Name: ptr("Personal, renamed")}); err != nil {
		t.Fatal(err)
	}
	if browsable(personal) {
		t.Error("a kept list that changed wasn't hidden by the report waiting on it")
	}

	for _, name := range []string{"One", "Two"} {
		l := list(name)
		report(bob, l, "spam")
		if err := KeepList(ctx, db, l.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := KeepList(ctx, db, spam.ID); err != nil {
		t.Fatal(err)
	}
	if r := report(bob, list("Fine"), "illegal"); r.Hid {
		t.Error("a household with three kept reports still hides lists")
	}
	if r := report(cat, list("Also fine"), "illegal"); !r.Hid {
		t.Error("another household's report stopped hiding lists")
	}

	for i, name := range []string{"A", "B", "C"} {
		from := annie
		if i%2 == 1 {
			from = annSecond.ID
		}
		report(from, list(name), "other")
	}
	if _, _, err := ReportList(ctx, db, annSecond.ID, list("D").ID, "other", ""); !errors.Is(err, ErrTooManyReports) {
		t.Errorf("a household's sixth report today = %v", err)
	}
}
