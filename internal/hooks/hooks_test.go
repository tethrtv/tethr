package hooks

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/dbtest"
	"github.com/tethrtv/tethr/internal/tmdb"
)

type household struct {
	db      *pgxpool.Pool
	mux     *http.ServeMux
	account int64
	profile int64
}

func newHousehold(t *testing.T) household {
	t.Helper()
	db := dbtest.New(t)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/3/find/349232" && r.URL.Query().Get("external_source") == "tvdb_id" {
			_, _ = w.Write([]byte(`{"tv_episode_results":[{"id":62085,"show_id":1396,"season_number":1,"episode_number":1}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"tv_episode_results":[]}`))
	}))
	t.Cleanup(fake.Close)
	mux := http.NewServeMux()
	(&Server{DB: db, TMDB: &tmdb.Client{Token: "secret", BaseURL: fake.URL}, Log: slog.New(slog.DiscardHandler)}).Routes(mux)
	account, profile, err := core.CreateAccount(context.Background(), db, core.SignUp{Email: "nathan@example.com", Name: "Nathan", Username: "nathan"})
	if err != nil {
		t.Fatal(err)
	}
	return household{db: db, mux: mux, account: account.ID, profile: profile.ID}
}

func (h household) post(path, contentType, body string) int {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	return rec.Code
}

func (h household) history(t *testing.T) []core.Entry {
	t.Helper()
	entries, _, err := core.History(context.Background(), h.db, h.profile, core.HistoryFilter{}, core.Page{Page: 1, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

func (h household) matchEveryone(t *testing.T) {
	t.Helper()
	servers, err := core.MediaServers(context.Background(), h.db, h.account)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range servers {
		for _, u := range s.Users {
			if err := core.MatchServerUser(context.Background(), h.db, h.account, s.ID, u.ID, &h.profile); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestJellyfin(t *testing.T) {
	h := newHousehold(t)
	db, ctx := h.db, context.Background()
	server, secret, err := core.AddMediaServer(ctx, db, h.account, "jellyfin")
	if err != nil {
		t.Fatal(err)
	}
	send := func(body string) int { return h.post("/hooks/jellyfin/"+secret, "text/plain", body) }
	history := func() []core.Entry { return h.history(t) }
	const episode = `{"NotificationType":"PlaybackStop","ServerName":"Home","UserId":"u1","NotificationUsername":"Sam","ItemType":"Episode",
		"Provider_tmdb":"62085","Provider_tvdb":"349232","RunTimeTicks":34800000000,"PlaybackPositionTicks":34000000000,"PlayedToCompletion":true}`

	if code := send(episode); code != http.StatusNoContent || len(history()) != 0 {
		t.Fatalf("unmatched user = %d, history %v", code, history())
	}
	servers, err := core.MediaServers(ctx, db, h.account)
	if err != nil || len(servers) != 1 || *servers[0].Name != "Home" || len(servers[0].Users) != 1 || servers[0].Users[0].ProfileID != nil {
		t.Fatalf("servers = %+v, %v", servers, err)
	}
	user := servers[0].Users[0].ID
	if err := core.MatchServerUser(ctx, db, h.account, server.ID, user, &h.profile); err != nil {
		t.Fatal(err)
	}

	if code := send(episode); code != http.StatusNoContent {
		t.Fatalf("episode = %d", code)
	}
	if code := send(episode); code != http.StatusNoContent {
		t.Fatalf("the same episode again = %d", code)
	}
	got := history()
	if len(got) != 1 || got[0].Item.Episode == nil || *got[0].Item.Title.TmdbID != 1396 || got[0].Item.Episode.EpisodeNumber != 1 {
		t.Fatalf("history after the episode = %+v", got)
	}

	movie := func(event, reason string) string {
		return fmt.Sprintf(`{"NotificationType":%q,"UserId":"u1","NotificationUsername":"Sam","ItemType":"Movie","Provider_tmdb":"27205",
			"RunTimeTicks":88800000000,"PlaybackPositionTicks":26640000000,"PlayedToCompletion":false,"SaveReason":%q,"Played":true}`, event, reason)
	}
	if code := send(movie("PlaybackStop", "")); code != http.StatusNoContent {
		t.Fatalf("paused film = %d", code)
	}
	paused, err := core.Playback(ctx, db, h.profile, "movie", core.Range{}, core.Page{Page: 1, Limit: 10})
	if err != nil || len(paused) != 1 || paused[0].Progress != 30 {
		t.Errorf("playback = %+v, %v", paused, err)
	}
	send(movie("UserDataSaved", "PlaybackFinished"))
	if len(history()) != 1 {
		t.Errorf("only marking played by hand should log a watch: %+v", history())
	}
	send(movie("UserDataSaved", "TogglePlayed"))
	if got := history(); len(got) != 2 || got[0].Item.Kind() != "movie" || got[0].Action != "watch" {
		t.Errorf("marked played = %+v", got)
	}

	if code := send(`{"NotificationType":"PlaybackStop","UserId":"u1","ItemType":"Episode","Provider_tvdb":"1","PlayedToCompletion":true}`); code != http.StatusNoContent {
		t.Errorf("an episode tmdb doesn't know = %d", code)
	}
	if code := send("not json"); code != http.StatusBadRequest {
		t.Errorf("bad body = %d", code)
	}

	if err := core.MatchServerUser(ctx, db, h.account, server.ID, user, nil); err != nil {
		t.Fatal(err)
	}
	send(strings.Replace(episode, "PlaybackStop", "PlaybackStart", 1))
	if err := core.RemoveMediaServer(ctx, db, h.account, server.ID); err != nil {
		t.Fatal(err)
	}
	if code := send(episode); code != http.StatusNotFound {
		t.Errorf("removed server = %d", code)
	}
	if err := core.KnownApp(ctx, db, "jellyfin"); !errors.Is(err, core.ErrUnknownApp) {
		t.Errorf("the built-in app shouldn't sign in like an app: %v", err)
	}
}
