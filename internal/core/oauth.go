package core

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

var (
	ErrUnknownApp       = errors.New("unknown client id")
	ErrBadSecret        = errors.New("client secret doesn't match")
	ErrInvalidGrant     = errors.New("that code or token isn't valid")
	ErrDevicePending    = errors.New("waiting for the code to be approved")
	ErrDeviceSlowDown   = errors.New("polling too quickly")
	ErrDeviceDenied     = errors.New("the code was denied")
	ErrDeviceUsed       = errors.New("the code has already been used")
	ErrDeviceExpired    = errors.New("the code has expired")
	ErrUnknownDevice    = errors.New("no code like that is waiting")
	ErrBadRedirect      = errors.New("that redirect uri isn't registered for the app")
	ErrBadCodeChallenge = errors.New("code challenge method must be S256")
)

const (
	deviceCodeLifetime = 10 * time.Minute
	devicePollInterval = 5
	accessLifetime     = 24 * time.Hour
	refreshLifetime    = 90 * 24 * time.Hour
	oauthCodeLifetime  = 10 * time.Minute
	userCodeAlphabet   = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
)

type App struct {
	ClientID     string
	ClientSecret string
}

type DeviceCode struct {
	Code      string
	UserCode  string
	ExpiresIn int
	Interval  int
}

type Tokens struct {
	Access    string
	Refresh   string
	ExpiresIn int
	Scope     string
	CreatedAt time.Time
}

type Caller struct {
	AccountID int64
	ProfileID int64
	GrantID   int64
	AppID     int64
}

func app(ctx context.Context, q *store.Queries, clientID, secret string) (store.App, error) {
	a, err := q.AppByClientID(ctx, clientID)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.App{}, ErrUnknownApp
	}
	if err != nil {
		return store.App{}, err
	}
	if secret != "" && subtle.ConstantTimeCompare(hash(secret), a.ClientSecretHash) != 1 {
		return store.App{}, ErrBadSecret
	}
	return a, nil
}

func KnownApp(ctx context.Context, db *pgxpool.Pool, clientID string) error {
	_, err := app(ctx, store.New(db), clientID, "")
	return err
}

func StartDeviceCode(ctx context.Context, db *pgxpool.Pool, clientID string) (DeviceCode, error) {
	q := store.New(db)
	a, err := app(ctx, q, clientID, "")
	if err != nil {
		return DeviceCode{}, err
	}
	code, err := randomToken()
	if err != nil {
		return DeviceCode{}, err
	}
	userCode, err := newUserCode()
	if err != nil {
		return DeviceCode{}, err
	}
	_, err = q.CreateDeviceCode(ctx, store.CreateDeviceCodeParams{
		AppID:           a.ID,
		DeviceCodeHash:  hash(code),
		UserCode:        userCode,
		ExpiresAt:       time.Now().Add(deviceCodeLifetime),
		IntervalSeconds: devicePollInterval,
	})
	return DeviceCode{
		Code:      code,
		UserCode:  userCode,
		ExpiresIn: int(deviceCodeLifetime.Seconds()),
		Interval:  devicePollInterval,
	}, err
}

func newUserCode() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = userCodeAlphabet[int(b[i])%len(userCodeAlphabet)]
	}
	return string(b), nil
}

func NormaliseUserCode(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if strings.ContainsRune(userCodeAlphabet, r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func PollDeviceCode(ctx context.Context, db *pgxpool.Pool, clientID, secret, code, userAgent string) (Tokens, error) {
	var tokens Tokens
	var waiting error
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		a, err := app(ctx, q, clientID, secret)
		if err != nil {
			return err
		}
		d, err := q.DeviceCodeByHash(ctx, hash(code))
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && d.AppID != a.ID) {
			return ErrUnknownDevice
		}
		if err != nil {
			return err
		}
		switch {
		case d.Status == "used":
			return ErrDeviceUsed
		case d.Status == "denied":
			return ErrDeviceDenied
		case time.Now().After(d.ExpiresAt):
			return ErrDeviceExpired
		case d.Status == "pending":
			waiting = ErrDevicePending
			if d.LastPolledAt != nil && time.Since(*d.LastPolledAt) < time.Duration(d.IntervalSeconds)*time.Second {
				waiting = ErrDeviceSlowDown
			}
			return q.PollDeviceCode(ctx, d.ID)
		}
		if err := q.SetDeviceCodeStatus(ctx, store.SetDeviceCodeStatusParams{ID: d.ID, Status: "used", ProfileID: d.ProfileID}); err != nil {
			return err
		}
		tokens, err = connect(ctx, q, a.ID, *d.ProfileID, "device", userAgent)
		return err
	})
	if err == nil && waiting != nil {
		return Tokens{}, waiting
	}
	return tokens, err
}

