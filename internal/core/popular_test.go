package core

import (
	"context"
	"testing"
	"time"

	"github.com/tethrtv/tethr/internal/dbtest"
)

func TestPopular(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	now := time.Now()
	watch := func(profileID int64, ref Ref, at time.Time) {
		t.Helper()
		if _, err := AddHistory(ctx, db, Caller{ProfileID: profileID}, []Watch{{Ref: ref, At: at}}); err != nil {
			t.Fatal(err)
		}
	}
	household := func() (int64, int64) {
		t.Helper()
		var accountID, profileID int64
		err := db.QueryRow(ctx, `
			WITH a AS (INSERT INTO accounts (email, name) VALUES ('h' || gen_random_uuid() || '@example.com', 'Viewer') RETURNING id),
			h AS (INSERT INTO households (owner_account_id) SELECT id FROM a RETURNING id, owner_account_id)
			INSERT INTO profiles (household_id, account_id, username, name, kind)
			SELECT h.id, h.owner_account_id, 'v' || replace(gen_random_uuid()::text, '-', ''), 'Viewer', 'owner' FROM h
			RETURNING account_id, id`).Scan(&accountID, &profileID)
		if err != nil {
			t.Fatal(err)
		}
		return accountID, profileID
	}

	var firstAccount, first int64
	for i := range popularFloor {
		accountID, profile := household()
		if i == 0 {
			firstAccount, first = accountID, profile
		}
		watch(profile, inception(), now.Add(-time.Hour))
		if i < popularFloor-1 {
			watch(profile, episode(1, 1), now.Add(-time.Hour))
		}
		if i == 1 {
			partner, err := AddProfile(ctx, db, accountID, NewProfile{Name: "Sam", Kind: "person"})
			if err != nil {
				t.Fatal(err)
			}
			watch(partner.ID, episode(1, 2), now.Add(-2*time.Hour))
		}
	}
	kidsAccount, _ := household()
	kids, err := AddProfile(ctx, db, kidsAccount, NewProfile{Name: "Kids", Kind: "kids"})
	if err != nil {
		t.Fatal(err)
	}
	watch(kids.ID, episode(1, 1), now.Add(-time.Hour))
	_, late := household()
	watch(late, episode(1, 1), now.AddDate(0, 0, -8))

	got, err := Popular(ctx, db, first, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Households != popularFloor || got[0].Item.Kind() != "movie" {
		t.Fatalf("popular = %+v", got)
	}

	out := false
	if _, err := UpdateProfile(ctx, db, firstAccount, first, ProfileChanges{InPopular: &out}); err != nil {
		t.Fatal(err)
	}
	if got, err := Popular(ctx, db, first, now); err != nil || len(got) != 0 {
		t.Errorf("popular after one household left = %+v, %v", got, err)
	}
}
