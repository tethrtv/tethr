package core

import (
	"context"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

const (
	searchResults   = 20
	matchCandidates = 5
)

type FilmName struct {
	Name string
	Year int
}

type FilmMatch struct {
	Match      *Item
	Candidates []Item
}

func Search(ctx context.Context, db *pgxpool.Pool, profileID int64, words, kind, scope string) ([]Item, error) {
	words = strings.TrimSpace(words)
	if words == "" {
		return []Item{}, nil
	}
	q := store.New(db)
	params := store.SearchLibraryParams{ProfileID: profileID, Words: likeEscaper.Replace(words)}
	if kind != "" {
		params.Kind = &kind
	}
	ids, err := q.SearchLibrary(ctx, params)
	if err != nil {
		return nil, err
	}
	if scope == "catalogue" {
		p, err := q.ProfileByID(ctx, profileID)
		if err != nil {
			return nil, err
		}
		more, err := q.SearchCatalogue(ctx, store.SearchCatalogueParams{Words: words, Kids: p.Kind == "kids", Kind: params.Kind, Max: searchResults})
		if err != nil {
			return nil, err
		}
		for _, id := range more {
			if len(ids) < searchResults && !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
	}
	return items(ctx, q, ids)
}

func items(ctx context.Context, q *store.Queries, ids []int64) ([]Item, error) {
	c, err := loadCatalogue(ctx, q, 0, ids, nil)
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(ids))
	for _, id := range ids {
		out = append(out, c.item(id, nil, nil))
	}
	return out, nil
}

func MatchFilms(ctx context.Context, db *pgxpool.Pool, films []FilmName) ([]FilmMatch, error) {
	q := store.New(db)
	movie := "movie"
	matches := make([]int64, len(films))
	candidates := make([][]int64, len(films))
	var all []int64
	for i, f := range films {
		name := strings.TrimSpace(f.Name)
		if name == "" || len(name) > 200 {
			continue
		}
		named, err := q.FilmsNamed(ctx, name)
		if err != nil {
			return nil, err
		}
		matches[i] = onlyFilm(named, f.Year, 0)
		if candidates[i], err = q.SearchCatalogue(ctx, store.SearchCatalogueParams{Words: name, Kind: &movie, Max: matchCandidates}); err != nil {
			return nil, err
		}
		if near := onlyFilm(named, f.Year, 1); matches[i] == 0 && near != 0 {
			candidates[i] = append([]int64{near}, slices.DeleteFunc(candidates[i], func(id int64) bool { return id == near })...)
			candidates[i] = candidates[i][:min(len(candidates[i]), matchCandidates)]
		}
		all = append(all, candidates[i]...)
		if matches[i] != 0 {
			all = append(all, matches[i])
		}
	}
	c, err := loadCatalogue(ctx, q, 0, all, nil)
	if err != nil {
		return nil, err
	}
	out := make([]FilmMatch, len(films))
	for i := range films {
		out[i].Candidates = make([]Item, 0, len(candidates[i]))
		for _, id := range candidates[i] {
			out[i].Candidates = append(out[i].Candidates, c.item(id, nil, nil))
		}
		if matches[i] != 0 {
			m := c.item(matches[i], nil, nil)
			out[i].Match = &m
		}
	}
	return out, nil
}

func onlyFilm(named []store.FilmsNamedRow, year, within int) int64 {
	if year <= 0 {
		return 0
	}
	var found int64
	for _, n := range named {
		if int(n.Year) >= year-within && int(n.Year) <= year+within {
			if found != 0 {
				return 0
			}
			found = n.ID
		}
	}
	return found
}