type PendingDevice struct {
	UserCode string
	AppName  string
}

func FindDevice(ctx context.Context, db *pgxpool.Pool, userCode string) (PendingDevice, error) {
	d, err := store.New(db).PendingDeviceCode(ctx, NormaliseUserCode(userCode))
	if errors.Is(err, pgx.ErrNoRows) {
		return PendingDevice{}, ErrUnknownDevice
	}
	if err != nil {
		return PendingDevice{}, err
	}
	return PendingDevice{UserCode: d.UserCode, AppName: d.AppName}, nil
}

func ApproveDevice(ctx context.Context, db *pgxpool.Pool, accountID int64, userCode string, profileID int64) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		d, err := q.PendingDeviceCode(ctx, NormaliseUserCode(userCode))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUnknownDevice
		}
		if err != nil {
			return err
		}
		if _, err := q.HouseholdProfile(ctx, store.HouseholdProfileParams{ID: profileID, OwnerAccountID: accountID}); err != nil {
			return notFound(err)
		}
		return q.SetDeviceCodeStatus(ctx, store.SetDeviceCodeStatusParams{ID: d.ID, Status: "approved", ProfileID: &profileID})
	})
}

func DenyDevice(ctx context.Context, db *pgxpool.Pool, userCode string) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		d, err := q.PendingDeviceCode(ctx, NormaliseUserCode(userCode))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUnknownDevice
		}
		if err != nil {
			return err
		}
		return q.SetDeviceCodeStatus(ctx, store.SetDeviceCodeStatusParams{ID: d.ID, Status: "denied"})
	})
}

type Authorization struct {
	ClientID            string
	RedirectURI         string
	ProfileID           int64
	CodeChallenge       string
	CodeChallengeMethod string
}

func AuthorizingApp(ctx context.Context, db *pgxpool.Pool, clientID, redirectURI string) (string, error) {
	a, err := app(ctx, store.New(db), clientID, "")
	if err != nil {
		return "", err
	}
	if !registeredRedirect(a, redirectURI) {
		return "", ErrBadRedirect
	}
	return a.Name, nil
}

func registeredRedirect(a store.App, uri string) bool {
	for _, r := range a.RedirectUris {
		if r == uri {
			return true
		}
	}
	return false
}

func Authorize(ctx context.Context, db *pgxpool.Pool, accountID int64, in Authorization) (string, error) {
	if in.CodeChallenge != "" && in.CodeChallengeMethod != "S256" {
		return "", ErrBadCodeChallenge
	}
	code, err := randomToken()
	if err != nil {
		return "", err
	}
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		a, err := app(ctx, q, in.ClientID, "")
		if err != nil {
			return err
		}
		if !registeredRedirect(a, in.RedirectURI) {
			return ErrBadRedirect
		}
		if _, err := q.HouseholdProfile(ctx, store.HouseholdProfileParams{ID: in.ProfileID, OwnerAccountID: accountID}); err != nil {
			return notFound(err)
		}
		return q.CreateOAuthCode(ctx, store.CreateOAuthCodeParams{
			AppID:               a.ID,
			ProfileID:           in.ProfileID,
			CodeHash:            hash(code),
			RedirectUri:         in.RedirectURI,
			CodeChallenge:       optional(in.CodeChallenge),
			CodeChallengeMethod: optional(in.CodeChallengeMethod),
			ExpiresAt:           time.Now().Add(oauthCodeLifetime),
		})
	})
	return code, err
}

func ExchangeCode(ctx context.Context, db *pgxpool.Pool, clientID, secret, code, redirectURI, verifier, userAgent string) (Tokens, error) {
	var tokens Tokens
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		a, err := app(ctx, q, clientID, secret)
		if err != nil {
			return err
		}
		c, err := q.TakeOAuthCode(ctx, store.TakeOAuthCodeParams{CodeHash: hash(code), AppID: a.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidGrant
		}
		if err != nil {
			return err
		}
		if c.RedirectUri != redirectURI {
			return ErrInvalidGrant
		}
		if c.CodeChallenge != nil {
			sum := sha256.Sum256([]byte(verifier))
			if base64.RawURLEncoding.EncodeToString(sum[:]) != *c.CodeChallenge {
				return ErrInvalidGrant
			}
		} else if secret == "" {
			return ErrBadSecret
		}
		tokens, err = connect(ctx, q, a.ID, c.ProfileID, "redirect", userAgent)
		return err
	})
	return tokens, err
}

