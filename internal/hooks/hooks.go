package hooks

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/limit"
	"github.com/tethrtv/tethr/internal/tmdb"
)

// Marking a whole series played sends one event per episode, and servers don't retry.
var serverLimit = limit.Rule{Name: "MEDIA_SERVER_LIMIT", Limit: 600, Period: time.Minute, Burst: 600}

type Server struct {
	DB   *pgxpool.Pool
	TMDB *tmdb.Client
	Log  *slog.Logger

	once   sync.Once
	events *limit.Limiter
}

func (s *Server) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /hooks/jellyfin/{secret}", s.jellyfin)
}

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

func (s *Server) jellyfin(w http.ResponseWriter, r *http.Request) {
	secret := r.PathValue("secret")
	if !s.allow(w, secret) {
		return
	}
	var e jellyfinEvent
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&e); err != nil {
		http.Error(w, "send all properties as JSON", http.StatusBadRequest)
		return
	}
	switch e.NotificationType {
	case "PlaybackStart", "PlaybackStop", "UserDataSaved":
	default:
		w.WriteHeader(http.StatusNoContent)
		return
	}
	ctx := r.Context()
	c, matched, err := core.ServerCaller(ctx, s.DB, core.ServerEvent{Kind: "jellyfin", Secret: secret, ServerName: e.ServerName, UserID: e.UserID, UserName: e.NotificationUsername})
	if errors.Is(err, core.ErrNotFound) {
		http.Error(w, "there's no media server with this address", http.StatusNotFound)
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	ref, ok := jellyfinRef(e)
	if !matched || !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	scrobble := core.Scrobble{Ref: ref}
	if e.RunTimeTicks > 0 {
		scrobble.Progress = 100 * float64(e.PlaybackPositionTicks) / float64(e.RunTimeTicks)
		seconds := float64(e.RunTimeTicks) / 1e7
		scrobble.Duration = &seconds
	}
	var log func(core.Ref) error
	switch {
	case e.NotificationType == "PlaybackStart":
		log = func(ref core.Ref) error {
			scrobble.Ref = ref
			_, err := core.ScrobbleStart(ctx, s.DB, c, scrobble)
			return err
		}
	case e.NotificationType == "PlaybackStop" && e.PlayedToCompletion:
		log = func(ref core.Ref) error {
			scrobble.Ref, scrobble.Progress = ref, 100
			_, err := core.ScrobbleStop(ctx, s.DB, c, scrobble)
			return err
		}
	case e.NotificationType == "PlaybackStop":
		log = func(ref core.Ref) error {
			scrobble.Ref = ref
			_, err := core.ScrobblePause(ctx, s.DB, c, scrobble)
			return err
		}
	case e.SaveReason == "TogglePlayed" && e.Played:
		log = func(ref core.Ref) error {
			result, err := core.AddHistory(ctx, s.DB, c, []core.Watch{{Ref: ref, At: time.Now()}})
			if err == nil && len(result.NotFound) > 0 {
				return core.ErrNotFound
			}
			return err
		}
	default:
		w.WriteHeader(http.StatusNoContent)
		return
	}

	err = log(ref)
	if errors.Is(err, core.ErrNotFound) && ref.Kind == "episode" {
		if found, ok := s.findShow(ctx, e); ok {
			err = log(found)
		}
	}
	var already core.AlreadyScrobbled
	switch {
	case err == nil, errors.As(err, &already), errors.Is(err, core.ErrProgressTooLow):
	case errors.Is(err, core.ErrNotFound):
		s.Log.Info("media server sent something tethr can't match", "kind", "jellyfin", "type", e.ItemType, "tmdb", e.TMDB, "tvdb", e.TVDB, "imdb", e.IMDB)
	default:
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func jellyfinRef(e jellyfinEvent) (core.Ref, bool) {
	tmdbID, _ := strconv.ParseInt(e.TMDB, 10, 64)
	switch e.ItemType {
	case "Movie":
		return core.Ref{Kind: "movie", IDs: core.IDs{TMDB: tmdbID, IMDB: e.IMDB}}, tmdbID > 0 || e.IMDB != ""
	case "Episode":
		return core.Ref{Kind: "episode", IDs: core.IDs{TMDB: tmdbID}}, tmdbID > 0 || e.TVDB != "" || e.IMDB != ""
	}
	return core.Ref{}, false
}

// Jellyfin only sends the episode's own IDs, so a show Tethr hasn't seen yet is found through TMDB.
func (s *Server) findShow(ctx context.Context, e jellyfinEvent) (core.Ref, bool) {
	if s.TMDB == nil {
		return core.Ref{}, false
	}
	for _, id := range []struct{ source, value string }{{"tvdb_id", e.TVDB}, {"imdb_id", e.IMDB}} {
		if id.value == "" {
			continue
		}
		found, err := s.TMDB.FindEpisode(ctx, id.source, id.value)
		if errors.Is(err, tmdb.ErrNotFound) {
			continue
		}
		if err != nil {
			s.Log.Error("finding an episode on tmdb", "err", err)
			return core.Ref{}, false
		}
		season, number := found.Season, found.Number
		return core.Ref{Kind: "episode", IDs: core.IDs{TMDB: int64(found.ID)}, Show: core.IDs{TMDB: int64(found.ShowID)}, Season: &season, Number: &number}, true
	}
	return core.Ref{}, false
}

func (s *Server) allow(w http.ResponseWriter, secret string) bool {
	s.once.Do(func() { s.events = limit.New(serverLimit) })
	now := time.Now()
	wait, ok := s.events.Allow(secret, now)
	if !ok {
		limit.Refuse(w, serverLimit, wait, now)
		http.Error(w, "too many events, try again in a moment", http.StatusTooManyRequests)
	}
	return ok
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	s.Log.Error("media server event failed", "kind", "jellyfin", "err", err)
	http.Error(w, "something went wrong", http.StatusInternalServerError)
}
