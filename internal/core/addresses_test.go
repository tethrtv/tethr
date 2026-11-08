package core

import (
	"context"
	"errors"
	"testing"

	"github.com/tethrtv/tethr/internal/dbtest"
)

func TestListAddresses(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	nathanAccount, _ := signedUpAccount(t, db)
	_, mine, _ := Me(ctx, db, nathanAccount)
	nathan := mine[0].ID
	sam, _ := AddProfile(ctx, db, nathanAccount, NewProfile{Name: "Sam", Kind: "person"})
	otherAccount, _ := otherAccount(t, db)
	_, theirs, _ := Me(ctx, db, otherAccount)
	other := theirs[0].ID

	films, _ := CreateList(ctx, db, nathan, ListChanges{Name: ptr("Slow films"), Privacy: ptr("public")})
	home, _ := CreateList(ctx, db, nathan, ListChanges{Name: ptr("Home"), Privacy: ptr("household")})

	found, err := FindList(ctx, db, other, "Nathan", "slow-films")
	if err != nil || found.ID != films.ID {
		t.Errorf("find by address = %+v, %v", found, err)
	}
	if _, err := FindList(ctx, db, other, "nathan", "home"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a household list from outside = %v", err)
	}
	if l, err := FindList(ctx, db, sam.ID, "nathan", "home"); err != nil || l.ID != home.ID {
		t.Errorf("a household list from inside = %+v, %v", l, err)
	}
	if _, err := FindList(ctx, db, other, "nobody", "slow-films"); !errors.Is(err, ErrNotFound) {
		t.Errorf("an unknown username = %v", err)
	}

	if _, err := UpdateList(ctx, db, nathan, films.ID, ListChanges{Name: ptr("Slower films")}); err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateProfile(ctx, db, nathanAccount, nathan, ProfileChanges{Username: ptr("nate")}); err != nil {
		t.Fatal(err)
	}
	for _, old := range [][2]string{{"nathan", "slow-films"}, {"nate", "slow-films"}, {"nathan", "slower-films"}} {
		l, err := FindList(ctx, db, other, old[0], old[1])
		if err != nil || l.ID != films.ID || l.Slug != "slower-films" || l.Owner.Username != "nate" {
			t.Errorf("old address %v = %+v, %v", old, l, err)
		}
	}
	if p, err := ProfilePage(ctx, db, other, "nathan"); err != nil || p.Username != "nate" {
		t.Errorf("profile at an old username = %+v, %v", p, err)
	}

	again, _ := CreateList(ctx, db, nathan, ListChanges{Name: ptr("Slow films"), Privacy: ptr("public")})
	if l, err := FindList(ctx, db, other, "nate", "slow-films"); err != nil || l.ID != again.ID {
		t.Errorf("a new list takes back an old slug = %+v, %v", l, err)
	}

	if _, err := UpdateProfile(ctx, db, otherAccount, other, ProfileChanges{Username: ptr("nathan")}); err != nil {
		t.Fatal(err)
	}
	if p, err := ProfilePage(ctx, db, nathan, "nathan"); err != nil || p.Username != "nathan" {
		t.Errorf("someone else takes the old username = %+v, %v", p, err)
	}
	if _, err := FindList(ctx, db, nathan, "nathan", "slower-films"); !errors.Is(err, ErrNotFound) {
		t.Errorf("an old address once the username is taken = %v", err)
	}
	if _, err := UpdateProfile(ctx, db, otherAccount, other, ProfileChanges{Username: ptr("other")}); err != nil {
		t.Fatal(err)
	}
	if p, err := ProfilePage(ctx, db, nathan, "nathan"); err != nil || p.Username != "other" {
		t.Errorf("a username given up twice follows the last owner = %+v, %v", p, err)
	}
}
