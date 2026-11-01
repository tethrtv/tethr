package core

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/dbtest"
)

func signedUpAccount(t *testing.T, db *pgxpool.Pool) (int64, Session) {
	t.Helper()
	email := signUp(t, db)
	session, err := Verify(context.Background(), db, Verification{Token: email.Token}, "Safari on a Mac")
	if err != nil {
		t.Fatal(err)
	}
	accountID, err := SessionAccount(context.Background(), db, session.Token)
	if err != nil {
		t.Fatal(err)
	}
	return accountID, session
}

func TestAddProfile(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	accountID, _ := signedUpAccount(t, db)

	sam, err := AddProfile(ctx, db, accountID, NewProfile{Name: "Sam", Kind: "person"})
	if err != nil {
		t.Fatal(err)
	}
	if sam.Username != "nathan-sam" || sam.Kind != "person" || !sam.Private || sam.AccountID != nil {
		t.Errorf("sam = %+v", sam)
	}
	ollie, err := AddProfile(ctx, db, accountID, NewProfile{Name: "Ollie", Kind: "kids", Username: "ollie-t"})
	if err != nil {
		t.Fatal(err)
	}
	if ollie.Username != "ollie-t" || ollie.Position <= sam.Position {
		t.Errorf("ollie = %+v", ollie)
	}

	_, profiles, err := Me(ctx, db, accountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 3 || profiles[0].Kind != "owner" || profiles[2].Name != "Ollie" {
		t.Errorf("profiles = %+v", profiles)
	}

	tests := []struct {
		name string
		in   NewProfile
		want error
	}{
		{"owner kind", NewProfile{Name: "Boss", Kind: "owner"}, ErrInvalidKind},
		{"no name", NewProfile{Name: " ", Kind: "guest"}, ErrNameRequired},
		{"taken username", NewProfile{Name: "Sam", Kind: "person"}, ErrUsernameTaken},
		{"bad username", NewProfile{Name: "Zed", Kind: "guest", Username: "Zed!"}, ErrInvalidUsername},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := AddProfile(ctx, db, accountID, tt.in); !errors.Is(err, tt.want) {
				t.Errorf("= %v, want %v", err, tt.want)
			}
		})
	}
}

func TestUpdateAndRemoveProfile(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	accountID, _ := signedUpAccount(t, db)
	sam, err := AddProfile(ctx, db, accountID, NewProfile{Name: "Sam", Kind: "person"})
	if err != nil {
		t.Fatal(err)
	}

	name, username, private := "Samantha", "sam-t", false
	updated, err := UpdateProfile(ctx, db, accountID, sam.ID, ProfileChanges{Name: &name, Username: &username, Private: &private})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Samantha" || updated.Username != "sam-t" || updated.Private {
		t.Errorf("updated = %+v", updated)
	}

	other, _ := otherAccount(t, db)
	if _, err := UpdateProfile(ctx, db, other, sam.ID, ProfileChanges{Name: &name}); !errors.Is(err, ErrNotFound) {
		t.Errorf("someone else's profile = %v, want ErrNotFound", err)
	}
	if err := RemoveProfile(ctx, db, other, sam.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("removing someone else's profile = %v, want ErrNotFound", err)
	}

	_, profiles, _ := Me(ctx, db, accountID)
	if err := RemoveProfile(ctx, db, accountID, profiles[0].ID); !errors.Is(err, ErrOwnerProfile) {
		t.Errorf("removing the owner = %v, want ErrOwnerProfile", err)
	}
	if err := RemoveProfile(ctx, db, accountID, sam.ID); err != nil {
		t.Fatal(err)
	}
	if _, profiles, _ := Me(ctx, db, accountID); len(profiles) != 1 {
		t.Errorf("%d profiles after removing Sam, want 1", len(profiles))
	}
}

func otherAccount(t *testing.T, db *pgxpool.Pool) (int64, Session) {
	t.Helper()
	ctx := context.Background()
	email, err := StartSignUp(ctx, db, SignUp{Email: "other@example.com", Name: "Other", Username: "other"}, false)
	if err != nil {
		t.Fatal(err)
	}
	session, err := Verify(ctx, db, Verification{Token: email.Token}, "Firefox on Windows")
	if err != nil {
		t.Fatal(err)
	}
	id, err := SessionAccount(ctx, db, session.Token)
	if err != nil {
		t.Fatal(err)
	}
	return id, session
}

