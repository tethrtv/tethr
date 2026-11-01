package core

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

var (
	ErrChallengeExpired = errors.New("that passkey request has expired, try again")
	ErrPasskeyRejected  = errors.New("that passkey couldn't be checked")
)

const challengeLifetime = 5 * time.Minute

type PasskeyOptions struct {
	Challenge string
	Options   json.RawMessage
}

type passkeyUser struct {
	account     store.Account
	credentials []webauthn.Credential
}

func (u passkeyUser) WebAuthnID() []byte                         { return []byte(strconv.FormatInt(u.account.ID, 10)) }
func (u passkeyUser) WebAuthnName() string                       { return u.account.Email }
func (u passkeyUser) WebAuthnDisplayName() string                { return u.account.Name }
func (u passkeyUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

func loadPasskeyUser(ctx context.Context, q *store.Queries, accountID int64) (passkeyUser, error) {
	account, err := q.AccountByID(ctx, accountID)
	if err != nil {
		return passkeyUser{}, err
	}
	rows, err := q.ListPasskeys(ctx, accountID)
	if err != nil {
		return passkeyUser{}, err
	}
	user := passkeyUser{account: account}
	for _, row := range rows {
		var c webauthn.Credential
		if err := json.Unmarshal(row.Credential, &c); err != nil {
			return passkeyUser{}, err
		}
		user.credentials = append(user.credentials, c)
	}
	return user, nil
}

func BeginAddPasskey(ctx context.Context, db *pgxpool.Pool, wa *webauthn.WebAuthn, accountID int64) (PasskeyOptions, error) {
	q := store.New(db)
	user, err := loadPasskeyUser(ctx, q, accountID)
	if err != nil {
		return PasskeyOptions{}, err
	}
	creation, session, err := wa.BeginRegistration(user,
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
		webauthn.WithExclusions(webauthn.Credentials(user.credentials).CredentialDescriptors()),
	)
	if err != nil {
		return PasskeyOptions{}, err
	}
	return saveChallenge(ctx, q, &accountID, "register", session, creation.Response)
}

func FinishAddPasskey(ctx context.Context, db *pgxpool.Pool, wa *webauthn.WebAuthn, accountID int64, challenge string, response []byte, name string) (store.Passkey, error) {
	row, session, err := takeChallenge(ctx, store.New(db), challenge, "register")
	if err != nil {
		return store.Passkey{}, err
	}
	if row.AccountID == nil || *row.AccountID != accountID {
		return store.Passkey{}, ErrChallengeExpired
	}
	var passkey store.Passkey
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		parsed, err := protocol.ParseCredentialCreationResponseBytes(response)
		if err != nil {
			return ErrPasskeyRejected
		}
		user, err := loadPasskeyUser(ctx, q, accountID)
		if err != nil {
			return err
		}
		credential, err := wa.CreateCredential(user, session, parsed)
		if err != nil {
			return ErrPasskeyRejected
		}
		raw, err := json.Marshal(credential)
		if err != nil {
			return err
		}
		if name = strings.TrimSpace(name); name == "" {
			name = "Passkey"
		}
		passkey, err = q.CreatePasskey(ctx, store.CreatePasskeyParams{AccountID: accountID, CredentialID: credential.ID, Credential: raw, Name: name})
		return err
	})
	return passkey, err
}

func BeginPasskeySignIn(ctx context.Context, db *pgxpool.Pool, wa *webauthn.WebAuthn) (PasskeyOptions, error) {
	assertion, session, err := wa.BeginDiscoverableLogin()
	if err != nil {
		return PasskeyOptions{}, err
	}
	return saveChallenge(ctx, store.New(db), nil, "sign_in", session, assertion.Response)
}

func FinishPasskeySignIn(ctx context.Context, db *pgxpool.Pool, wa *webauthn.WebAuthn, challenge string, response []byte, userAgent string) (Session, error) {
	_, session, err := takeChallenge(ctx, store.New(db), challenge, "sign_in")
	if err != nil {
		return Session{}, err
	}
	var signedIn Session
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		parsed, err := protocol.ParseCredentialRequestResponseBytes(response)
		if err != nil {
			return ErrPasskeyRejected
		}
		var accountID int64
		_, credential, err := wa.ValidatePasskeyLogin(func(_, userHandle []byte) (webauthn.User, error) {
			id, err := strconv.ParseInt(string(userHandle), 10, 64)
			if err != nil {
				return nil, err
			}
			accountID = id
			return loadPasskeyUser(ctx, q, id)
		}, session, parsed)
		if err != nil {
			return ErrPasskeyRejected
		}
		row, err := q.PasskeyByCredentialID(ctx, credential.ID)
		if err != nil {
			return ErrPasskeyRejected
		}
		raw, err := json.Marshal(credential)
		if err != nil {
			return err
		}
		if err := q.UsePasskey(ctx, store.UsePasskeyParams{ID: row.ID, Credential: raw}); err != nil {
			return err
		}
		signedIn, err = createSession(ctx, q, accountID, userAgent)
		return err
	})
	return signedIn, err
}

func ListPasskeys(ctx context.Context, db *pgxpool.Pool, accountID int64) ([]store.Passkey, error) {
	return store.New(db).ListPasskeys(ctx, accountID)
}

func RemovePasskey(ctx context.Context, db *pgxpool.Pool, accountID, passkeyID int64) error {
	n, err := store.New(db).DeletePasskey(ctx, store.DeletePasskeyParams{ID: passkeyID, AccountID: accountID})
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

func saveChallenge(ctx context.Context, q *store.Queries, accountID *int64, kind string, session *webauthn.SessionData, options any) (PasskeyOptions, error) {
	token, err := randomToken()
	if err != nil {
		return PasskeyOptions{}, err
	}
	data, err := json.Marshal(session)
	if err != nil {
		return PasskeyOptions{}, err
	}
	err = q.CreateChallenge(ctx, store.CreateChallengeParams{
		TokenHash:   hash(token),
		AccountID:   accountID,
		Kind:        kind,
		SessionData: data,
		ExpiresAt:   time.Now().Add(challengeLifetime),
	})
	if err != nil {
		return PasskeyOptions{}, err
	}
	raw, err := json.Marshal(options)
	return PasskeyOptions{Challenge: token, Options: raw}, err
}

func takeChallenge(ctx context.Context, q *store.Queries, token, kind string) (store.AuthChallenge, webauthn.SessionData, error) {
	row, err := q.TakeChallenge(ctx, store.TakeChallengeParams{TokenHash: hash(token), Kind: kind})
	if errors.Is(err, pgx.ErrNoRows) {
		return store.AuthChallenge{}, webauthn.SessionData{}, ErrChallengeExpired
	}
	if err != nil {
		return store.AuthChallenge{}, webauthn.SessionData{}, err
	}
	var session webauthn.SessionData
	err = json.Unmarshal(row.SessionData, &session)
	return row, session, err
}
