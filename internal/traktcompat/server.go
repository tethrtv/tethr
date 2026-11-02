package traktcompat

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/core"
)

type Server struct {
	DB     *pgxpool.Pool
	AppURL string
	Log    *slog.Logger
}

type callerKey struct{}

func (s *Server) Routes(mux *http.ServeMux) {
	handle := func(pattern string, h http.HandlerFunc) { mux.Handle(pattern, s.cors(h)) }
	app := func(pattern string, h http.HandlerFunc) { handle(pattern, s.withApp(h)) }
	user := func(pattern string, h http.HandlerFunc) { handle(pattern, s.withApp(s.withCaller(h))) }

	handle("OPTIONS /oauth/", preflight)
	handle("POST /oauth/device/code", s.deviceCode)
	handle("POST /oauth/device/token", s.deviceToken)
	handle("POST /oauth/token", s.token)
	handle("POST /oauth/revoke", s.revoke)
	handle("GET /oauth/authorize", s.authorize)

	for _, prefix := range []string{"/scrobble/", "/sync/", "/users/", "/search/", "/lists/", "/shows/"} {
		handle("OPTIONS "+prefix, preflight)
	}
	user("POST /scrobble/start", s.scrobble(core.ScrobbleStart))
	user("POST /scrobble/pause", s.scrobble(core.ScrobblePause))
	user("POST /scrobble/stop", s.scrobble(core.ScrobbleStop))

	user("GET /sync/last_activities", s.lastActivities)
	user("GET /sync/playback", s.playback)
	user("GET /sync/playback/{type}", s.playback)
	user("DELETE /sync/playback/{id}", s.removePlayback)
	user("GET /sync/history", s.history)
	user("GET /sync/history/{type}", s.history)
	user("GET /sync/history/{type}/{id}", s.history)
	user("POST /sync/history", s.addHistory)
	user("POST /sync/history/remove", s.removeHistory)
	user("GET /sync/watchlist", s.watchlist)
	user("GET /sync/watchlist/{type}", s.watchlist)
	user("GET /sync/watchlist/{type}/{sort_by}", s.watchlist)
	user("GET /sync/watchlist/{type}/{sort_by}/{sort_how}", s.watchlist)
	user("POST /sync/watchlist", s.addToWatchlist)
	user("POST /sync/watchlist/remove", s.removeFromWatchlist)

	user("GET /sync/ratings", s.ratings)
	user("GET /sync/ratings/{type}", s.ratings)
	user("GET /sync/ratings/{type}/{rating}", s.ratings)
	user("POST /sync/ratings", s.rate)
	user("POST /sync/ratings/remove", s.unrate)
	user("GET /sync/watched/{type}", s.watched)
	user("GET /sync/progress/up_next", s.upNext)
	user("GET /shows/{id}/progress/watched", s.showProgress)

	for _, section := range core.HiddenSections {
		user("GET /users/hidden/"+section, s.hidden(section))
		user("POST /users/hidden/"+section, s.hide(section, false))
		user("POST /users/hidden/"+section+"/remove", s.hide(section, true))
	}
	user("GET /users/settings", s.settings)
	app("GET /users/{id}/lists", empty)
	app("GET /users/{id}/lists/collaborations", empty)
	app("GET /users/{id}/likes", empty)
	app("GET /users/{id}/likes/{type}", empty)
	app("GET /lists/{id}/items", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "Lists aren't shared on Tethr yet.")
	})
	app("GET /search/{id_type}/{id}", s.lookUp)
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Expose-Headers", "X-Pagination-Page, X-Pagination-Limit, X-Pagination-Page-Count, X-Pagination-Item-Count, X-Sort-By, X-Sort-How")
		next.ServeHTTP(w, r)
	})
}

func preflight(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, trakt-api-key, trakt-api-version")
	w.Header().Set("Access-Control-Max-Age", "600")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) withApp(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := core.KnownApp(r.Context(), s.DB, r.Header.Get("trakt-api-key"))
		if errors.Is(err, core.ErrUnknownApp) {
			writeError(w, http.StatusForbidden, "invalid_api_key", "Send a client ID Tethr knows in the trakt-api-key header.")
			return
		}
		if err != nil {
			s.fail(w, r, err)
			return
		}
		next(w, r)
	}
}

func (s *Server) withCaller(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || token == "" {
			writeError(w, http.StatusUnauthorized, "invalid_token", "Sign in first.")
			return
		}
		caller, err := core.TokenCaller(r.Context(), s.DB, token)
		if errors.Is(err, core.ErrInvalidGrant) {
			writeError(w, http.StatusUnauthorized, "invalid_token", "That access token has expired or been revoked.")
			return
		}
		if err != nil {
			s.fail(w, r, err)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), callerKey{}, caller)))
	}
}

func caller(r *http.Request) core.Caller {
	c, _ := r.Context().Value(callerKey{}).(core.Caller)
	return c
}

func empty(w http.ResponseWriter, r *http.Request) {
	writePage(w, 1, 10, 0)
	writeJSON(w, http.StatusOK, []any{})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, description string) {
	writeJSON(w, status, map[string]string{"error": code, "error_description": description})
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	s.Log.Error("request failed", "path", r.URL.Path, "err", err)
	writeError(w, http.StatusInternalServerError, "server_error", "Something went wrong on our side.")
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "That request body couldn't be read.")
		return false
	}
	return true
}

func paging(r *http.Request, defaultLimit int) core.Page {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = defaultLimit
	}
	return core.Page{Page: page, Limit: min(limit, 1000)}
}

func writePage(w http.ResponseWriter, page, limit int, total int64) {
	pages := int(math.Ceil(float64(total) / float64(limit)))
	h := w.Header()
	h.Set("X-Pagination-Page", strconv.Itoa(page))
	h.Set("X-Pagination-Limit", strconv.Itoa(limit))
	h.Set("X-Pagination-Page-Count", strconv.Itoa(max(pages, 1)))
	h.Set("X-Pagination-Item-Count", strconv.FormatInt(total, 10))
}