func RefreshTokens(ctx context.Context, db *pgxpool.Pool, clientID, secret, refresh string) (Tokens, error) {
	var tokens Tokens
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		a, err := app(ctx, q, clientID, secret)
		if err != nil {
			return err
		}
		old, err := q.TokenByRefresh(ctx, store.TokenByRefreshParams{RefreshTokenHash: hash(refresh), AppID: a.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidGrant
		}
		if err != nil {
			return err
		}
		if err := q.RevokeToken(ctx, old.ID); err != nil {
			return err
		}
		tokens, err = issueTokens(ctx, q, old.GrantID)
		return err
	})
	return tokens, err
}

func RevokeToken(ctx context.Context, db *pgxpool.Pool, clientID, token string) error {
	q := store.New(db)
	a, err := app(ctx, q, clientID, "")
	if err != nil {
		return err
	}
	_, err = q.RevokeTokenByHash(ctx, store.RevokeTokenByHashParams{AccessTokenHash: hash(token), AppID: a.ID})
	return err
}

func connect(ctx context.Context, q *store.Queries, appID, profileID int64, via, userAgent string) (Tokens, error) {
	grant, err := q.CreateGrant(ctx, store.CreateGrantParams{
		AppID:       appID,
		ProfileID:   profileID,
		UserAgent:   optional(userAgent),
		ApprovedVia: via,
	})
	if err != nil {
		return Tokens{}, err
	}
	return issueTokens(ctx, q, grant.ID)
}

func issueTokens(ctx context.Context, q *store.Queries, grantID int64) (Tokens, error) {
	access, err := randomToken()
	if err != nil {
		return Tokens{}, err
	}
	refresh, err := randomToken()
	if err != nil {
		return Tokens{}, err
	}
	t, err := q.CreateToken(ctx, store.CreateTokenParams{
		GrantID:          grantID,
		AccessTokenHash:  hash(access),
		RefreshTokenHash: hash(refresh),
		AccessExpiresAt:  time.Now().Add(accessLifetime),
		RefreshExpiresAt: time.Now().Add(refreshLifetime),
	})
	if err != nil {
		return Tokens{}, err
	}
	return Tokens{
		Access:    access,
		Refresh:   refresh,
		ExpiresIn: int(accessLifetime.Seconds()),
		Scope:     "public",
		CreatedAt: t.CreatedAt,
	}, nil
}

func TokenCaller(ctx context.Context, db *pgxpool.Pool, access string) (Caller, error) {
	q := store.New(db)
	row, err := q.TokenByAccess(ctx, hash(access))
	if errors.Is(err, pgx.ErrNoRows) {
		return Caller{}, ErrInvalidGrant
	}
	if err != nil {
		return Caller{}, err
	}
	if row.LastUsedAt == nil || time.Since(*row.LastUsedAt) > time.Minute {
		if err := q.TouchGrant(ctx, row.GrantID); err != nil {
			return Caller{}, err
		}
	}
	return Caller{AccountID: row.AccountID, ProfileID: row.ProfileID, GrantID: row.GrantID, AppID: row.AppID}, nil
}

func ListGrants(ctx context.Context, db *pgxpool.Pool, accountID int64) ([]store.HouseholdGrantsRow, error) {
	return store.New(db).HouseholdGrants(ctx, accountID)
}

func RevokeGrant(ctx context.Context, db *pgxpool.Pool, accountID, grantID int64) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		n, err := q.RevokeGrant(ctx, store.RevokeGrantParams{ID: grantID, OwnerAccountID: accountID})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		return q.RevokeGrantTokens(ctx, grantID)
	})
}

func RenameGrant(ctx context.Context, db *pgxpool.Pool, accountID, grantID int64, label string) error {
	n, err := store.New(db).RenameGrant(ctx, store.RenameGrantParams{ID: grantID, OwnerAccountID: accountID, Label: optional(strings.TrimSpace(label))})
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
