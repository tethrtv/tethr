package api

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/dbtest"
)

func TestOfficialApp(t *testing.T) {
	mail := &fakeMail{}
	s := &Server{DB: dbtest.New(t), Mail: mail, AppURL: "https://staging.tethr.tv", AllowedOrigins: []string{"https://staging.tethr.tv"}, Cookie: CookieConfig{Name: "tethr_session"}}
	h := s.Handler()
	cookie := signIn(t, h, mail)
	ctx := context.Background()

	code, err := core.StartDeviceCode(ctx, s.DB, "tethr-jellyfin")
	if err != nil {
		t.Fatalf("the jellyfin plugin should sign in without registering: %v", err)
	}
	var device Device
	_ = json.Unmarshal(do(t, h, "GET", "/v1/devices/"+code.UserCode, "", cookie).Body.Bytes(), &device)
	if device.AppName != "Tethr for Jellyfin" || !device.Official {
		t.Errorf("device = %+v", device)
	}

	var me Me
	_ = json.Unmarshal(do(t, h, "GET", "/v1/me", "", cookie).Body.Bytes(), &me)
	if rec := do(t, h, "POST", "/v1/devices/"+code.UserCode+"/approve", `{"profile_id":`+strconv.FormatInt(me.Profiles[0].Id, 10)+`}`, cookie); rec.Code != 204 {
		t.Fatalf("approve = %d %s", rec.Code, rec.Body)
	}
	if _, err := core.PollDeviceCode(ctx, s.DB, "tethr-jellyfin", "", code.Code, "jellyfin-plugin-tethr/0.1.0.0"); err != nil {
		t.Fatalf("token = %v", err)
	}
	var connections []Connection
	_ = json.Unmarshal(do(t, h, "GET", "/v1/connections", "", cookie).Body.Bytes(), &connections)
	if len(connections) != 1 || !connections[0].Official {
		t.Errorf("connections = %+v", connections)
	}
	if body := do(t, h, "GET", "/v1/apps", "", cookie).Body.String(); strings.Contains(body, "tethr-jellyfin") {
		t.Errorf("official apps aren't anyone's to manage: %s", body)
	}
}
