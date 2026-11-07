package core

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

const (
	invitesEach  = 3
	invitesPause = "invites_paused"
)

var (
	ErrNoInvitesLeft = errors.New("no invites left")
	ErrInvitesPaused = errors.New("invites are paused for now")
)

type Invite struct {
	Code      string
	CreatedAt time.Time
	UsedAt    *time.Time
	UsedBy    string
}

type Invites struct {
	Paused bool
	Left   int
	Made   []Invite
}

type InviteCheck struct {
	Valid     bool
	InvitedBy string
}

func AccountInvites(ctx context.Context, db *pgxpool.Pool, accountID int64) (Invites, error) {
	return accountInvites(ctx, store.New(db), accountID)
}

func accountInvites(ctx context.Context, q *store.Queries, accountID int64) (Invites, error) {
	paused, err := q.SwitchOn(ctx, invitesPause)
	if err != nil {
		return Invites{}, err
	}
	extra, err := q.ExtraInvites(ctx, accountID)
	if err != nil {
		return Invites{}, err
	}
	rows, err := q.InvitesMadeBy(ctx, &accountID)
	if err != nil {
		return Invites{}, err
	}
	out := Invites{Paused: paused, Left: max(invitesEach+int(extra)-len(rows), 0), Made: make([]Invite, 0, len(rows))}
	for _, r := range rows {
		i := Invite{Code: r.Code, CreatedAt: r.CreatedAt, UsedAt: r.UsedAt}
		if r.UsedByUsername != nil {
			i.UsedBy = *r.UsedByUsername
		}
		out.Made = append(out.Made, i)
	}
	return out, nil
}

func MakeInvite(ctx context.Context, db *pgxpool.Pool, accountID int64) (Invite, error) {
	var out Invite
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		invites, err := accountInvites(ctx, q, accountID)
		switch {
		case err != nil:
			return err
		case invites.Paused:
			return ErrInvitesPaused
		case invites.Left == 0:
			return ErrNoInvitesLeft
		}
		code, err := inviteCode()
		if err != nil {
			return err
		}
		row, err := q.CreateUserInvite(ctx, store.CreateUserInviteParams{Code: code, CreatedByAccountID: &accountID})
		out = Invite{Code: row.Code, CreatedAt: row.CreatedAt}
		return err
	})
	return out, err
}

func CheckInvite(ctx context.Context, db *pgxpool.Pool, code string) (InviteCheck, error) {
	row, err := store.New(db).CheckInvite(ctx, strings.ToUpper(strings.TrimSpace(code)))
	if errors.Is(err, pgx.ErrNoRows) {
		return InviteCheck{}, nil
	}
	if err != nil {
		return InviteCheck{}, err
	}
	out := InviteCheck{Valid: row.UsedAt == nil}
	if out.Valid && row.InvitedBy != nil {
		out.InvitedBy = *row.InvitedBy
	}
	return out, nil
}

func GrantInvites(ctx context.Context, db *pgxpool.Pool, username string, n int) (int, error) {
	var left int
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		accountID, err := q.AccountOfUsername(ctx, username)
		if err != nil {
			return notFound(err)
		}
		if _, err := q.GrantInvites(ctx, store.GrantInvitesParams{ID: accountID, N: int32(n)}); err != nil {
			return err
		}
		invites, err := accountInvites(ctx, q, accountID)
		left = invites.Left
		return err
	})
	return left, err
}

func PauseInvites(ctx context.Context, db *pgxpool.Pool, paused bool) error {
	q := store.New(db)
	if paused {
		return q.TurnOn(ctx, invitesPause)
	}
	return q.TurnOff(ctx, invitesPause)
}
