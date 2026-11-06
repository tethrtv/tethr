package hooks

import (
	"context"
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

const maxBody = 1 << 20

type Server struct {
	DB   *pgxpool.Pool
	TMDB *tmdb.Client
	Log  *slog.Logger

	once   sync.Once
	events *limit.Limiter
}

type action int

const (
	ignore action = iota
	started
	paused
	finished
	marked
)

type event struct {
	server   string
	userID   string
	userName string
	action   action
	ref      core.Ref
	tvdb     string
	imdb     string
	progress float64
	duration *float64
}

func (s *Server) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /hooks/jellyfin/{secret}", s.receive("jellyfin", jellyfin))
	mux.HandleFunc("POST /hooks/plex/{secret}", s.receive("plex", plex))
	mux.HandleFunc("POST /hooks/emby/{secret}", s.receive("emby", emby))
}

func (s *Server) receive(kind string, read func(*http.Request) (event, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		secret := r.PathValue("secret")
		if !s.allow(w, secret) {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		e, err := read(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if e.action == ignore {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		ctx := r.Context()
		c, matched, err := core.ServerCaller(ctx, s.DB, core.ServerEvent{Kind: kind, Secret: secret, ServerName: e.server, UserID: e.userID, UserName: e.userName})
		if errors.Is(err, core.ErrNotFound) {
			http.Error(w, "there's no media server with this address", http.StatusNotFound)
			return
		}
		if err == nil && matched && e.ref.Kind != "" {
			err = s.log(ctx, c, e)
		}
		var already core.AlreadyScrobbled
		switch {
		case err == nil, errors.As(err, &already), errors.Is(err, core.ErrProgressTooLow):
		case errors.Is(err, core.ErrNotFound):
			s.Log.Info("media server sent something tethr can't match", "kind", kind, "type", e.ref.Kind, "tmdb", e.ref.IDs.TMDB, "tvdb", e.tvdb, "imdb", e.imdb)
		default:
			s.Log.Error("media server event failed", "kind", kind, "err", err)
			http.Error(w, "something went wrong", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) log(ctx context.Context, c core.Caller, e event) error {
	send := func(ref core.Ref) error {
		scrobble := core.Scrobble{Ref: ref, Progress: e.progress, Duration: e.duration}
		var err error
		switch e.action {
		case started:
			_, err = core.ScrobbleStart(ctx, s.DB, c, scrobble)
		case paused:
			_, err = core.ScrobblePause(ctx, s.DB, c, scrobble)
		case finished:
			scrobble.Progress = 100
			_, err = core.ScrobbleStop(ctx, s.DB, c, scrobble)
		case marked:
			var result core.SyncResult
			result, err = core.AddHistory(ctx, s.DB, c, []core.Watch{{Ref: ref, At: time.Now()}})
			if err == nil && len(result.NotFound) > 0 {
				err = core.ErrNotFound
			}
		}
		return err
	}
	err := send(e.ref)
	if errors.Is(err, core.ErrNotFound) && e.ref.Kind == "episode" {
		if found, ok := s.findShow(ctx, e); ok {
			err = send(found)
		}
	}
	return err
}

// Servers often send only the episode's own IDs, so a show Tethr hasn't seen yet is found through TMDB.
func (s *Server) findShow(ctx context.Context, e event) (core.Ref, bool) {
	if s.TMDB == nil {
		return core.Ref{}, false
	}
	for _, id := range []struct{ source, value string }{{"tvdb_id", e.tvdb}, {"imdb_id", e.imdb}} {
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

func ref(kind, tmdbID, imdb, tvdb string) core.Ref {
	id, _ := strconv.ParseInt(tmdbID, 10, 64)
	switch kind {
	case "movie":
		if id > 0 || imdb != "" {
			return core.Ref{Kind: "movie", IDs: core.IDs{TMDB: id, IMDB: imdb}}
		}
	case "episode":
		if id > 0 || imdb != "" || tvdb != "" {
			return core.Ref{Kind: "episode", IDs: core.IDs{TMDB: id}}
		}
	}
	return core.Ref{}
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