func TestSessions(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	accountID, here := signedUpAccount(t, db)
	link, err := StartSignIn(ctx, db, "nathan@example.com")
	if err != nil {
		t.Fatal(err)
	}
	phone, err := Verify(ctx, db, Verification{Token: link.Token}, "Safari on an iPhone")
	if err != nil {
		t.Fatal(err)
	}

	sessions, current, err := ListSessions(ctx, db, accountID, here.Token)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 || current == 0 {
		t.Fatalf("sessions = %+v, current %d", sessions, current)
	}

	if err := SignOutOthers(ctx, db, accountID, here.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := SessionAccount(ctx, db, phone.Token); !errors.Is(err, ErrNoSession) {
		t.Errorf("phone after signing out others = %v, want ErrNoSession", err)
	}
	if _, err := SessionAccount(ctx, db, here.Token); err != nil {
		t.Errorf("this session after signing out others = %v", err)
	}

	if err := EndSession(ctx, db, accountID, current); err != nil {
		t.Fatal(err)
	}
	if _, err := SessionAccount(ctx, db, here.Token); !errors.Is(err, ErrNoSession) {
		t.Errorf("after ending it = %v, want ErrNoSession", err)
	}
	if err := EndSession(ctx, db, accountID, current); !errors.Is(err, ErrNotFound) {
		t.Errorf("ending it twice = %v, want ErrNotFound", err)
	}
}

func TestDeleteAccount(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	accountID, session := signedUpAccount(t, db)
	if _, err := AddProfile(ctx, db, accountID, NewProfile{Name: "Sam", Kind: "person"}); err != nil {
		t.Fatal(err)
	}

	if err := DeleteAccount(ctx, db, accountID, "NATHAN"); !errors.Is(err, ErrConfirmMismatch) {
		t.Errorf("wrong confirmation = %v, want ErrConfirmMismatch", err)
	}
	if err := DeleteAccount(ctx, db, accountID, "nathan"); err != nil {
		t.Fatal(err)
	}
	if _, err := SessionAccount(ctx, db, session.Token); !errors.Is(err, ErrNoSession) {
		t.Errorf("session after deletion = %v, want ErrNoSession", err)
	}
	var left int
	if err := db.QueryRow(ctx, "SELECT (SELECT count(*) FROM accounts) + (SELECT count(*) FROM profiles) + (SELECT count(*) FROM households) + (SELECT count(*) FROM email_links)").Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Errorf("%d rows left after deleting the account", left)
	}
}

func TestPasskeyChallenges(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	wa, err := webauthn.New(&webauthn.Config{RPID: "localhost", RPDisplayName: "Tethr", RPOrigins: []string{"http://localhost:5173"}})
	if err != nil {
		t.Fatal(err)
	}
	accountID, _ := signedUpAccount(t, db)
	other, _ := otherAccount(t, db)

	options, err := BeginAddPasskey(ctx, db, wa, accountID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(options.Options), `"residentKey":"required"`) {
		t.Errorf("options should ask for a passkey that can sign in by itself: %s", options.Options)
	}
	if _, err := FinishAddPasskey(ctx, db, wa, other, options.Challenge, []byte(`{}`), ""); !errors.Is(err, ErrChallengeExpired) {
		t.Errorf("another account using the challenge = %v, want ErrChallengeExpired", err)
	}
	if _, err := FinishAddPasskey(ctx, db, wa, accountID, options.Challenge, []byte(`{}`), ""); !errors.Is(err, ErrChallengeExpired) {
		t.Errorf("challenge used twice = %v, want ErrChallengeExpired", err)
	}

	signIn, err := BeginPasskeySignIn(ctx, db, wa)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := FinishPasskeySignIn(ctx, db, wa, signIn.Challenge, []byte(`{"id":"nope"}`), "test"); !errors.Is(err, ErrPasskeyRejected) {
		t.Errorf("junk response = %v, want ErrPasskeyRejected", err)
	}
}
