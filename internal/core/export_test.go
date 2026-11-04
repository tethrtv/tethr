package core

import (
	"context"
	"errors"
	"testing"

	"github.com/tethrtv/tethr/internal/dbtest"
)

func TestStartExport(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	accountID, session := signedUpAccount(t, db)

	for range 2 {
		start, err := StartExport(ctx, db, session.Token)
		if err != nil || start.AccountID != accountID || start.Email == "" || start.Device != "Safari on a Mac" {
			t.Fatalf("start = %+v, %v", start, err)
		}
	}
	if _, err := StartExport(ctx, db, session.Token); !errors.Is(err, ErrTooManyExports) {
		t.Errorf("a third export in a day = %v", err)
	}

	if _, err := db.Exec(ctx, `UPDATE audit_log SET created_at = now() - interval '25 hours' WHERE kind = 'export'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE sessions SET created_at = now() - interval '11 minutes'`); err != nil {
		t.Fatal(err)
	}
	if _, err := StartExport(ctx, db, session.Token); !errors.Is(err, ErrSignInAgain) {
		t.Errorf("an old session = %v", err)
	}
	if _, err := StartExport(ctx, db, "nope"); !errors.Is(err, ErrNoSession) {
		t.Errorf("no session = %v", err)
	}
}

func TestExport(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	caller, _, _ := connectedProfile(t, db)
	if _, err := AddHistory(ctx, db, caller, []Watch{{Ref: episode(1, 1), At: *aired(5)}, {Ref: inception(), At: *aired(2)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := ScrobblePause(ctx, db, caller, Scrobble{Ref: episode(1, 2), Progress: 40}); err != nil {
		t.Fatal(err)
	}
	if _, err := Rate(ctx, db, caller.ProfileID, []Rating{{Ref: inception(), Rating: 8}}); err != nil {
		t.Fatal(err)
	}
	if _, err := AddToWatchlist(ctx, db, caller.ProfileID, []Listing{{Ref: Ref{Kind: "movie", IDs: IDs{TMDB: 194}}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := Hide(ctx, db, caller.ProfileID, "progress_watched", []Ref{{Kind: "show", IDs: IDs{TMDB: 1396}}}); err != nil {
		t.Fatal(err)
	}
	var accountID int64
	if err := db.QueryRow(ctx, `SELECT h.owner_account_id FROM profiles p JOIN households h ON h.id = p.household_id WHERE p.id = $1`, caller.ProfileID).Scan(&accountID); err != nil {
		t.Fatal(err)
	}
	if _, err := AddProfile(ctx, db, accountID, NewProfile{Name: "Sam", Kind: "person"}); err != nil {
		t.Fatal(err)
	}

	e, err := Export(ctx, db, accountID)
	if err != nil {
		t.Fatal(err)
	}
	if e.Account.ID != accountID || len(e.Connections) != 1 || len(e.Profiles) != 2 {
		t.Fatalf("export = %+v", e)
	}
	owner := e.Profiles[0]
	if owner.Profile.ID != caller.ProfileID || len(owner.History) != 2 || len(owner.Playback) != 1 || len(owner.Ratings) != 1 || len(owner.Watchlist) != 1 {
		t.Errorf("owner = %+v", owner)
	}
	if len(owner.Hidden) != 1 || owner.Hidden[0].Section != "progress_watched" {
		t.Errorf("hidden = %+v", owner.Hidden)
	}
	if sam := e.Profiles[1]; sam.Profile.Name != "Sam" || len(sam.History) != 0 {
		t.Errorf("Sam = %+v", sam)
	}
}
