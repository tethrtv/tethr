package meta

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/tethrtv/tethr/internal/limit"
	"github.com/tethrtv/tethr/internal/tmdb"
)

const (
	movieCache  = "public, max-age=86400, s-maxage=2592000, stale-while-revalidate=604800"
	showCache   = "public, max-age=3600, s-maxage=86400, stale-while-revalidate=86400"
	seasonCache = "public, max-age=86400, s-maxage=604800, stale-while-revalidate=86400"
	searchCache = "private, max-age=300"

	searchResults = 10
)

var groupID = regexp.MustCompile(`^[0-9a-f]{24}$`)

var searchLimit = limit.Rule{Name: "SEARCH_LIMIT", Limit: 30, Period: time.Minute, Burst: 30}

type Server struct {
	TMDB     *tmdb.Client
	Log      *slog.Logger
	SignedIn func(func(http.ResponseWriter, *http.Request, int64)) http.Handler

	once     sync.Once
	searches *limit.Limiter
}

type title struct {
	Name        string       `json:"name"`
	Year        *int         `json:"year"`
	Poster      *string      `json:"poster"`
	Overview    string       `json:"overview,omitempty"`
	Tagline     string       `json:"tagline,omitempty"`
	Genres      []string     `json:"genres,omitempty"`
	Runtime     int          `json:"runtime,omitempty"`
	Certificate *certificate `json:"certificate,omitempty"`
	Creators    []string     `json:"creators,omitempty"`
	Cast        []string     `json:"cast,omitempty"`
	Networks    []string     `json:"networks,omitempty"`
	Status      string       `json:"status,omitempty"`
	Next        *episode     `json:"next_episode,omitempty"`
	Seasons     []season     `json:"seasons,omitempty"`
}

type season struct {
	Number   int32 `json:"number"`
	Episodes int   `json:"episodes"`
}

type order struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     int16  `json:"type"`
	Seasons  int    `json:"seasons"`
	Episodes int    `json:"episodes"`
}

type orderSeason struct {
	Number   int32          `json:"number"`
	Name     string         `json:"name"`
	Episodes []orderEpisode `json:"episodes"`
}

type orderEpisode struct {
	Number   int32   `json:"number"`
	Aired    aired   `json:"aired"`
	Name     string  `json:"name"`
	AirDate  *string `json:"air_date"`
	Overview string  `json:"overview,omitempty"`
	Runtime  int     `json:"runtime,omitempty"`
}

type aired struct {
	Season int32 `json:"season"`
	Number int32 `json:"number"`
}

type result struct {
	Type   string  `json:"type"`
	TMDB   int32   `json:"tmdb"`
	Name   string  `json:"name"`
	Year   *int    `json:"year"`
	Poster *string `json:"poster"`
}

type certificate struct {
	Country string `json:"country"`
	Rating  string `json:"rating"`
}

type episode struct {
	Season   int32   `json:"season,omitempty"`
	Number   int32   `json:"number"`
	Name     string  `json:"name"`
	AirDate  *string `json:"air_date"`
	Overview string  `json:"overview,omitempty"`
	Runtime  int     `json:"runtime,omitempty"`
}

func (s *Server) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /meta/movie/{id}", s.ownSite(s.title(s.TMDB.Movie, movieCache)))
	mux.HandleFunc("GET /meta/tv/{id}", s.ownSite(s.title(s.TMDB.Show, showCache)))
	mux.HandleFunc("GET /meta/tv/{id}/season/{season}", s.ownSite(s.season))
	mux.HandleFunc("GET /meta/tv/{id}/orders", s.ownSite(s.orders))
	mux.HandleFunc("GET /meta/tv/{id}/order/{group}", s.ownSite(s.order))
	mux.Handle("GET /meta/search", s.SignedIn(s.search))
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

