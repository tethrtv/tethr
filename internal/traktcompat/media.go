package traktcompat

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/tethrtv/tethr/internal/core"
)

const timeLayout = "2006-01-02T15:04:05.000Z"

func stamp(t time.Time) string { return t.UTC().Format(timeLayout) }

// Trakt clients cache Trakt's own IDs, so Tethr's go out from a billion or a trillion up, above any Trakt has issued.
const (
	catalogueOffset = 999_999_999
	listOffset      = 999_999_999
	entryOffset     = 999_999_999_999
)

func traktID(id, offset int64) int64 {
	if id == 0 {
		return 0
	}
	return id + offset
}

func tethrID(n, offset int64) int64 {
	if n <= offset {
		return 0
	}
	return n - offset
}

type ids struct {
	Trakt int64   `json:"trakt"`
	Slug  *string `json:"slug,omitempty"`
	IMDB  *string `json:"imdb"`
	TMDB  *int32  `json:"tmdb"`
	TVDB  *int64  `json:"tvdb,omitempty"`
}

type title struct {
	Title string `json:"title"`
	Year  *int16 `json:"year"`
	IDs   ids    `json:"ids"`
}

type season struct {
	Number int32 `json:"number"`
	IDs    ids   `json:"ids"`
}

type episode struct {
	Season int32   `json:"season"`
	Number int32   `json:"number"`
	Title  *string `json:"title"`
	IDs    ids     `json:"ids"`
}

func titleJSON(i core.Item) title {
	t := i.Title
	out := title{Year: t.Year, IDs: ids{Trakt: traktID(t.ID, catalogueOffset), Slug: &t.Slug, TMDB: t.TmdbID}}
	if t.Name != nil {
		out.Title = *t.Name
	}
	if t.Imdb != "" {
		out.IDs.IMDB = &t.Imdb
	}
	if t.Kind == "show" {
		tvdb, err := strconv.ParseInt(t.Tvdb, 10, 64)
		if err == nil {
			out.IDs.TVDB = &tvdb
		}
	}
	return out
}

func episodeJSON(i core.Item) episode {
	e := i.Episode
	return episode{Season: e.Season, Number: e.Number, IDs: ids{Trakt: traktID(e.ID, catalogueOffset), TMDB: e.TmdbEpisodeID}}
}

func itemFields(i core.Item, out map[string]any) map[string]any {
	switch i.Kind() {
	case "movie":
		out["movie"] = titleJSON(i)
	case "show":
		out["show"] = titleJSON(i)
	case "season":
		out["season"] = season{Number: i.Season.SeasonNumber, IDs: ids{Trakt: traktID(i.Season.ID, catalogueOffset)}}
		out["show"] = titleJSON(i)
	case "episode":
		out["episode"] = episodeJSON(i)
		out["show"] = titleJSON(i)
	}
	return out
}

type idsIn struct {
	Trakt int64  `json:"trakt"`
	Slug  string `json:"slug"`
	IMDB  string `json:"imdb"`
	TMDB  int64  `json:"tmdb"`
	TVDB  int64  `json:"tvdb"`
}

func (i idsIn) core() core.IDs {
	out := core.IDs{Tethr: tethrID(i.Trakt, catalogueOffset), Slug: i.Slug, IMDB: i.IMDB, TMDB: i.TMDB, TVDB: i.TVDB}
	if out.Tethr == 0 {
		out.Trakt = i.Trakt
	}
	return out
}

