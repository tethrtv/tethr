package core

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

var (
	ErrAppName     = errors.New("apps need a name of up to 60 characters")
	ErrRedirectURI = errors.New("redirect uris must be https, a custom scheme, or http on localhost")
	ErrTooManyApps = errors.New("an account can register up to 10 apps")
)

const maxApps = 10

type AppChanges struct {
	Name         *string
	RedirectURIs *[]string
}

func CreateApp(ctx context.Context, db *pgxpool.Pool, name string, redirectURIs []string) (App, error) {
	app, _, err := createApp(ctx, store.New(db), nil, name, redirectURIs)
	return app, err
}

func RegisterApp(ctx context.Context, db *pgxpool.Pool, accountID int64, name string, redirectURIs []string) (App, store.App, error) {
	var app App
	var row store.App
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", accountID); err != nil {
			return err
		}
		n, err := q.CountAccountApps(ctx, &accountID)
		if err != nil {
			return err
		}
		if n >= maxApps {
			return ErrTooManyApps
		}
		app, row, err = createApp(ctx, q, &accountID, name, redirectURIs)
		return err
	})
	return app, row, err
}

func createApp(ctx context.Context, q *store.Queries, owner *int64, name string, redirectURIs []string) (App, store.App, error) {
	name, redirectURIs, err := cleanApp(name, redirectURIs)
	if err != nil {
		return App{}, store.App{}, err
	}
	id, err := randomToken()
	if err != nil {
		return App{}, store.App{}, err
	}
	secret, err := randomToken()
	if err != nil {
		return App{}, store.App{}, err
	}
	clientID := "tz_" + strings.NewReplacer("-", "", "_", "").Replace(id)[:24]
	row, err := q.CreateApp(ctx, store.CreateAppParams{
		OwnerAccountID:   owner,
		ClientID:         clientID,
		ClientSecretHash: hash(secret),
		Name:             name,
		RedirectUris:     redirectURIs,
	})
	return App{ClientID: clientID, ClientSecret: secret}, row, err
}

func cleanApp(name string, redirectURIs []string) (string, []string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 60 {
		return "", nil, ErrAppName
	}
	uris, err := cleanRedirects(redirectURIs)
	return name, uris, err
}

func cleanRedirects(redirectURIs []string) ([]string, error) {
	uris := []string{}
	for _, raw := range redirectURIs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if !validRedirect(raw) {
			return nil, ErrRedirectURI
		}
		uris = append(uris, raw)
	}
	if len(uris) > 10 {
		return nil, ErrRedirectURI
	}
	return uris, nil
}

func validRedirect(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 500 || u.Fragment != "" || u.Scheme == "" {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return u.Host != ""
	case "http":
		host := u.Hostname()
		return host == "localhost" || host == "127.0.0.1" || host == "::1"
	case "javascript", "data", "file", "vbscript", "blob", "about":
		return false
	}
	return true
}

func ListApps(ctx context.Context, db *pgxpool.Pool, accountID int64) ([]store.AccountAppsRow, error) {
	return store.New(db).AccountApps(ctx, &accountID)
}

func UpdateApp(ctx context.Context, db *pgxpool.Pool, accountID, id int64, c AppChanges) (store.App, error) {
	params := store.UpdateAppParams{ID: id, OwnerAccountID: &accountID}
	if c.Name != nil {
		name, _, err := cleanApp(*c.Name, nil)
		if err != nil {
			return store.App{}, err
		}
		params.Name = &name
	}
	if c.RedirectURIs != nil {
		uris, err := cleanRedirects(*c.RedirectURIs)
		if err != nil {
			return store.App{}, err
		}
		params.RedirectUris = uris
	}
	row, err := store.New(db).UpdateApp(ctx, params)
	return row, notFound(err)
}

func RevokeApp(ctx context.Context, db *pgxpool.Pool, accountID, id int64) error {
	n, err := store.New(db).RevokeApp(ctx, store.RevokeAppParams{ID: id, OwnerAccountID: &accountID})
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

func NewAppSecret(ctx context.Context, db *pgxpool.Pool, accountID, id int64) (string, error) {
	secret, err := randomToken()
	if err != nil {
		return "", err
	}
	n, err := store.New(db).SetAppSecret(ctx, store.SetAppSecretParams{ID: id, OwnerAccountID: &accountID, ClientSecretHash: hash(secret)})
	if err == nil && n == 0 {
		return "", ErrNotFound
	}
	return secret, err
}
