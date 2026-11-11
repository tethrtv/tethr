package traktcompat

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tethrtv/tethr/internal/core"
)

func stampOrNil(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := stamp(*t)
	return &s
}

func episodeOrNil(e *core.OrderedEpisode) any {
	if e == nil {
		return nil
	}
	return episode{Season: e.Season, Number: e.Number, IDs: ids{Trakt: traktID(e.ID, catalogueOffset), TMDB: e.TmdbEpisodeID}}
}

func (s *Server) ratings(w http.ResponseWriter, r *http.Request) {
	kind := singular(r.PathValue("type"))
	if t := r.PathValue("type"); t != "" && t != "all" && kind == "" {
		writeError(w, http.StatusNotFound, "not_found", "Rating types are movies, shows, seasons and episodes.")
		return
	}
	var ratings []int16
	if v := r.PathValue("rating"); v != "" {
		for _, part := range strings.Split(v, ",") {
			n, err := strconv.Atoi(part)
			if err == nil && n >= 1 && n <= 10 {
				ratings = append(ratings, int16(n))
			}
		}
	}
	entries, err := core.Ratings(r.Context(), s.DB, caller(r).ProfileID, kind, ratings)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if r.URL.Query().Has("page") || r.URL.Query().Has("limit") {
		page := paging(r, 10)
		start := min((page.Page-1)*page.Limit, len(entries))
		writePage(w, page.Page, page.Limit, int64(len(entries)))
		entries = entries[start:min(start+page.Limit, len(entries))]
	}
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		out = append(out, itemFields(e.Item, map[string]any{
			"rated_at": stamp(e.At),
			"rating":   e.Rating,
			"type":     e.Item.Kind(),
		}))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) rate(w http.ResponseWriter, r *http.Request) {
	var in syncIn
	if !readJSON(w, r, &in) {
		return
	}
	var ratings []core.Rating
	for _, e := range in.entries(caller(r).ProfileID) {
		var rating int16
		if e.rating != nil {
			rating = *e.rating
		}
		ratings = append(ratings, core.Rating{Ref: e.ref, Rating: rating, At: parseTime(e.ratedAt)})
	}
	result, err := core.Rate(r.Context(), s.DB, caller(r).ProfileID, ratings)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"added":     countsJSON(result.Added),
		"not_found": notFound(result.NotFound, nil),
	})
}

func (s *Server) unrate(w http.ResponseWriter, r *http.Request) {
	var in syncIn
	if !readJSON(w, r, &in) {
		return
	}
	var refs []core.Ref
	for _, e := range in.entries(caller(r).ProfileID) {
		refs = append(refs, e.ref)
	}
	result, err := core.Unrate(r.Context(), s.DB, caller(r).ProfileID, refs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"deleted":   countsJSON(result.Deleted),
		"not_found": notFound(result.NotFound, nil),
	})
}

func (s *Server) watched(w http.ResponseWriter, r *http.Request) {
	kind := singular(r.PathValue("type"))
	if kind != "movie" && kind != "show" {
		writeError(w, http.StatusNotFound, "not_found", "Watched types are movies and shows.")
		return
	}
	titles, err := core.Watched(r.Context(), s.DB, caller(r).ProfileID, kind)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	noSeasons := strings.Contains(r.URL.Query().Get("extended"), "noseasons")
	out := make([]map[string]any, 0, len(titles))
	for _, t := range titles {
		entry := itemFields(t.Item, map[string]any{
			"plays":           t.Plays.Plays,
			"last_watched_at": stamp(t.LastWatchedAt),
			"last_updated_at": stamp(t.LastWatchedAt),
		})
		if kind == "show" {
			entry["reset_at"] = nil
			if !noSeasons {
				entry["seasons"] = watchedSeasons(t.Episodes)
			}
		}
		out = append(out, entry)
	}
	writeJSON(w, http.StatusOK, out)
}

type watchedEpisode struct {
	Number        int32  `json:"number"`
	Plays         int64  `json:"plays"`
	LastWatchedAt string `json:"last_watched_at"`
}

type watchedSeason struct {
	Number   int32            `json:"number"`
	Episodes []watchedEpisode `json:"episodes"`
}

func watchedSeasons(episodes []core.WatchedEpisode) []watchedSeason {
	out := []watchedSeason{}
	for _, e := range episodes {
		if len(out) == 0 || out[len(out)-1].Number != e.Episode.Season {
			out = append(out, watchedSeason{Number: e.Episode.Season})
		}
		s := &out[len(out)-1]
		s.Episodes = append(s.Episodes, watchedEpisode{Number: e.Episode.Number, Plays: e.Plays.Plays, LastWatchedAt: stamp(e.LastWatchedAt)})
	}
	return out
}

