package core

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

var (
	ErrInvalidEmail    = errors.New("invalid email address")
	ErrInvalidUsername = errors.New("usernames are 2 to 24 lowercase letters, numbers and dashes")
	ErrNameRequired    = errors.New("name is required")
	ErrEmailTaken      = errors.New("email address already has an account")
	ErrUsernameTaken   = errors.New("username is taken")
	ErrInviteRequired  = errors.New("sign-ups need an invite for now")
	ErrInvalidInvite   = errors.New("that invite code isn't valid or has been used")
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,23}$`)

var reservedUsernames = map[string]bool{
	"about": true, "admin": true, "api": true, "help": true, "me": true,
	"settings": true, "staging": true, "support": true, "tethr": true, "www": true,
}

type SignUp struct {
	Email    string
	Name     string
	Username string
	Invite   string
}

func ValidUsername(s string) error {
	if !usernamePattern.MatchString(s) || reservedUsernames[s] {
		return ErrInvalidUsername
	}
	return nil
}

func CreateAccount(ctx context.Context, db *pgxpool.Pool, in SignUp) (store.Account, store.Profile, error) {
	in, err := in.clean()
	if err != nil {
		return store.Account{}, store.Profile{}, err
	}
	var account store.Account
	var owner store.Profile
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		var err error
		account, owner, err = createAccount(ctx, store.New(tx), in)
		return err
	})
	if err != nil {
		return store.Account{}, store.Profile{}, taken(err)
	}
	return account, owner, nil
}

func (in SignUp) clean() (SignUp, error) {
	in.Email = strings.TrimSpace(in.Email)
	in.Name = strings.TrimSpace(in.Name)
	if !validEmail(in.Email) {
		return in, ErrInvalidEmail
	}
	if in.Name == "" {
		return in, ErrNameRequired
	}
	return in, ValidUsername(in.Username)
}

func validEmail(s string) bool {
	at := strings.Index(s, "@")
	return at > 0 && at < len(s)-1 && !strings.ContainsAny(s, " \t\n")
}

func createAccount(ctx context.Context, q *store.Queries, in SignUp) (store.Account, store.Profile, error) {
	account, err := q.CreateAccount(ctx, store.CreateAccountParams{Email: in.Email, Name: in.Name})
	if err != nil {
		return store.Account{}, store.Profile{}, err
	}
	household, err := q.CreateHousehold(ctx, account.ID)
	if err != nil {
		return store.Account{}, store.Profile{}, err
	}
	owner, err := q.CreateProfile(ctx, store.CreateProfileParams{
		HouseholdID: household.ID,
		AccountID:   &account.ID,
		Username:    in.Username,
		Name:        in.Name,
		Kind:        "owner",
	})
	return account, owner, err
}

func taken(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return err
	}
	switch pgErr.ConstraintName {
	case "accounts_email_key":
		return ErrEmailTaken
	case "profiles_username_key":
		return ErrUsernameTaken
	}
	return err
}
