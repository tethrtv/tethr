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
}

func ValidUsername(s string) error {
	if !usernamePattern.MatchString(s) || reservedUsernames[s] {
		return ErrInvalidUsername
	}
	return nil
}

func CreateAccount(ctx context.Context, db *pgxpool.Pool, in SignUp) (store.Account, store.Profile, error) {
	email := strings.TrimSpace(in.Email)
	name := strings.TrimSpace(in.Name)
	if at := strings.Index(email, "@"); at < 1 || at == len(email)-1 || strings.ContainsAny(email, " \t\n") {
		return store.Account{}, store.Profile{}, ErrInvalidEmail
	}
	if name == "" {
		return store.Account{}, store.Profile{}, ErrNameRequired
	}
	if err := ValidUsername(in.Username); err != nil {
		return store.Account{}, store.Profile{}, err
	}

	var account store.Account
	var owner store.Profile
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		var err error
		account, err = q.CreateAccount(ctx, store.CreateAccountParams{Email: email, Name: name})
		if err != nil {
			return err
		}
		household, err := q.CreateHousehold(ctx, account.ID)
		if err != nil {
			return err
		}
		owner, err = q.CreateProfile(ctx, store.CreateProfileParams{
			HouseholdID: household.ID,
			AccountID:   &account.ID,
			Username:    in.Username,
			Name:        name,
			Kind:        "owner",
		})
		return err
	})
	if err != nil {
		return store.Account{}, store.Profile{}, taken(err)
	}
	return account, owner, nil
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
