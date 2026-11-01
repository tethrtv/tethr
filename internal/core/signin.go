package core

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

var (
	ErrInvalidLink     = errors.New("that link or code has expired or already been used")
	ErrTooManyAttempts = errors.New("too many wrong codes, ask for a new email")
	ErrTooManyEmails   = errors.New("too many emails, try again in a few minutes")
	ErrNoSession       = errors.New("not signed in")
)

const (
	linkLifetime    = 15 * time.Minute
	emailWindow     = 15 * time.Minute
	emailsPerWindow = 3
	codeAttempts    = 5
	sessionLifetime = 30 * 24 * time.Hour
)

type SignInEmail struct {
	To      string
	Purpose string
	Token   string
	Code    string
}

type Verification struct {
	Token string
	Email string
	Code  string
}

type Session struct {
	Token     string
	ExpiresAt time.Time
}

func StartSignUp(ctx context.Context, db *pgxpool.Pool, in SignUp, requireInvite bool) (*SignInEmail, error) {
	in, err := in.clean()
	if err != nil {
		return nil, err
	}
	if requireInvite && strings.TrimSpace(in.Invite) == "" {
		return nil, ErrInviteRequired
	}
	var email *SignInEmail
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		var invite *store.Invite
		if requireInvite {
			found, err := q.InviteByCode(ctx, strings.ToUpper(strings.TrimSpace(in.Invite)))
			if errors.Is(err, pgx.ErrNoRows) || (err == nil && found.UsedAt != nil) {
				return ErrInvalidInvite
			}
			if err != nil {
				return err
			}
			invite = &found
		}
		if err := underEmailLimit(ctx, q, in.Email); err != nil {
			return err
		}
		account, err := q.AccountByEmail(ctx, in.Email)
		if err == nil {
			email, err = createLink(ctx, q, account, "sign_in")
			return err
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		account, _, err = createAccount(ctx, q, in)
		if err != nil {
			return err
		}
		if invite != nil {
			if err := q.UseInvite(ctx, store.UseInviteParams{ID: invite.ID, UsedByAccountID: &account.ID}); err != nil {
				return err
			}
		}
		email, err = createLink(ctx, q, account, "sign_up")
		return err
	})
	if err != nil {
		return nil, taken(err)
	}
	return email, nil
}

func StartSignIn(ctx context.Context, db *pgxpool.Pool, address string) (*SignInEmail, error) {
	address = strings.TrimSpace(address)
	if !validEmail(address) {
		return nil, ErrInvalidEmail
	}
	var email *SignInEmail
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := underEmailLimit(ctx, q, address); err != nil {
			return err
		}
		account, err := q.AccountByEmail(ctx, address)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		email, err = createLink(ctx, q, account, "sign_in")
		return err
	})
	return email, err
}

func underEmailLimit(ctx context.Context, q *store.Queries, address string) error {
	sent, err := q.RecentEmailLinks(ctx, store.RecentEmailLinksParams{Email: address, CreatedAt: time.Now().Add(-emailWindow)})
	if err != nil {
		return err
	}
	if sent >= emailsPerWindow {
		return ErrTooManyEmails
	}
	return nil
}

func createLink(ctx context.Context, q *store.Queries, account store.Account, purpose string) (*SignInEmail, error) {
	token, err := randomToken()
	if err != nil {
		return nil, err
	}
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return nil, err
	}
	code := fmt.Sprintf("%06d", n.Int64())
	_, err = q.CreateEmailLink(ctx, store.CreateEmailLinkParams{
		Email:     account.Email,
		AccountID: account.ID,
		Purpose:   purpose,
		TokenHash: hash(token),
		CodeHash:  hash(code),
		ExpiresAt: time.Now().Add(linkLifetime),
	})
	if err != nil {
		return nil, err
	}
	return &SignInEmail{To: account.Email, Purpose: purpose, Token: token, Code: code}, nil
}

