package core

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

type MediaServer struct {
	ID          int64
	Kind        string
	Name        *string
	CreatedAt   time.Time
	LastEventAt *time.Time
	Users       []ServerUser
}

type ServerUser struct {
	ID          int64
	Name        string
	ProfileID   *int64
	LastEventAt time.Time
}

type ServerEvent struct {
	Kind       string
	Secret     string
	ServerName string
	UserID     string
	UserName   string
}

func AddMediaServer(ctx context.Context, db *pgxpool.Pool, accountID int64, kind string) (MediaServer, string, error) {
	secret, err := randomToken()
	if err != nil {
		return MediaServer{}, "", err
	}
	s, err := store.New(db).CreateMediaServer(ctx, store.CreateMediaServerParams{AccountID: accountID, Kind: kind, SecretHash: hash(secret)})
	if err != nil {
		return MediaServer{}, "", err
	}
	return MediaServer{ID: s.ID, Kind: s.Kind, CreatedAt: s.CreatedAt, Users: []ServerUser{}}, secret, nil
}

func MediaServers(ctx context.Context, db *pgxpool.Pool, accountID int64) ([]MediaServer, error) {
	q := store.New(db)
	servers, err := q.AccountMediaServers(ctx, accountID)
	if err != nil {
		return nil, err
	}
	users, err := q.MediaServerUsers(ctx, accountID)
	if err != nil {
		return nil, err
	}
	out := make([]MediaServer, 0, len(servers))
	for _, s := range servers {
		server := MediaServer{ID: s.ID, Kind: s.Kind, Name: s.Name, CreatedAt: s.CreatedAt, LastEventAt: s.LastEventAt, Users: []ServerUser{}}
		for _, u := range users {
			if u.ServerID == s.ID {
				server.Users = append(server.Users, ServerUser{ID: u.ID, Name: u.Name, ProfileID: u.ProfileID, LastEventAt: u.LastEventAt})
			}
		}
		out = append(out, server)
	}
	return out, nil
}

func NewMediaServerSecret(ctx context.Context, db *pgxpool.Pool, accountID, serverID int64) (string, error) {
	secret, err := randomToken()
	if err != nil {
		return "", err
	}
	n, err := store.New(db).SetMediaServerSecret(ctx, store.SetMediaServerSecretParams{ID: serverID, AccountID: accountID, SecretHash: hash(secret)})
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", ErrNotFound
	}
	return secret, nil
}

func RemoveMediaServer(ctx context.Context, db *pgxpool.Pool, accountID, serverID int64) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		n, err := q.RevokeMediaServer(ctx, store.RevokeMediaServerParams{ID: serverID, AccountID: accountID})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		return q.RevokeMediaServerGrants(ctx, serverID)
	})
}

func MatchServerUser(ctx context.Context, db *pgxpool.Pool, accountID, serverID, userID int64, profileID *int64) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		u, err := q.MediaServerUser(ctx, store.MediaServerUserParams{ID: userID, ServerID: serverID, AccountID: accountID})
		if err != nil {
			return notFound(err)
		}
		if u.GrantID != nil {
			if _, err := q.RevokeGrant(ctx, store.RevokeGrantParams{ID: *u.GrantID, OwnerAccountID: accountID}); err != nil {
				return err
			}
		}
		if profileID == nil {
			return q.MatchMediaServerUser(ctx, store.MatchMediaServerUserParams{ID: u.ID})
		}
		if _, err := q.HouseholdProfile(ctx, store.HouseholdProfileParams{ID: *profileID, OwnerAccountID: accountID}); err != nil {
			return notFound(err)
		}
		app, err := q.BuiltInApp(ctx, u.Kind)
		if err != nil {
			return err
		}
		label := u.Name
		if u.ServerName != nil {
			label += " on " + *u.ServerName
		}
		grant, err := q.CreateGrant(ctx, store.CreateGrantParams{AppID: app.ID, ProfileID: *profileID, Label: &label, ApprovedVia: "server"})
		if err != nil {
			return err
		}
		return q.MatchMediaServerUser(ctx, store.MatchMediaServerUserParams{ID: u.ID, GrantID: &grant.ID})
	})
}

// Users nobody has matched to a profile log nothing.
func ServerCaller(ctx context.Context, db *pgxpool.Pool, e ServerEvent) (Caller, bool, error) {
	q := store.New(db)
	s, err := q.MediaServerBySecret(ctx, store.MediaServerBySecretParams{SecretHash: hash(e.Secret), Kind: e.Kind})
	if err != nil {
		return Caller{}, false, notFound(err)
	}
	var name *string
	if e.ServerName != "" {
		name = &e.ServerName
	}
	if err := q.TouchMediaServer(ctx, store.TouchMediaServerParams{ID: s.ID, Name: name}); err != nil {
		return Caller{}, false, err
	}
	if e.UserID == "" {
		return Caller{}, false, nil
	}
	u, err := q.SeeMediaServerUser(ctx, store.SeeMediaServerUserParams{ServerID: s.ID, ExternalID: e.UserID, Name: e.UserName})
	if err != nil || u.GrantID == nil {
		return Caller{}, false, err
	}
	g, err := q.ServerUserGrant(ctx, *u.GrantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Caller{}, false, nil
	}
	if err != nil {
		return Caller{}, false, err
	}
	if err := q.TouchGrant(ctx, g.ID); err != nil {
		return Caller{}, false, err
	}
	return Caller{AccountID: s.AccountID, ProfileID: g.ProfileID, GrantID: g.ID}, true, nil
}
