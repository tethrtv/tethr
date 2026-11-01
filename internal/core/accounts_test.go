package core

import (
	"context"
	"errors"
	"testing"

	"github.com/tethrtv/tethr/internal/dbtest"
)

func TestValidUsername(t *testing.T) {
	tests := []struct {
		in   string
		want error
	}{
		{"nathan", nil},
		{"sam-2", nil},
		{"n8", nil},
		{"a", ErrInvalidUsername},
		{"-sam", ErrInvalidUsername},
		{"Sam", ErrInvalidUsername},
		{"sam_t", ErrInvalidUsername},
		{"abcdefghijklmnopqrstuvwxy", ErrInvalidUsername},
		{"admin", ErrInvalidUsername},
		{"tethr", ErrInvalidUsername},
	}
	for _, tt := range tests {
		if got := ValidUsername(tt.in); !errors.Is(got, tt.want) {
			t.Errorf("ValidUsername(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestCreateAccount(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()

	account, owner, err := CreateAccount(ctx, db, SignUp{Email: " Nathan@Example.com ", Name: "Nathan", Username: "nathan"})
	if err != nil {
		t.Fatal(err)
	}
	if account.Email != "Nathan@Example.com" {
		t.Errorf("email = %q, want it trimmed", account.Email)
	}
	if owner.Kind != "owner" || owner.Username != "nathan" || owner.Name != "Nathan" {
		t.Errorf("owner profile = %+v", owner)
	}
	if owner.AccountID == nil || *owner.AccountID != account.ID {
		t.Errorf("owner profile account = %v, want %d", owner.AccountID, account.ID)
	}
	if !owner.Private {
		t.Error("owner profile should be private by default")
	}
}

func TestCreateAccountTaken(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()

	if _, _, err := CreateAccount(ctx, db, SignUp{Email: "nathan@example.com", Name: "Nathan", Username: "nathan"}); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		in   SignUp
		want error
	}{
		{"same email in other case", SignUp{Email: "NATHAN@example.com", Name: "N", Username: "other"}, ErrEmailTaken},
		{"same username", SignUp{Email: "sam@example.com", Name: "Sam", Username: "nathan"}, ErrUsernameTaken},
		{"bad email", SignUp{Email: "sam", Name: "Sam", Username: "sam"}, ErrInvalidEmail},
		{"no name", SignUp{Email: "sam@example.com", Name: " ", Username: "sam"}, ErrNameRequired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := CreateAccount(ctx, db, tt.in); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}

	var accounts, households int
	if err := db.QueryRow(ctx, "SELECT (SELECT count(*) FROM accounts), (SELECT count(*) FROM households)").Scan(&accounts, &households); err != nil {
		t.Fatal(err)
	}
	if accounts != 1 || households != 1 {
		t.Errorf("after failed sign-ups: %d accounts, %d households, want 1 of each", accounts, households)
	}
}
