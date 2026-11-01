package core

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

var (
	ErrNotFound        = errors.New("not found")
	ErrInvalidKind     = errors.New("profiles are a person, kids or guest")
	ErrOwnerProfile    = errors.New("the owner's profile can't be removed")
	ErrConfirmMismatch = errors.New("that isn't your username")
)

type NewProfile struct {
	Name     string
	Kind     string
	Username string
}

type ProfileChanges struct {
	Name     *string
	Username *string
	Private  *bool
}

func AddProfile(ctx context.Context, db *pgxpool.Pool, accountID int64, in NewProfile) (store.Profile, error) {
	in.Name = strings.TrimSpace(in.Name)
	if !slices.Contains([]string{"person", "kids", "guest"}, in.Kind) {
		return store.Profile{}, ErrInvalidKind
	}
	if in.Name == "" {
		return store.Profile{}, ErrNameRequired
	}
	var profile store.Profile
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		household, err := q.HouseholdOfAccount(ctx, accountID)
		if err != nil {
			return err
		}
		username := in.Username
		if username == "" {
			owner, err := q.OwnerProfile(ctx, accountID)
			if err != nil {
				return err
			}
			username = suggestUsername(owner.Username, in.Name)
		}
		if err := ValidUsername(username); err != nil {
			return err
		}
		position, err := q.NextProfilePosition(ctx, household.ID)
		if err != nil {
			return err
		}
		profile, err = q.CreateProfile(ctx, store.CreateProfileParams{
			HouseholdID: household.ID,
			Username:    username,
			Name:        in.Name,
			Kind:        in.Kind,
			Position:    position,
		})
		return err
	})
	if err != nil {
		return store.Profile{}, taken(err)
	}
	return profile, nil
}

func suggestUsername(owner, name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "-"):
			b.WriteByte('-')
		}
	}
	suffix := strings.Trim(b.String(), "-")
	username := owner + "-" + suffix
	if len(username) > 24 {
		username = strings.TrimRight(username[:24], "-")
	}
	return username
}

func UpdateProfile(ctx context.Context, db *pgxpool.Pool, accountID, profileID int64, c ProfileChanges) (store.Profile, error) {
	if c.Name != nil {
		name := strings.TrimSpace(*c.Name)
		if name == "" {
			return store.Profile{}, ErrNameRequired
		}
		c.Name = &name
	}
	if c.Username != nil {
		if err := ValidUsername(*c.Username); err != nil {
			return store.Profile{}, err
		}
	}
	var profile store.Profile
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		if _, err := q.HouseholdProfile(ctx, store.HouseholdProfileParams{ID: profileID, OwnerAccountID: accountID}); err != nil {
			return notFound(err)
		}
		var err error
		profile, err = q.UpdateProfile(ctx, store.UpdateProfileParams{ID: profileID, Name: c.Name, Username: c.Username, Private: c.Private})
		return err
	})
	if err != nil {
		return store.Profile{}, taken(err)
	}
	return profile, nil
}

func RemoveProfile(ctx context.Context, db *pgxpool.Pool, accountID, profileID int64) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		profile, err := q.HouseholdProfile(ctx, store.HouseholdProfileParams{ID: profileID, OwnerAccountID: accountID})
		if err != nil {
			return notFound(err)
		}
		if profile.Kind == "owner" {
			return ErrOwnerProfile
		}
		return q.DeleteProfile(ctx, profileID)
	})
}

func DeleteAccount(ctx context.Context, db *pgxpool.Pool, accountID int64, confirm string) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		owner, err := q.OwnerProfile(ctx, accountID)
		if err != nil {
			return err
		}
		if strings.TrimSpace(confirm) != owner.Username {
			return ErrConfirmMismatch
		}
		return q.DeleteAccount(ctx, accountID)
	})
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
