package core

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/dbtest"
)

func connectedProfile(t *testing.T, db *pgxpool.Pool) (Caller, App, Tokens) {
	t.Helper()
	ctx := context.Background()
	accountID, _ := signedUpAccount(t, db)
	_, profiles, err := Me(ctx, db, accountID)
	if err != nil {
		t.Fatal(err)
	}
	app, err := CreateApp(ctx, db, "Zuno", nil)
	if err != nil {
		t.Fatal(err)
	}
	device, err := StartDeviceCode(ctx, db, app.ClientID)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApproveDevice(ctx, db, accountID, device.UserCode, profiles[0].ID); err != nil {
		t.Fatal(err)
	}
	tokens, err := PollDeviceCode(ctx, db, app.ClientID, "", device.Code, "Zuno/2.3")
	if err != nil {
		t.Fatal(err)
	}
	caller, err := TokenCaller(ctx, db, tokens.Access)
	if err != nil {
		t.Fatal(err)
	}
	return caller, app, tokens
}

func TestDeviceCode(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	accountID, _ := signedUpAccount(t, db)
	sam, err := AddProfile(ctx, db, accountID, NewProfile{Name: "Sam", Kind: "person"})
	if err != nil {
		t.Fatal(err)
	}
	app, err := CreateApp(ctx, db, "Zuno", nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := StartDeviceCode(ctx, db, "tz_nope"); !errors.Is(err, ErrUnknownApp) {
		t.Fatalf("unknown app: %v", err)
	}
	device, err := StartDeviceCode(ctx, db, app.ClientID)
	if err != nil {
		t.Fatal(err)
	}
	if len(device.UserCode) != 8 || device.Interval != 5 || device.ExpiresIn != 600 {
		t.Errorf("device = %+v", device)
	}

	if _, err := PollDeviceCode(ctx, db, app.ClientID, "", device.Code, "Zuno/2.3"); !errors.Is(err, ErrDevicePending) {
		t.Errorf("first poll: %v", err)
	}
	if _, err := PollDeviceCode(ctx, db, app.ClientID, "", device.Code, "Zuno/2.3"); !errors.Is(err, ErrDeviceSlowDown) {
		t.Errorf("quick second poll: %v", err)
	}
	if _, err := PollDeviceCode(ctx, db, app.ClientID, "", "not-a-code", "Zuno/2.3"); !errors.Is(err, ErrUnknownDevice) {
		t.Errorf("unknown code: %v", err)
	}
	if _, err := PollDeviceCode(ctx, db, app.ClientID, "wrong", device.Code, "Zuno/2.3"); !errors.Is(err, ErrBadSecret) {
		t.Errorf("wrong secret: %v", err)
	}

	pending, err := FindDevice(ctx, db, " "+device.UserCode[:4]+"-"+device.UserCode[4:]+" ")
	if err != nil || pending.AppName != "Zuno" {
		t.Fatalf("find = %+v, %v", pending, err)
	}
	other, _ := signedUpOther(t, db)
	if err := ApproveDevice(ctx, db, other, device.UserCode, sam.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("approving with someone else's profile: %v", err)
	}
	if err := ApproveDevice(ctx, db, accountID, device.UserCode, sam.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := FindDevice(ctx, db, device.UserCode); !errors.Is(err, ErrUnknownDevice) {
		t.Errorf("approved code still pending: %v", err)
	}

	tokens, err := PollDeviceCode(ctx, db, app.ClientID, app.ClientSecret, device.Code, "Zuno/2.3")
	if err != nil {
		t.Fatal(err)
	}
	if tokens.Access == "" || tokens.Refresh == "" || tokens.ExpiresIn != 86400 || tokens.Scope != "public" {
		t.Errorf("tokens = %+v", tokens)
	}
	if _, err := PollDeviceCode(ctx, db, app.ClientID, "", device.Code, "Zuno/2.3"); !errors.Is(err, ErrDeviceUsed) {
		t.Errorf("polling a used code: %v", err)
	}
	caller, err := TokenCaller(ctx, db, tokens.Access)
	if err != nil || caller.ProfileID != sam.ID {
		t.Errorf("caller = %+v, %v", caller, err)
	}

	denied, err := StartDeviceCode(ctx, db, app.ClientID)
	if err != nil {
		t.Fatal(err)
	}
	if err := DenyDevice(ctx, db, denied.UserCode); err != nil {
		t.Fatal(err)
	}
	if _, err := PollDeviceCode(ctx, db, app.ClientID, "", denied.Code, "Zuno/2.3"); !errors.Is(err, ErrDeviceDenied) {
		t.Errorf("denied code: %v", err)
	}
}

func signedUpOther(t *testing.T, db *pgxpool.Pool) (int64, Session) {
	t.Helper()
	ctx := context.Background()
	email, err := StartSignUp(ctx, db, SignUp{Email: "sam@example.com", Name: "Sam", Username: "sam"}, false)
	if err != nil {
		t.Fatal(err)
	}
	session, err := Verify(ctx, db, Verification{Token: email.Token}, "Firefox")
	if err != nil {
		t.Fatal(err)
	}
	accountID, err := SessionAccount(ctx, db, session.Token)
	if err != nil {
		t.Fatal(err)
	}
	return accountID, session
}

func TestRefreshAndRevoke(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	_, app, tokens := connectedProfile(t, db)

	fresh, err := RefreshTokens(ctx, db, app.ClientID, app.ClientSecret, tokens.Refresh)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Access == tokens.Access || fresh.Refresh == tokens.Refresh {
		t.Error("refresh didn't rotate the tokens")
	}
	if _, err := TokenCaller(ctx, db, tokens.Access); !errors.Is(err, ErrInvalidGrant) {
		t.Errorf("old access token still works: %v", err)
	}
	if _, err := RefreshTokens(ctx, db, app.ClientID, "", tokens.Refresh); !errors.Is(err, ErrInvalidGrant) {
		t.Errorf("old refresh token still works: %v", err)
	}
	if err := RevokeToken(ctx, db, app.ClientID, fresh.Access); err != nil {
		t.Fatal(err)
	}
	if _, err := TokenCaller(ctx, db, fresh.Access); !errors.Is(err, ErrInvalidGrant) {
		t.Errorf("revoked token still works: %v", err)
	}
}

func TestAuthorizationCode(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	accountID, _ := signedUpAccount(t, db)
	_, profiles, err := Me(ctx, db, accountID)
	if err != nil {
		t.Fatal(err)
	}
	app, err := CreateApp(ctx, db, "Zuno", []string{"zuno://trakt/callback"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AuthorizingApp(ctx, db, app.ClientID, "https://evil.example"); !errors.Is(err, ErrBadRedirect) {
		t.Errorf("unregistered redirect: %v", err)
	}

	in := Authorization{ClientID: app.ClientID, RedirectURI: "zuno://trakt/callback", ProfileID: profiles[0].ID}
	code, err := Authorize(ctx, db, accountID, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExchangeCode(ctx, db, app.ClientID, "", code, in.RedirectURI, "", "Zuno/2.3"); !errors.Is(err, ErrBadSecret) {
		t.Errorf("no secret and no pkce: %v", err)
	}
	code, err = Authorize(ctx, db, accountID, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExchangeCode(ctx, db, app.ClientID, app.ClientSecret, code, in.RedirectURI, "", "Zuno/2.3"); err != nil {
		t.Fatal(err)
	}
	if _, err := ExchangeCode(ctx, db, app.ClientID, app.ClientSecret, code, in.RedirectURI, "", "Zuno/2.3"); !errors.Is(err, ErrInvalidGrant) {
		t.Errorf("code reused: %v", err)
	}

	verifier := "a-long-enough-verifier-for-the-test-to-be-realistic"
	sum := sha256.Sum256([]byte(verifier))
	in.CodeChallenge = base64.RawURLEncoding.EncodeToString(sum[:])
	in.CodeChallengeMethod = "S256"
	code, err = Authorize(ctx, db, accountID, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExchangeCode(ctx, db, app.ClientID, "", code, in.RedirectURI, "wrong", "Zuno/2.3"); !errors.Is(err, ErrInvalidGrant) {
		t.Errorf("wrong verifier: %v", err)
	}
	code, err = Authorize(ctx, db, accountID, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExchangeCode(ctx, db, app.ClientID, "", code, in.RedirectURI, verifier, "Zuno/2.3"); err != nil {
		t.Errorf("pkce exchange: %v", err)
	}

	grants, err := ListGrants(ctx, db, accountID)
	if err != nil || len(grants) != 2 {
		t.Fatalf("grants = %d, %v", len(grants), err)
	}
	if err := RenameGrant(ctx, db, accountID, grants[0].ID, "Living room"); err != nil {
		t.Fatal(err)
	}
	if err := RevokeGrant(ctx, db, accountID, grants[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := RevokeGrant(ctx, db, accountID, grants[0].ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("revoking twice: %v", err)
	}
}

func TestDisconnectStopsTokens(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	caller, app, tokens := connectedProfile(t, db)
	var accountID int64
	err := db.QueryRow(ctx, "SELECT h.owner_account_id FROM profiles p JOIN households h ON h.id = p.household_id WHERE p.id = $1", caller.ProfileID).Scan(&accountID)
	if err != nil {
		t.Fatal(err)
	}
	if err := RevokeGrant(ctx, db, accountID, caller.GrantID); err != nil {
		t.Fatal(err)
	}
	if _, err := TokenCaller(ctx, db, tokens.Access); !errors.Is(err, ErrInvalidGrant) {
		t.Errorf("access token after disconnect: %v", err)
	}
	if _, err := RefreshTokens(ctx, db, app.ClientID, app.ClientSecret, tokens.Refresh); !errors.Is(err, ErrInvalidGrant) {
		t.Errorf("refresh after disconnect: %v", err)
	}
}
