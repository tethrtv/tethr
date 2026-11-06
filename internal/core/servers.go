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
	Ignored     bool
	Held        int64
	LastEventAt time.Time
}

type HeldPlay struct {
	Ref      Ref
	Action   string
	At       time.Time
	Duration *float64
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
				server.Users = append(server.Users, ServerUser{ID: u.ID, Name: u.Name, ProfileID: u.ProfileID, Ignored: u.Ignored, Held: u.Held, LastEventAt: u.LastEventAt})
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
			if err := q.DropHeldPlays(ctx, u.ID); err != nil {
				return err
			}
			return q.MatchMediaServerUser(ctx, store.MatchMediaServerUserParams{ID: u.ID, Ignored: true})
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
		if err := q.MatchMediaServerUser(ctx, store.MatchMediaServerUserParams{ID: u.ID, GrantID: &grant.ID}); err != nil {
			return err
		}
		return logHeldPlays(ctx, q, u.ID, *profileID, grant.ID)
	})
}

func logHeldPlays(ctx context.Context, q *store.Queries, serverUserID, profileID, grantID int64) error {
	held, err := q.HeldPlays(ctx, serverUserID)
	if err != nil {
		return err
	}
	for _, h := range held {
		near, err := q.WatchEventNear(ctx, store.WatchEventNearParams{ProfileID: profileID, TitleID: h.TitleID, EpisodeID: h.EpisodeID, At: h.WatchedAt})
		if err != nil {
			return err
		}
		if near {
			continue
		}
		r := resolved{}
		if h.EpisodeID == nil {
			r.title, err = q.TitleByID(ctx, store.TitleByIDParams{ID: h.TitleID, Kind: "movie"})
		} else {
			var e store.Episode
			if e, err = q.EpisodeByID(ctx, *h.EpisodeID); err == nil {
				r.episode = &e
				r.title, err = q.TitleByID(ctx, store.TitleByIDParams{ID: h.TitleID, Kind: "show"})
			}
		}
		if err != nil {
			return err
		}
		if _, err := watched(ctx, q, profileID, &grantID, r, h.WatchedAt, h.Action, h.DurationSeconds); err != nil {
			return err
		}
	}
	return q.DropHeldPlays(ctx, serverUserID)
}

// Plays from someone not matched yet are kept for a week, so the play that makes them show up isn't lost.
func HoldPlay(ctx context.Context, db *pgxpool.Pool, serverUserID int64, h HeldPlay) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		r, err := resolve(ctx, q, h.Ref, true)
		if err != nil {
			return err
		}
		near, err := q.HeldPlayNear(ctx, store.HeldPlayNearParams{ServerUserID: serverUserID, TitleID: r.title.ID, EpisodeID: r.episodeID(), At: h.At})
		if err != nil || near {
			return err
		}
		return q.HoldPlay(ctx, store.HoldPlayParams{ServerUserID: serverUserID, TitleID: r.title.ID, EpisodeID: r.episodeID(), Action: h.Action, WatchedAt: h.At, DurationSeconds: h.Duration})
	})
}

func PruneHeldPlays(ctx context.Context, db *pgxpool.Pool) error {
	return store.New(db).PruneHeldPlays(ctx)
}

// Someone not matched yet comes back as an ID to hold their plays under, and someone set not to log as neither.
func ServerCaller(ctx context.Context, db *pgxpool.Pool, e ServerEvent) (Caller, int64, error) {
	q := store.New(db)
	s, err := q.MediaServerBySecret(ctx, store.MediaServerBySecretParams{SecretHash: hash(e.Secret), Kind: e.Kind})
	if err != nil {
		return Caller{}, 0, notFound(err)
	}
	var name *string
	if e.ServerName != "" {
		name = &e.ServerName
	}
	if err := q.TouchMediaServer(ctx, store.TouchMediaServerParams{ID: s.ID, Name: name}); err != nil {
		return Caller{}, 0, err
	}
	if e.UserID == "" {
		return Caller{}, 0, nil
	}
	u, err := q.SeeMediaServerUser(ctx, store.SeeMediaServerUserParams{ServerID: s.ID, ExternalID: e.UserID, Name: e.UserName})
	if err != nil {
		return Caller{}, 0, err
	}
	if u.GrantID == nil {
		if u.Ignored {
			return Caller{}, 0, nil
		}
		return Caller{}, u.ID, nil
	}
	g, err := q.ServerUserGrant(ctx, *u.GrantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Caller{}, 0, nil
	}
	if err != nil {
		return Caller{}, 0, err
	}
	if err := q.TouchGrant(ctx, g.ID); err != nil {
		return Caller{}, 0, err
	}
	return Caller{AccountID: s.AccountID, ProfileID: g.ProfileID, GrantID: g.ID}, 0, nil
}
