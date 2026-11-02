package meta

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"

	"github.com/tethrtv/tethr/internal/tmdb"
)

const (
	titleCache  = "public, max-age=86400, s-maxage=2592000, stale-while-revalidate=604800"
	seasonCache = "public, max-age=86400, s-maxage=604800, stale-while-revalidate=86400"
)

type Server struct {
	TMDB *tmdb.Client
	Log  *slog.Logger
}

type title struct {
	Name   string  `json:"name"`
	Year   *int    `json:"year"`
	Poster *string `json:"poster"`
}

type episode struct {
	Number  int32   `json:"number"`
	Name    string  `json:"name"`
	AirDate *string `json:"air_date"`
}

func (s *Server) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /meta/movie/{id}", s.ownSite(s.title(s.TMDB.Movie)))
	mux.HandleFunc("GET /meta/tv/{id}", s.ownSite(s.title(s.TMDB.Show)))
	mux.HandleFunc("GET /meta/tv/{id}/season/{season}", s.ownSite(s.season))
}

// Only Tethr's own pages may ask; apps fetch from TMDB with their own keys.
func (s *Server) ownSite(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("Sec-Fetch-Site") {
		case "same-origin", "same-site":
			w.Header().Set("Access-Control-Allow-Origin", "*")
			next(w, r)
		default:
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, "only Tethr's own website can use this", http.StatusForbidden)
		}
	}
}

func (s *Server) title(fetch func(context.Context, int32) (tmdb.Title, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		t, err := fetch(r.Context(), id)
		if !s.found(w, r, err) {
			return
		}
		out := title{Name: t.Name}
		if t.Year > 0 {
			out.Year = &t.Year
		}
		if t.Poster != "" {
			out.Poster = &t.Poster
		}
		write(w, titleCache, out)
	}
}

func (s *Server) season(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	n, ok := pathID(w, r, "season")
	if !ok {
		return
	}
	episodes, err := s.TMDB.Season(r.Context(), id, n)
	if !s.found(w, r, err) {
		return
	}
	out := make([]episode, 0, len(episodes))
	for _, e := range episodes {
		ep := episode{Number: e.Number, Name: e.Name}
		if e.AirDate != "" {
			ep.AirDate = &e.AirDate
		}
		out = append(out, ep)
	}
	write(w, seasonCache, map[string]any{"episodes": out})
}

func (s *Server) found(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case errors.Is(err, tmdb.ErrNotFound):
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.NotFound(w, r)
	case err != nil:
		s.Log.Error("tmdb lookup failed", "path", r.URL.Path, "err", err)
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, "tmdb didn't answer", http.StatusBadGateway)
	default:
		return true
	}
	return false
}

func pathID(w http.ResponseWriter, r *http.Request, name string) (int32, bool) {
	n, err := strconv.ParseInt(r.PathValue(name), 10, 32)
	if err != nil || n < 0 || n > math.MaxInt32 {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.NotFound(w, r)
		return 0, false
	}
	return int32(n), true
}

func write(w http.ResponseWriter, cache string, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", cache)
	_ = json.NewEncoder(w).Encode(v)
}
