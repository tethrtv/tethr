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

func signUp(t *testing.T, db *pgxpool.Pool) *SignInEmail {
	t.Helper()
	email, err := StartSignUp(context.Background(), db, SignUp{Email: "nathan@example.com", Name: "Nathan", Username: "nathan"}, false)
	if err != nil {
		t.Fatal(err)
	}
	return email
}

func TestSignUpThenVerifyWithLink(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()

	email := signUp(t, db)
	if email.Purpose != "sign_up" || email.To != "nathan@example.com" || email.Token == "" || len(email.Code) != 6 {
		t.Fatalf("sign-up email = %+v", email)
	}

	session, err := Verify(ctx, db, Verification{Token: email.Token}, "test")
	if err != nil {
		t.Fatal(err)
	}
	accountID, err := SessionAccount(ctx, db, session.Token)
	if err != nil {
		t.Fatal(err)
	}
	account, profiles, err := Me(ctx, db, accountID)
	if err != nil {
		t.Fatal(err)
	}
	if account.EmailVerifiedAt == nil {
		t.Error("email should be verified after using the link")
	}
	if len(profiles) != 1 || profiles[0].Username != "nathan" || profiles[0].Kind != "owner" {
		t.Errorf("profiles = %+v", profiles)
	}
}

func TestVerifyWithCode(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	email := signUp(t, db)

	if _, err := Verify(ctx, db, Verification{Email: "NATHAN@example.com", Code: email.Code}, "test"); err != nil {
		t.Fatalf("code with the email in another case: %v", err)
	}
}

func TestLinksWorkOnce(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	email := signUp(t, db)

	if _, err := Verify(ctx, db, Verification{Token: email.Token}, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(ctx, db, Verification{Token: email.Token}, "test"); !errors.Is(err, ErrInvalidLink) {
		t.Errorf("second use = %v, want ErrInvalidLink", err)
	}
	if _, err := Verify(ctx, db, Verification{Email: email.To, Code: email.Code}, "test"); !errors.Is(err, ErrInvalidLink) {
		t.Errorf("code after the link was used = %v, want ErrInvalidLink", err)
	}
}

func TestExpiredLink(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	email := signUp(t, db)

	if _, err := db.Exec(ctx, "UPDATE email_links SET expires_at = now() - interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(ctx, db, Verification{Token: email.Token}, "test"); !errors.Is(err, ErrInvalidLink) {
		t.Errorf("expired link = %v, want ErrInvalidLink", err)
	}
}

func TestWrongCodesLockTheLink(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	email := signUp(t, db)

	wrong := "000000"
	if email.Code == wrong {
		wrong = "111111"
	}
	for i := range 5 {
		if _, err := Verify(ctx, db, Verification{Email: email.To, Code: wrong}, "test"); !errors.Is(err, ErrInvalidLink) {
			t.Fatalf("wrong code %d = %v, want ErrInvalidLink", i+1, err)
		}
	}
	if _, err := Verify(ctx, db, Verification{Email: email.To, Code: email.Code}, "test"); !errors.Is(err, ErrTooManyAttempts) {
		t.Errorf("right code after five wrong ones = %v, want ErrTooManyAttempts", err)
	}
}

func TestSignUpWithExistingEmailSendsSignIn(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	signUp(t, db)

	email, err := StartSignUp(ctx, db, SignUp{Email: "nathan@example.com", Name: "Someone", Username: "someone"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if email.Purpose != "sign_in" {
		t.Errorf("purpose = %q, want sign_in", email.Purpose)
	}
	var accounts int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM accounts").Scan(&accounts); err != nil {
		t.Fatal(err)
	}
	if accounts != 1 {
		t.Errorf("%d accounts, want 1", accounts)
	}
}

func TestSignInUnknownEmailSendsNothing(t *testing.T) {
	db := dbtest.New(t)
	email, err := StartSignIn(context.Background(), db, "nobody@example.com")
	if err != nil || email != nil {
		t.Errorf("StartSignIn = %+v, %v, want nothing", email, err)
	}
}

func TestTooManyEmails(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	signUp(t, db)

	for range 2 {
		if _, err := StartSignIn(ctx, db, "nathan@example.com"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := StartSignIn(ctx, db, "nathan@example.com"); !errors.Is(err, ErrTooManyEmails) {
		t.Errorf("fourth email in a row = %v, want ErrTooManyEmails", err)
	}
}

func TestSignOut(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	email := signUp(t, db)
	session, err := Verify(ctx, db, Verification{Token: email.Token}, "test")
	if err != nil {
		t.Fatal(err)
	}

	if err := SignOut(ctx, db, session.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := SessionAccount(ctx, db, session.Token); !errors.Is(err, ErrNoSession) {
		t.Errorf("after sign-out = %v, want ErrNoSession", err)
	}
	if _, err := SessionAccount(ctx, db, "not-a-token"); !errors.Is(err, ErrNoSession) {
		t.Errorf("made-up token = %v, want ErrNoSession", err)
	}
}

func TestInvites(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	code, err := CreateInvite(ctx, db, "for sam")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		invite string
		want   error
	}{
		{"no invite", "", ErrInviteRequired},
		{"made-up invite", "NOPE-NOPE", ErrInvalidInvite},
		{"real invite in lower case", strings.ToLower(code), nil},
		{"same invite again", code, ErrInvalidInvite},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := SignUp{Email: fmt.Sprintf("sam%d@example.com", i), Name: "Sam", Username: fmt.Sprintf("sam-%d", i), Invite: tt.invite}
			if _, err := StartSignUp(ctx, db, in, true); !errors.Is(err, tt.want) {
				t.Errorf("= %v, want %v", err, tt.want)
			}
		})
	}

	if _, err := StartSignUp(ctx, db, SignUp{Email: "open@example.com", Name: "Open", Username: "open"}, false); err != nil {
		t.Errorf("open sign-up without an invite = %v", err)
	}
}
