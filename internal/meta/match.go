package meta

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/tethrtv/tethr/internal/limit"
	"github.com/tethrtv/tethr/internal/tmdb"
	"golang.org/x/text/unicode/norm"
)

const (
	matchBatch      = 100
	matchCandidates = 5
	matchWorkers    = 8
)

var matchLimit = limit.Rule{Name: "MATCH_LIMIT", Limit: 120, Period: time.Hour, Burst: 120}

type film struct {
	Name string `json:"name"`
	Year int    `json:"year"`
}

type matched struct {
	Match      *result  `json:"match"`
	Candidates []result `json:"candidates"`
}

func (s *Server) match(w http.ResponseWriter, r *http.Request, accountID int64) {
	var body struct {
		Films []film `json:"films"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil || len(body.Films) == 0 || len(body.Films) > matchBatch {
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, "send between 1 and 100 films", http.StatusBadRequest)
		return
	}
	s.once.Do(s.limits)
	now := time.Now()
	if wait, ok := s.matches.Allow(strconv.FormatInt(accountID, 10), now); !ok {
		w.Header().Set("Cache-Control", "no-store")
		limit.Refuse(w, matchLimit, wait, now)
		http.Error(w, "too many films at once, try again in a moment", http.StatusTooManyRequests)
		return
	}

	out := make([]matched, len(body.Films))
	errs := make([]error, len(body.Films))
	next := make(chan int)
	var wg sync.WaitGroup
	for range matchWorkers {
		wg.Go(func() {
			for i := range next {
				out[i], errs[i] = s.matchFilm(r.Context(), body.Films[i])
			}
		})
	}
	for i := range body.Films {
		next <- i
	}
	close(next)
	wg.Wait()
	for _, err := range errs {
		if !s.found(w, r, err) {
			return
		}
	}
	write(w, "no-store", map[string]any{"results": out})
}

func (s *Server) matchFilm(ctx context.Context, f film) (matched, error) {
	name := strings.TrimSpace(f.Name)
	if name == "" || len(name) > 200 {
		return matched{Candidates: []result{}}, nil
	}
	found, err := s.TMDB.SearchFilms(ctx, name, f.Year)
	if err != nil {
		return matched{}, err
	}
	named := func(r tmdb.Result) bool { return fold(r.Name) == fold(name) || fold(r.Original) == fold(name) }
	same := sameFilm(found, named, f.Year)
	if len(same) != 1 && f.Year > 0 {
		more, err := s.TMDB.SearchFilms(ctx, name, 0)
		if err != nil {
			return matched{}, err
		}
		for _, r := range more {
			if !slices.ContainsFunc(found, func(x tmdb.Result) bool { return x.ID == r.ID }) {
				found = append(found, r)
			}
		}
		same = sameFilm(found, named, f.Year)
	}
	out := matched{Candidates: make([]result, 0, matchCandidates)}
	if len(same) == 1 {
		m := toResult(same[0])
		out.Match = &m
	}
	slices.SortStableFunc(found, func(a, b tmdb.Result) int {
		switch {
		case named(a) && !named(b):
			return -1
		case named(b) && !named(a):
			return 1
		}
		return 0
	})
	for _, r := range found[:min(matchCandidates, len(found))] {
		out.Candidates = append(out.Candidates, toResult(r))
	}
	return out, nil
}

func sameFilm(found []tmdb.Result, named func(tmdb.Result) bool, year int) []tmdb.Result {
	var out []tmdb.Result
	for _, r := range found {
		if named(r) && year > 0 && r.Year >= year-1 && r.Year <= year+1 {
			out = append(out, r)
		}
	}
	return out
}

func toResult(r tmdb.Result) result {
	res := result{Type: r.Kind, TMDB: r.ID, Name: r.Name, Poster: optional(r.Poster)}
	if r.Year > 0 {
		res.Year = &r.Year
	}
	return res
}

func fold(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ReplaceAll(s, "&", " and ")) {
		switch {
		case unicode.Is(unicode.Mn, r):
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}
