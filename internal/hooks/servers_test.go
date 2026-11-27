package hooks

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"strconv"
	"testing"

	"github.com/tethrtv/tethr/internal/core"
)

func form(field, value string) (string, string) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField(field, value)
	thumb, _ := w.CreateFormFile("thumb", "thumb.jpg")
	_, _ = thumb.Write([]byte("jpeg"))
	_ = w.Close()
	return w.FormDataContentType(), buf.String()
}

func playback(t *testing.T, h household, kind string) []core.Entry {
	t.Helper()
	entries, err := core.Playback(context.Background(), h.db, h.profile, kind, core.Range{}, core.Page{Page: 1, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

func TestPlex(t *testing.T) {
	h := newHousehold(t)
	_, secret, err := core.AddMediaServer(context.Background(), h.db, h.account, "plex")
	if err != nil {
		t.Fatal(err)
	}
	send := func(payload string) int {
		contentType, body := form("payload", payload)
		return h.post("/hooks/plex/"+secret, contentType, body)
	}
	episode := func(event string, offset int) string {
		return `{"event":"` + event + `","user":true,"owner":true,"Account":{"id":1,"title":"nathan"},"Server":{"title":"Plex Box","uuid":"x"},
			"Metadata":{"type":"episode","guid":"plex://episode/5d9c","viewOffset":` + strconv.Itoa(offset) + `,"duration":3480000,
			"Guid":[{"id":"imdb://tt0959621"},{"id":"tmdb://62085"},{"id":"tvdb://349232"}]}}`
	}

	if code := send(episode("media.play", 0)); code != http.StatusNoContent || len(h.history(t)) != 0 {
		t.Fatalf("unmatched = %d", code)
	}
	h.matchEveryone(t)
	if code := send(episode("media.pause", 1740000)); code != http.StatusNoContent {
		t.Fatalf("pause = %d", code)
	}
	if p := playback(t, h, "episode"); len(p) != 1 || p[0].Progress != 50 {
		t.Errorf("paused halfway = %+v", p)
	}
	send(episode("media.scrobble", 3132000))
	send(episode("media.stop", 3300000))
	if got := h.history(t); len(got) != 1 || *got[0].Item.Title.TmdbID != 1396 || got[0].Item.Episode.EpisodeNumber != 1 {
		t.Errorf("scrobbled = %+v", got)
	}
	if p := playback(t, h, "episode"); len(p) != 0 {
		t.Errorf("stopping after the scrobble shouldn't leave a resume point: %+v", p)
	}
	send(episode("media.stop", 0))
	if p := playback(t, h, "episode"); len(p) != 0 {
		t.Errorf("plex resets the offset once something is watched, so a stop at 0 shouldn't leave a resume point: %+v", p)
	}
	send(episode("media.play", 0))
	send(episode("media.stop", 0))
	var watching int
	if err := h.db.QueryRow(context.Background(), "SELECT count(*) FROM watching WHERE profile_id = $1", h.profile).Scan(&watching); err != nil || watching != 0 {
		t.Errorf("stopping in the first moments should still end watching now: %d, %v", watching, err)
	}
	if p := playback(t, h, "episode"); len(p) != 0 {
		t.Errorf("or leave a resume point: %+v", p)
	}

	send(`{"event":"media.scrobble","Account":{"id":2,"title":"guest"},"Server":{"title":"Plex Box"},"Metadata":{"type":"movie","Guid":[{"id":"tmdb://27205"}]}}`)
	if len(h.history(t)) != 1 {
		t.Errorf("a second Plex user isn't matched yet, so nothing should log")
	}
	h.matchEveryone(t)
	send(`{"event":"media.scrobble","Account":{"id":2,"title":"guest"},"Server":{"title":"Plex Box"},"Metadata":{"type":"movie","Guid":[{"id":"tmdb://27205"}]}}`)
	if got := h.history(t); len(got) != 2 || got[0].Item.Kind() != "movie" {
		t.Errorf("film = %+v", got)
	}
	if code := h.post("/hooks/plex/"+secret, "application/json", "{}"); code != http.StatusBadRequest {
		t.Errorf("not a form = %d", code)
	}
}

func TestPlexOldAgents(t *testing.T) {
	h := newHousehold(t)
	_, secret, err := core.AddMediaServer(context.Background(), h.db, h.account, "plex")
	if err != nil {
		t.Fatal(err)
	}
	send := func(kind, guid string) {
		contentType, body := form("payload", `{"event":"media.scrobble","Account":{"id":1,"title":"nathan"},"Server":{"title":"Old Box"},"Metadata":{"type":"`+kind+`","guid":"`+guid+`"}}`)
		if code := h.post("/hooks/plex/"+secret, contentType, body); code != http.StatusNoContent {
			t.Fatalf("%s = %d", guid, code)
		}
	}
	send("movie", "com.plexapp.agents.none://x")
	h.matchEveryone(t)

	send("episode", "com.plexapp.agents.thetvdb://81189/1/2?lang=en")
	send("movie", "com.plexapp.agents.imdb://tt0137523?lang=en")
	send("movie", "com.plexapp.agents.themoviedb://27205?lang=en")
	send("episode", "com.plexapp.agents.themoviedb://1396/2/3?lang=en")
	send("movie", "com.plexapp.agents.none://abc")
	send("episode", "com.plexapp.agents.hama://anidb-1/1/1?lang=en")

	got := h.history(t)
	if len(got) != 4 {
		t.Fatalf("history = %d, want the four the old agents name", len(got))
	}
	var episodes []string
	for _, w := range got {
		if w.Item.Episode != nil {
			episodes = append(episodes, strconv.Itoa(int(w.Item.Episode.SeasonNumber))+"x"+strconv.Itoa(int(w.Item.Episode.EpisodeNumber)))
		}
	}
	if len(episodes) != 2 || episodes[0] != "2x3" || episodes[1] != "1x2" {
		t.Errorf("episodes = %v", episodes)
	}
}

func TestEmby(t *testing.T) {
	h := newHousehold(t)
	_, secret, err := core.AddMediaServer(context.Background(), h.db, h.account, "emby")
	if err != nil {
		t.Fatal(err)
	}
	episode := `{"Event":"playback.stop","User":{"Name":"Sam","Id":"e1"},"Server":{"Name":"Emby","Id":"s"},
		"Item":{"Type":"Episode","RunTimeTicks":34800000000,"ProviderIds":{"Tvdb":"349232","Imdb":"tt0959621"}},
		"PlaybackInfo":{"PlayedToCompletion":true,"PositionTicks":34000000000}}`

	if code := h.post("/hooks/emby/"+secret, "application/json", episode); code != http.StatusNoContent || len(h.history(t)) != 0 {
		t.Fatalf("unmatched = %d", code)
	}
	h.matchEveryone(t)
	h.post("/hooks/emby/"+secret, "application/json", episode)
	if got := h.history(t); len(got) != 1 || got[0].Item.Episode == nil || got[0].Action != "scrobble" {
		t.Fatalf("episode = %+v", got)
	}

	contentType, body := form("data", `{"Event":"playback.pause","User":{"Name":"Sam","Id":"e1"},"Item":{"Type":"Movie","RunTimeTicks":88800000000,"ProviderIds":{"Tmdb":"27205"}},"PlaybackInfo":{"PositionTicks":35520000000}}`)
	if code := h.post("/hooks/emby/"+secret, contentType, body); code != http.StatusNoContent {
		t.Fatalf("paused film as a form = %d", code)
	}
	if p := playback(t, h, "movie"); len(p) != 1 || p[0].Progress != 40 {
		t.Errorf("paused film = %+v", p)
	}
	contentType, body = form("data", `{"Event":"item.markplayed","User":{"Name":"Sam","Id":"e1"},"Item":{"Type":"Movie","ProviderIds":{"Tmdb":"27205"}}}`)
	h.post("/hooks/emby/"+secret, contentType, body)
	if got := h.history(t); len(got) != 2 || got[0].Action != "watch" || got[0].Item.Kind() != "movie" {
		t.Errorf("marked played = %+v", got)
	}
	if code := h.post("/hooks/jellyfin/"+secret, "application/json", `{"NotificationType":"PlaybackStart","UserId":"u1","ItemType":"Movie","Provider_tmdb":"27205"}`); code != http.StatusNotFound {
		t.Errorf("an emby address used for jellyfin = %d", code)
	}
}