func (s *Server) title(fetch func(context.Context, int32) (tmdb.Title, error), cache string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		t, err := fetch(r.Context(), id)
		if !s.found(w, r, err) {
			return
		}
		out := title{Name: t.Name, Overview: t.Overview, Tagline: t.Tagline, Genres: t.Genres, Runtime: t.Runtime, Creators: t.Creators, Cast: t.Cast, Networks: t.Networks, Status: t.Status}
		if t.Year > 0 {
			out.Year = &t.Year
		}
		if t.Poster != "" {
			out.Poster = &t.Poster
		}
		if c := t.Certificate; c.Rating != "" {
			out.Certificate = &certificate{Country: c.Country, Rating: c.Rating}
		}
		for _, c := range t.Seasons {
			out.Seasons = append(out.Seasons, season{Number: c.Number, Episodes: c.Episodes})
		}
		if n := t.Next; n != nil {
			out.Next = &episode{Season: n.Season, Number: n.Number, Name: n.Name, AirDate: optional(n.AirDate)}
		}
		write(w, cache, out)
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
	season, err := s.TMDB.Season(r.Context(), id, n)
	if !s.found(w, r, err) {
		return
	}
	out := make([]episode, 0, len(season.Episodes))
	for _, e := range season.Episodes {
		out = append(out, episode{Number: e.Number, Name: e.Name, AirDate: optional(e.AirDate), Overview: e.Overview, Runtime: e.Runtime})
	}
	write(w, seasonCache, map[string]any{"overview": season.Overview, "episodes": out})
}

func (s *Server) orders(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	groups, err := s.TMDB.Groups(r.Context(), id)
	if !s.found(w, r, err) {
		return
	}
	out := make([]order, 0, len(groups))
	for _, g := range groups {
		out = append(out, order(g))
	}
	write(w, showCache, map[string]any{"orders": out})
}

func (s *Server) order(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	group := r.PathValue("group")
	if !groupID.MatchString(group) {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.NotFound(w, r)
		return
	}
	g, err := s.TMDB.EpisodeGroup(r.Context(), group)
	if !s.found(w, r, err) {
		return
	}
	seasons := make([]orderSeason, 0, len(g.Seasons))
	for _, season := range g.Seasons {
		out := orderSeason{Number: season.Number, Name: season.Name, Episodes: []orderEpisode{}}
		for _, e := range season.Episodes {
			if e.ShowID != id {
				w.Header().Set("Cache-Control", "public, max-age=86400")
				http.NotFound(w, r)
				return
			}
			out.Episodes = append(out.Episodes, orderEpisode{Number: e.Place, Aired: aired{e.Season, e.Number}, Name: e.Name, AirDate: optional(e.AirDate), Overview: e.Overview, Runtime: e.Runtime})
		}
		seasons = append(seasons, out)
	}
	write(w, seasonCache, map[string]any{"name": g.Name, "type": g.Type, "seasons": seasons})
}

func (s *Server) search(w http.ResponseWriter, r *http.Request, accountID int64) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if utf8.RuneCountInString(q) < 2 || len(q) > 100 {
		write(w, "no-store", map[string]any{"results": []result{}})
		return
	}
	s.once.Do(func() { s.searches = limit.New(searchLimit) })
	now := time.Now()
	if wait, ok := s.searches.Allow(strconv.FormatInt(accountID, 10), now); !ok {
		w.Header().Set("Cache-Control", "no-store")
		limit.Refuse(w, searchLimit, wait, now)
		http.Error(w, "too many searches, try again in a moment", http.StatusTooManyRequests)
		return
	}
	found, err := s.TMDB.Search(r.Context(), q)
	if !s.found(w, r, err) {
		return
	}
	out := make([]result, 0, searchResults)
	for _, f := range found[:min(searchResults, len(found))] {
		res := result{Type: f.Kind, TMDB: f.ID, Name: f.Name, Poster: optional(f.Poster)}
		if f.Year > 0 {
			res.Year = &f.Year
		}
		out = append(out, res)
	}
	write(w, searchCache, map[string]any{"results": out})
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

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
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
