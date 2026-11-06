package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/dbtest"
)

func TestMediaServers(t *testing.T) {
	mail := &fakeMail{}
	s := &Server{DB: dbtest.New(t), Mail: mail, AppURL: "https://staging.tethr.tv", AllowedOrigins: []string{"https://staging.tethr.tv"}, Cookie: CookieConfig{Name: "tethr_session"}}
	h := s.Handler()
	cookie := signIn(t, h, mail)
	var sam Profile
	_ = json.Unmarshal(do(t, h, "POST", "/v1/profiles", `{"name":"Sam","kind":"person"}`, cookie).Body.Bytes(), &sam)

	if rec := do(t, h, "POST", "/v1/media-servers", `{"kind":"plex"}`, cookie); rec.Code != http.StatusBadRequest {
		t.Errorf("plex for now = %d %s", rec.Code, rec.Body)
	}
	rec := do(t, h, "POST", "/v1/media-servers", `{"kind":"jellyfin"}`, cookie)
	var added AddedMediaServer
	_ = json.Unmarshal(rec.Body.Bytes(), &added)
	if rec.Code != http.StatusCreated || !strings.HasPrefix(added.Path, "/hooks/jellyfin/") || len(added.Server.Users) != 0 {
		t.Fatalf("add = %d %s", rec.Code, rec.Body)
	}
	secret := strings.TrimPrefix(added.Path, "/hooks/jellyfin/")
	if _, _, err := core.ServerCaller(context.Background(), s.DB, core.ServerEvent{Kind: "jellyfin", Secret: secret, ServerName: "Home", UserID: "u1", UserName: "Sam"}); err != nil {
		t.Fatal(err)
	}

	var servers []MediaServer
	_ = json.Unmarshal(do(t, h, "GET", "/v1/media-servers", "", cookie).Body.Bytes(), &servers)
	if len(servers) != 1 || *servers[0].Name != "Home" || len(servers[0].Users) != 1 || servers[0].Users[0].ProfileId != nil {
		t.Fatalf("servers = %+v", servers)
	}
	user := "/v1/media-servers/" + strconv.FormatInt(added.Server.Id, 10) + "/users/" + strconv.FormatInt(servers[0].Users[0].Id, 10)
	if rec := do(t, h, "PUT", user, `{"profile_id":5}`, cookie); rec.Code != http.StatusNotFound {
		t.Errorf("someone else's profile = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "PUT", user, `{"profile_id":`+strconv.FormatInt(sam.Id, 10)+`}`, cookie); rec.Code != http.StatusNoContent {
		t.Fatalf("match = %d %s", rec.Code, rec.Body)
	}
	servers = nil
	_ = json.Unmarshal(do(t, h, "GET", "/v1/media-servers", "", cookie).Body.Bytes(), &servers)
	if p := servers[0].Users[0].ProfileId; p == nil || *p != sam.Id {
		t.Errorf("matched user = %+v", servers[0].Users[0])
	}
	if rec := do(t, h, "GET", "/v1/connections", "", cookie); !strings.Contains(rec.Body.String(), `"app_name":"Jellyfin"`) || !strings.Contains(rec.Body.String(), `"label":"Sam on Home"`) {
		t.Errorf("connections = %s", rec.Body)
	}

	server := "/v1/media-servers/" + strconv.FormatInt(added.Server.Id, 10)
	var address MediaServerAddress
	_ = json.Unmarshal(do(t, h, "POST", server+"/address", "", cookie).Body.Bytes(), &address)
	if address.Path == "" || address.Path == added.Path {
		t.Errorf("new address = %+v", address)
	}
	if rec := do(t, h, "DELETE", server, "", cookie); rec.Code != http.StatusNoContent {
		t.Errorf("remove = %d", rec.Code)
	}
	if rec := do(t, h, "GET", "/v1/media-servers", "", cookie); rec.Body.String() != "[]\n" {
		t.Errorf("after removing = %s", rec.Body)
	}
	if rec := do(t, h, "GET", "/v1/connections", "", cookie); strings.Contains(rec.Body.String(), "Jellyfin") {
		t.Errorf("removing the server should end its connections: %s", rec.Body)
	}
	if rec := do(t, h, "DELETE", server, "", cookie); rec.Code != http.StatusNotFound {
		t.Errorf("remove twice = %d", rec.Code)
	}
}
