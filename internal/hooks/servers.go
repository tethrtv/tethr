package hooks

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
)

type jellyfinEvent struct {
	NotificationType      string
	ServerName            string
	UserID                string `json:"UserId"`
	NotificationUsername  string
	ItemType              string
	RunTimeTicks          int64
	PlaybackPositionTicks int64
	PlayedToCompletion    bool
	Played                bool
	SaveReason            string
	TMDB                  string `json:"Provider_tmdb"`
	IMDB                  string `json:"Provider_imdb"`
	TVDB                  string `json:"Provider_tvdb"`
}

func jellyfin(r *http.Request) (event, error) {
	var j jellyfinEvent
	if err := json.NewDecoder(r.Body).Decode(&j); err != nil {
		return event{}, errors.New("send all properties as JSON")
	}
	e := event{server: j.ServerName, userID: j.UserID, userName: j.NotificationUsername, tvdb: j.TVDB, imdb: j.IMDB}
	e.ref = ref(strings.ToLower(j.ItemType), j.TMDB, j.IMDB, j.TVDB)
	e.progress, e.duration = ticks(j.PlaybackPositionTicks, j.RunTimeTicks)
	switch {
	case j.NotificationType == "PlaybackStart":
		e.action = started
	case j.NotificationType == "PlaybackStop" && j.PlayedToCompletion:
		e.action = finished
	case j.NotificationType == "PlaybackStop":
		e.action = paused
	case j.NotificationType == "UserDataSaved" && j.SaveReason == "TogglePlayed" && j.Played:
		e.action = marked
	}
	return e, nil
}

type plexEvent struct {
	Event   string `json:"event"`
	Account struct {
		ID    int64  `json:"id"`
		Title string `json:"title"`
	} `json:"Account"`
	Server struct {
		Title string `json:"title"`
	} `json:"Server"`
	Metadata struct {
		Type       string `json:"type"`
		ViewOffset int64  `json:"viewOffset"`
		Duration   int64  `json:"duration"`
		// Plex sends its own guid as a string next to the Guid list, and Go would otherwise try both into one field.
		PlexGUID string `json:"guid"`
		GUIDs    []struct {
			ID string `json:"id"`
		} `json:"Guid"`
	} `json:"Metadata"`
}

func plex(r *http.Request) (event, error) {
	if err := r.ParseMultipartForm(maxBody); err != nil {
		return event{}, errors.New("plex sends a form with a payload field")
	}
	var p plexEvent
	if err := json.Unmarshal([]byte(r.FormValue("payload")), &p); err != nil {
		return event{}, errors.New("the payload field isn't JSON")
	}
	ids := map[string]string{}
	for _, g := range p.Metadata.GUIDs {
		if source, id, ok := strings.Cut(g.ID, "://"); ok {
			ids[source] = id
		}
	}
	e := event{server: p.Server.Title, userID: strconv.FormatInt(p.Account.ID, 10), userName: p.Account.Title, tvdb: ids["tvdb"], imdb: ids["imdb"]}
	e.ref = ref(p.Metadata.Type, ids["tmdb"], ids["imdb"], ids["tvdb"])
	if p.Metadata.Duration > 0 {
		e.progress = 100 * float64(p.Metadata.ViewOffset) / float64(p.Metadata.Duration)
		seconds := float64(p.Metadata.Duration) / 1000
		e.duration = &seconds
	}
	switch p.Event {
	case "media.play", "media.resume":
		e.action = started
	case "media.pause", "media.stop":
		// Plex scrobbles at 90%, so a stop past that is the end of something already logged.
		if e.progress < 90 {
			e.action = paused
		}
	case "media.scrobble":
		e.action = finished
	}
	return e, nil
}

type embyEvent struct {
	Event string
	User  struct {
		ID   string `json:"Id"`
		Name string
	}
	Server struct {
		Name string
	}
	Item struct {
		Type         string
		RunTimeTicks int64
		ProviderIDs  map[string]string `json:"ProviderIds"`
	}
	PlaybackInfo struct {
		PlayedToCompletion bool
		PositionTicks      int64
	}
}

func emby(r *http.Request) (event, error) {
	var body []byte
	if media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); media == "multipart/form-data" {
		if err := r.ParseMultipartForm(maxBody); err != nil {
			return event{}, errors.New("emby's form couldn't be read")
		}
		body = []byte(r.FormValue("data"))
	} else {
		read, err := io.ReadAll(r.Body)
		if err != nil {
			return event{}, errors.New("the webhook couldn't be read")
		}
		body = read
	}
	var m embyEvent
	if err := json.Unmarshal(body, &m); err != nil {
		return event{}, errors.New("the webhook isn't JSON")
	}
	ids := map[string]string{}
	for source, id := range m.Item.ProviderIDs {
		ids[strings.ToLower(source)] = id
	}
	e := event{server: m.Server.Name, userID: m.User.ID, userName: m.User.Name, tvdb: ids["tvdb"], imdb: ids["imdb"]}
	e.ref = ref(strings.ToLower(m.Item.Type), ids["tmdb"], ids["imdb"], ids["tvdb"])
	e.progress, e.duration = ticks(m.PlaybackInfo.PositionTicks, m.Item.RunTimeTicks)
	switch {
	case m.Event == "playback.start", m.Event == "playback.unpause":
		e.action = started
	case m.Event == "playback.stop" && m.PlaybackInfo.PlayedToCompletion:
		e.action = finished
	case m.Event == "playback.pause", m.Event == "playback.stop":
		e.action = paused
	case m.Event == "item.markplayed":
		e.action = marked
	}
	return e, nil
}

func ticks(position, runtime int64) (float64, *float64) {
	if runtime <= 0 {
		return 0, nil
	}
	seconds := float64(runtime) / 1e7
	return 100 * float64(position) / float64(runtime), &seconds
}
