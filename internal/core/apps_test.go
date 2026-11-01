package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/tethrtv/tethr/internal/dbtest"
)

func TestRedirects(t *testing.T) {
	tests := []struct {
		uri  string
		want bool
	}{
		{"https://player.example/callback", true},
		{"zuno://trakt/callback", true},
		{"http://localhost:8080/callback", true},
		{"http://127.0.0.1/cb", true},
		{"http://player.example/callback", false},
		{"https://player.example/cb#frag", false},
		{"javascript:alert(1)", false},
		{"/relative", false},
		{"https://", false},
	}
	for _, tt := range tests {
		if got := validRedirect(tt.uri); got != tt.want {
			t.Errorf("validRedirect(%q) = %v, want %v", tt.uri, got, tt.want)
		}
	}
}

func TestRegisterApps(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	accountID, _ := signedUpAccount(t, db)
	other, _ := signedUpOther(t, db)

	app, row, err := RegisterApp(ctx, db, accountID, "  My Player ", []string{"myplayer://tethr/callback", ""})
	if err != nil {
		t.Fatal(err)
	}
	if row.Name != "My Player" || len(row.RedirectUris) != 1 || !strings.HasPrefix(app.ClientID, "tz_") {
		t.Errorf("registered = %+v", row)
	}
	if _, _, err := RegisterApp(ctx, db, accountID, "", nil); !errors.Is(err, ErrAppName) {
		t.Errorf("no name: %v", err)
	}
	if _, _, err := RegisterApp(ctx, db, accountID, "Bad", []string{"http://evil.example"}); !errors.Is(err, ErrRedirectURI) {
		t.Errorf("plain http: %v", err)
	}

	device, err := StartDeviceCode(ctx, db, app.ClientID)
	if err != nil {
		t.Fatal(err)
	}
	_, profiles, _ := Me(ctx, db, accountID)
	if err := ApproveDevice(ctx, db, accountID, device.UserCode, profiles[0].ID); err != nil {
		t.Fatal(err)
	}
	tokens, err := PollDeviceCode(ctx, db, app.ClientID, app.ClientSecret, device.Code, "")
	if err != nil {
		t.Fatal(err)
	}
	apps, err := ListApps(ctx, db, accountID)
	if err != nil || len(apps) != 1 || apps[0].Connections != 1 {
		t.Fatalf("apps = %+v, %v", apps, err)
	}

	tvOnly := []string{}
	if _, err := UpdateApp(ctx, db, other, row.ID, AppChanges{RedirectURIs: &tvOnly}); !errors.Is(err, ErrNotFound) {
		t.Errorf("someone else's app: %v", err)
	}
	updated, err := UpdateApp(ctx, db, accountID, row.ID, AppChanges{RedirectURIs: &tvOnly})
	if err != nil || len(updated.RedirectUris) != 0 || updated.Name != "My Player" {
		t.Errorf("update = %+v, %v", updated, err)
	}

	secret, err := NewAppSecret(ctx, db, accountID, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RefreshTokens(ctx, db, app.ClientID, app.ClientSecret, tokens.Refresh); !errors.Is(err, ErrBadSecret) {
		t.Errorf("old secret: %v", err)
	}
	fresh, err := RefreshTokens(ctx, db, app.ClientID, secret, tokens.Refresh)
	if err != nil {
		t.Fatal(err)
	}

	if err := RevokeApp(ctx, db, accountID, row.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := TokenCaller(ctx, db, fresh.Access); !errors.Is(err, ErrInvalidGrant) {
		t.Errorf("token after the app was revoked: %v", err)
	}
	if _, err := StartDeviceCode(ctx, db, app.ClientID); !errors.Is(err, ErrUnknownApp) {
		t.Errorf("device code for a revoked app: %v", err)
	}

	for i := range 10 {
		if _, _, err := RegisterApp(ctx, db, accountID, fmt.Sprintf("App %d", i), nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := RegisterApp(ctx, db, accountID, "One more", nil); !errors.Is(err, ErrTooManyApps) {
		t.Errorf("eleventh app: %v", err)
	}
}