func showRef(id string) core.Ref {
	ref := core.Ref{Kind: "show"}
	if n, err := strconv.ParseInt(id, 10, 64); err == nil {
		ref.IDs.Tethr = tethrID(n, catalogueOffset)
	} else {
		ref.IDs.Slug = id
	}
	return ref
}

func (s *Server) showProgress(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p, err := core.Progress(r.Context(), s.DB, caller(r).ProfileID, showRef(r.PathValue("id")), q.Get("specials") == "true")
	if errors.Is(err, core.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "There's no show with that ID.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	type episodeProgress struct {
		Number        int32   `json:"number"`
		Completed     bool    `json:"completed"`
		LastWatchedAt *string `json:"last_watched_at"`
	}
	type seasonProgress struct {
		Number    int32             `json:"number"`
		Title     *string           `json:"title"`
		Aired     int               `json:"aired"`
		Completed int               `json:"completed"`
		Episodes  []episodeProgress `json:"episodes"`
	}
	seasons := make([]seasonProgress, 0, len(p.Seasons))
	for _, season := range p.Seasons {
		sp := seasonProgress{Number: season.Number, Aired: season.Aired, Completed: season.Completed, Episodes: []episodeProgress{}}
		for _, e := range season.Episodes {
			sp.Episodes = append(sp.Episodes, episodeProgress{Number: e.Episode.Number, Completed: e.LastWatchedAt != nil, LastWatchedAt: stampOrNil(e.LastWatchedAt)})
		}
		seasons = append(seasons, sp)
	}
	hidden := make([]season, 0, len(p.HiddenSeasons))
	for _, n := range p.HiddenSeasons {
		hidden = append(hidden, season{Number: n})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"aired":           p.Aired,
		"completed":       p.Completed,
		"last_watched_at": stampOrNil(p.LastWatchedAt),
		"reset_at":        stampOrNil(p.ResetAt),
		"seasons":         seasons,
		"hidden_seasons":  hidden,
		"next_episode":    episodeOrNil(p.Next),
		"last_episode":    episodeOrNil(p.Last),
	})
}

func (s *Server) upNext(w http.ResponseWriter, r *http.Request) {
	entries, err := core.UpNext(r.Context(), s.DB, caller(r).ProfileID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	page := paging(r, 10)
	start := min((page.Page-1)*page.Limit, len(entries))
	writePage(w, page.Page, page.Limit, int64(len(entries)))
	out := make([]map[string]any, 0, page.Limit)
	for _, e := range entries[start:min(start+page.Limit, len(entries))] {
		out = append(out, map[string]any{
			"show": titleJSON(e.Show),
			"progress": map[string]any{
				"aired":           e.Aired,
				"completed":       e.Watched,
				"last_watched_at": stamp(e.LastWatchedAt),
				"reset_at":        stampOrNil(e.RewatchFrom),
				"next_episode":    episodeOrNil(&e.Next),
				"last_episode":    episodeOrNil(e.Last),
			},
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) hidden(section string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kind := r.URL.Query().Get("type")
		entries, err := core.Hidden(r.Context(), s.DB, caller(r).ProfileID, section, kind)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		page := paging(r, 10)
		start := min((page.Page-1)*page.Limit, len(entries))
		writePage(w, page.Page, page.Limit, int64(len(entries)))
		out := make([]map[string]any, 0, page.Limit)
		for _, e := range entries[start:min(start+page.Limit, len(entries))] {
			out = append(out, itemFields(e.Item, map[string]any{
				"hidden_at": stamp(e.At),
				"type":      e.Item.Kind(),
			}))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func hiddenCounts(c core.Counts) map[string]int {
	// Trakt's spec asks for season when adding and episodes when removing, so send every key.
	return map[string]int{"movies": c.Movies, "shows": c.Shows, "seasons": c.Seasons, "season": c.Seasons, "episodes": 0, "users": 0}
}

func (s *Server) hide(section string, remove bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in syncIn
		if !readJSON(w, r, &in) {
			return
		}
		var refs []core.Ref
		for _, e := range in.entries(caller(r).ProfileID) {
			refs = append(refs, e.ref)
		}
		change, key := core.Hide, "added"
		if remove {
			change, key = core.Unhide, "deleted"
		}
		result, err := change(r.Context(), s.DB, caller(r).ProfileID, section, refs)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		counts := result.Added
		if remove {
			counts = result.Deleted
		}
		missing := notFound(result.NotFound, nil)
		missing["users"] = []notFoundIDs{}
		writeJSON(w, http.StatusOK, map[string]any{key: hiddenCounts(counts), "not_found": missing})
	}
}