func Verify(ctx context.Context, db *pgxpool.Pool, v Verification, userAgent string) (Session, error) {
	var session Session
	var wrongCode bool
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		link, err := findLink(ctx, q, v)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidLink
		}
		if err != nil {
			return err
		}
		if link.UsedAt != nil || time.Now().After(link.ExpiresAt) {
			return ErrInvalidLink
		}
		if v.Token == "" {
			if link.Attempts >= codeAttempts {
				return ErrTooManyAttempts
			}
			if subtle.ConstantTimeCompare(link.CodeHash, hash(strings.TrimSpace(v.Code))) != 1 {
				wrongCode = true
				return q.CountEmailLinkAttempt(ctx, link.ID)
			}
		}
		if err := q.UseEmailLink(ctx, link.ID); err != nil {
			return err
		}
		if err := q.VerifyAccountEmail(ctx, link.AccountID); err != nil {
			return err
		}
		session, err = createSession(ctx, q, link.AccountID, userAgent)
		return err
	})
	if err != nil {
		return Session{}, err
	}
	if wrongCode {
		return Session{}, ErrInvalidLink
	}
	return session, nil
}

func findLink(ctx context.Context, q *store.Queries, v Verification) (store.EmailLink, error) {
	if v.Token != "" {
		return q.EmailLinkByToken(ctx, hash(v.Token))
	}
	return q.LatestEmailLink(ctx, strings.TrimSpace(v.Email))
}

func createSession(ctx context.Context, q *store.Queries, accountID int64, userAgent string) (Session, error) {
	token, err := randomToken()
	if err != nil {
		return Session{}, err
	}
	var profileID *int64
	owner, err := q.OwnerProfile(ctx, accountID)
	if err == nil {
		profileID = &owner.ID
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Session{}, err
	}
	expires := time.Now().Add(sessionLifetime)
	_, err = q.CreateSession(ctx, store.CreateSessionParams{
		AccountID: accountID,
		ProfileID: profileID,
		TokenHash: hash(token),
		UserAgent: &userAgent,
		ExpiresAt: expires,
	})
	return Session{Token: token, ExpiresAt: expires}, err
}

func SessionAccount(ctx context.Context, db *pgxpool.Pool, token string) (int64, error) {
	session, err := currentSession(ctx, db, token)
	return session.AccountID, err
}

func currentSession(ctx context.Context, db *pgxpool.Pool, token string) (store.Session, error) {
	q := store.New(db)
	session, err := q.SessionByToken(ctx, hash(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Session{}, ErrNoSession
	}
	if err != nil {
		return store.Session{}, err
	}
	return session, q.TouchSession(ctx, session.ID)
}

func ListSessions(ctx context.Context, db *pgxpool.Pool, accountID int64, token string) ([]store.Session, int64, error) {
	here, err := currentSession(ctx, db, token)
	if err != nil {
		return nil, 0, err
	}
	sessions, err := store.New(db).ListSessions(ctx, accountID)
	return sessions, here.ID, err
}

func EndSession(ctx context.Context, db *pgxpool.Pool, accountID, sessionID int64) error {
	n, err := store.New(db).RevokeSessionByID(ctx, store.RevokeSessionByIDParams{ID: sessionID, AccountID: accountID})
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

func SignOutOthers(ctx context.Context, db *pgxpool.Pool, accountID int64, token string) error {
	here, err := currentSession(ctx, db, token)
	if err != nil {
		return err
	}
	return store.New(db).RevokeOtherSessions(ctx, store.RevokeOtherSessionsParams{AccountID: accountID, ID: here.ID})
}

func SignOut(ctx context.Context, db *pgxpool.Pool, token string) error {
	return store.New(db).RevokeSession(ctx, hash(token))
}

func Me(ctx context.Context, db *pgxpool.Pool, accountID int64) (store.Account, []store.Profile, error) {
	q := store.New(db)
	account, err := q.AccountByID(ctx, accountID)
	if err != nil {
		return store.Account{}, nil, err
	}
	profiles, err := q.HouseholdProfiles(ctx, accountID)
	return account, profiles, err
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hash(s string) []byte {
	h := sha256.Sum256([]byte(s))
	return h[:]
}

func CreateInvite(ctx context.Context, db *pgxpool.Pool, note string) (string, error) {
	b := make([]byte, 5)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	raw := base32.StdEncoding.EncodeToString(b)
	code := raw[:4] + "-" + raw[4:]
	var notePtr *string
	if note != "" {
		notePtr = &note
	}
	_, err := store.New(db).CreateInvite(ctx, store.CreateInviteParams{Code: code, Note: notePtr})
	return code, err
}