func (i *idsIn) UnmarshalJSON(b []byte) error {
	var raw struct {
		Trakt json.RawMessage `json:"trakt"`
		Slug  string          `json:"slug"`
		IMDB  string          `json:"imdb"`
		TMDB  json.RawMessage `json:"tmdb"`
		TVDB  json.RawMessage `json:"tvdb"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	*i = idsIn{Trakt: number(raw.Trakt), Slug: raw.Slug, IMDB: raw.IMDB, TMDB: number(raw.TMDB), TVDB: number(raw.TVDB)}
	return nil
}

// Some clients send numeric IDs as strings.
func number(raw json.RawMessage) int64 {
	var n int64
	if json.Unmarshal(raw, &n) == nil {
		return n
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		n, _ = strconv.ParseInt(s, 10, 64)
	}
	return n
}

type episodeIn struct {
	Season    *int32  `json:"season"`
	Number    *int32  `json:"number"`
	IDs       idsIn   `json:"ids"`
	WatchedAt *string `json:"watched_at"`
	ListedAt  *string `json:"listed_at"`
	RatedAt   *string `json:"rated_at"`
	Rating    *int16  `json:"rating"`
	Notes     string  `json:"notes"`
}

type seasonIn struct {
	Number    *int32      `json:"number"`
	IDs       idsIn       `json:"ids"`
	Episodes  []episodeIn `json:"episodes"`
	WatchedAt *string     `json:"watched_at"`
	ListedAt  *string     `json:"listed_at"`
	RatedAt   *string     `json:"rated_at"`
	Rating    *int16      `json:"rating"`
	Notes     string      `json:"notes"`
}

type mediaIn struct {
	IDs       idsIn      `json:"ids"`
	Seasons   []seasonIn `json:"seasons"`
	WatchedAt *string    `json:"watched_at"`
	ListedAt  *string    `json:"listed_at"`
	RatedAt   *string    `json:"rated_at"`
	Rating    *int16     `json:"rating"`
	Notes     string     `json:"notes"`
}

type syncIn struct {
	Movies   []mediaIn   `json:"movies"`
	Shows    []mediaIn   `json:"shows"`
	Seasons  []seasonIn  `json:"seasons"`
	Episodes []episodeIn `json:"episodes"`
	IDs      []int64     `json:"ids"`
}

type entryIn struct {
	ref       core.Ref
	watchedAt *string
	listedAt  *string
	notes     string
	ratedAt   *string
	rating    *int16
}

func (in syncIn) entries(profileID int64) []entryIn {
	var out []entryIn
	for _, m := range in.Movies {
		out = append(out, entryIn{core.Ref{Kind: "movie", IDs: m.IDs.core()}, m.WatchedAt, m.ListedAt, m.Notes, m.RatedAt, m.Rating})
	}
	for _, m := range in.Shows {
		show := m.IDs.core()
		if len(m.Seasons) == 0 {
			out = append(out, entryIn{core.Ref{Kind: "show", IDs: show}, m.WatchedAt, m.ListedAt, m.Notes, m.RatedAt, m.Rating})
		}
		for _, s := range m.Seasons {
			if len(s.Episodes) == 0 {
				out = append(out, entryIn{core.Ref{Kind: "season", Show: show, Season: s.Number}, first(s.WatchedAt, m.WatchedAt), first(s.ListedAt, m.ListedAt), s.Notes, first(s.RatedAt, m.RatedAt), s.Rating})
			}
			for _, e := range s.Episodes {
				ref := core.Ref{Kind: "episode", IDs: e.IDs.core(), Show: show, Season: s.Number, Number: e.Number, OrderedFor: profileID}
				out = append(out, entryIn{ref, first(e.WatchedAt, s.WatchedAt, m.WatchedAt), first(e.ListedAt, s.ListedAt, m.ListedAt), e.Notes, first(e.RatedAt, s.RatedAt, m.RatedAt), e.Rating})
			}
		}
	}
	for _, s := range in.Seasons {
		out = append(out, entryIn{core.Ref{Kind: "season", IDs: s.IDs.core()}, s.WatchedAt, s.ListedAt, s.Notes, s.RatedAt, s.Rating})
	}
	for _, e := range in.Episodes {
		out = append(out, entryIn{core.Ref{Kind: "episode", IDs: e.IDs.core()}, e.WatchedAt, e.ListedAt, e.Notes, e.RatedAt, e.Rating})
	}
	return out
}

func first(times ...*string) *string {
	for _, t := range times {
		if t != nil {
			return t
		}
	}
	return nil
}

func parseTime(s *string) *time.Time {
	if s == nil || *s == "released" {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, *s)
	if err != nil {
		return nil
	}
	return &t
}

type counts struct {
	Movies   int `json:"movies"`
	Shows    int `json:"shows"`
	Seasons  int `json:"seasons"`
	Episodes int `json:"episodes"`
}

func countsJSON(c core.Counts) counts {
	return counts{Movies: c.Movies, Shows: c.Shows, Seasons: c.Seasons, Episodes: c.Episodes}
}

type notFoundIDs struct {
	IDs idsOut `json:"ids"`
}

type idsOut struct {
	Trakt int64  `json:"trakt,omitempty"`
	Slug  string `json:"slug,omitempty"`
	IMDB  string `json:"imdb,omitempty"`
	TMDB  int64  `json:"tmdb,omitempty"`
	TVDB  int64  `json:"tvdb,omitempty"`
}

func echo(i core.IDs) notFoundIDs {
	trakt := i.Trakt
	if i.Tethr != 0 {
		trakt = traktID(i.Tethr, catalogueOffset)
	}
	return notFoundIDs{IDs: idsOut{Trakt: trakt, Slug: i.Slug, IMDB: i.IMDB, TMDB: i.TMDB, TVDB: i.TVDB}}
}

func notFound(refs []core.Ref, missingIDs []int64) map[string]any {
	out := map[string]any{
		"movies":   []notFoundIDs{},
		"shows":    []notFoundIDs{},
		"seasons":  []notFoundIDs{},
		"episodes": []notFoundIDs{},
		"people":   []notFoundIDs{},
	}
	add := func(key string, i core.IDs) { out[key] = append(out[key].([]notFoundIDs), echo(i)) }
	for _, r := range refs {
		switch {
		case r.Kind == "movie":
			add("movies", r.IDs)
		case r.Kind == "show":
			add("shows", r.IDs)
		case r.Show != (core.IDs{}):
			add("shows", r.Show)
		case r.Kind == "season":
			add("seasons", r.IDs)
		default:
			add("episodes", r.IDs)
		}
	}
	if missingIDs != nil {
		out["ids"] = missingIDs
	}
	return out
}
