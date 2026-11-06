package traktcompat

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/core"
)

type scrobbleIn struct {
	Progress float64    `json:"progress"`
	Movie    *mediaIn   `json:"movie"`
	Show     *mediaIn   `json:"show"`
	Episode  *episodeIn `json:"episode"`
	Duration *float64   `json:"duration"`
}

func (in scrobbleIn) ref(profileID int64) core.Ref {
	switch {
	case in.Episode != nil:
		ref := core.Ref{Kind: "episode", IDs: in.Episode.IDs.core(), Season: in.Episode.Season, Number: in.Episode.Number, OrderedFor: profileID}
		if in.Show != nil {
			ref.Show = in.Show.IDs.core()
		}
		return ref
	case in.Movie != nil:
		return core.Ref{Kind: "movie", IDs: in.Movie.IDs.core()}
	}
	return core.Ref{}
}

type scrobbler func(context.Context, *pgxpool.Pool, core.Caller, core.Scrobble) (core.ScrobbleResult, error)

func (s *Server) scrobble(step scrobbler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in scrobbleIn
		if !readJSON(w, r, &in) {
			return
		}
		result, err := step(r.Context(), s.DB, caller(r), core.Scrobble{Ref: in.ref(caller(r).ProfileID), Progress: in.Progress, Duration: in.Duration})
		var already core.AlreadyScrobbled
		switch {
		case errors.As(err, &already):
			writeJSON(w, http.StatusConflict, map[string]string{"watched_at": stamp(already.WatchedAt), "expires_at": stamp(already.ExpiresAt)})
		case errors.Is(err, core.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "Send a movie, or a show with an episode's season and number, with at least one ID.")
		case errors.Is(err, core.ErrProgressTooLow):
			writeError(w, http.StatusUnprocessableEntity, "progress_too_low", "Progress under 1% isn't saved.")
		case err != nil:
			s.fail(w, r, err)
		default:
			writeJSON(w, http.StatusCreated, itemFields(result.Item, map[string]any{
				"id":       result.ID,
				"action":   result.Action,
				"progress": result.Progress,
			}))
		}
	}
}

var activityFields = map[string][]string{
	"movies":          {"watched_at", "collected_at", "rated_at", "watchlisted_at", "favorited_at", "recommendations_at", "commented_at", "paused_at", "hidden_at"},
	"episodes":        {"watched_at", "collected_at", "rated_at", "watchlisted_at", "commented_at", "paused_at"},
	"shows":           {"rated_at", "watchlisted_at", "favorited_at", "recommendations_at", "commented_at", "hidden_at", "dropped_at"},
	"seasons":         {"rated_at", "watchlisted_at", "commented_at", "hidden_at"},
	"comments":        {"liked_at", "blocked_at", "reacted_at"},
	"lists":           {"liked_at", "updated_at", "commented_at", "reacted_at"},
	"watchlist":       {"updated_at"},
	"favorites":       {"updated_at"},
	"recommendations": {"updated_at"},
	"collaborations":  {"updated_at"},
	"account":         {"settings_at", "followed_at", "following_at", "pending_at", "requested_at"},
	"saved_filters":   {"updated_at"},
	"notes":           {"updated_at"},
}

func (s *Server) lastActivities(w http.ResponseWriter, r *http.Request) {
	activity, err := core.LastActivities(r.Context(), s.DB, caller(r).ProfileID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	latest := activity.Since
	doc := map[string]any{}
	for group, fields := range activityFields {
		section := map[string]string{}
		for _, f := range fields {
			at, ok := activity.Marks[group+"."+f]
			if !ok {
				at = activity.Since
			}
			if at.After(latest) {
				latest = at
			}
			section[f] = stamp(at)
		}
		doc[group] = section
	}
	doc["all"] = stamp(latest)
	etag := `"` + strconv.FormatInt(latest.UnixMicro(), 36) + `"`
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

func singular(kind string) string {
	switch kind {
	case "movies":
		return "movie"
	case "shows":
		return "show"
	case "seasons":
		return "season"
	case "episodes":
		return "episode"
	}
	return ""
}

func dateRange(r *http.Request) core.Range {
	var rng core.Range
	for key, target := range map[string]**time.Time{"start_at": &rng.Start, "end_at": &rng.End} {
		if v := r.URL.Query().Get(key); v != "" {
			*target = parseTime(&v)
		}
	}
	return rng
}

func (s *Server) playback(w http.ResponseWriter, r *http.Request) {
	kind := singular(r.PathValue("type"))
	if r.PathValue("type") != "" && kind != "movie" && kind != "episode" {
		writeError(w, http.StatusNotFound, "not_found", "Playback is for movies or episodes.")
		return
	}
	page := core.Page{Page: 1, Limit: 1000}
	if r.URL.Query().Has("page") || r.URL.Query().Has("limit") {
		page = paging(r, 10)
	}
	entries, err := core.Playback(r.Context(), s.DB, caller(r).ProfileID, kind, dateRange(r), page)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		out = append(out, itemFields(e.Item, map[string]any{
			"id":        e.ID,
			"progress":  e.Progress,
			"paused_at": stamp(e.At),
			"type":      e.Item.Kind(),
		}))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) removePlayback(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	err := core.RemovePlayback(r.Context(), s.DB, caller(r).ProfileID, id)
	if errors.Is(err, core.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "There's no playback with that ID.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) history(w http.ResponseWriter, r *http.Request) {
	filter := core.HistoryFilter{Kind: singular(r.PathValue("type")), Range: dateRange(r)}
	if (r.PathValue("type") != "" && filter.Kind == "") || filter.Kind == "season" {
		writeError(w, http.StatusNotFound, "not_found", "History types are movies, shows and episodes.")
		return
	}
	if id := r.PathValue("id"); id != "" {
		filter.ID, _ = strconv.ParseInt(id, 10, 64)
	}
	page := paging(r, 10)
	entries, total, err := core.History(r.Context(), s.DB, caller(r).ProfileID, filter, page)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		out = append(out, itemFields(e.Item, map[string]any{
			"id":         e.ID,
			"watched_at": stamp(e.At),
			"action":     e.Action,
			"type":       e.Item.Kind(),
		}))
	}
	writePage(w, page.Page, page.Limit, total)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) addHistory(w http.ResponseWriter, r *http.Request) {
	var in syncIn
	if !readJSON(w, r, &in) {
		return
	}
	var watches []core.Watch
	for _, e := range in.entries(caller(r).ProfileID) {
		at := time.Now()
		if t := parseTime(e.watchedAt); t != nil {
			at = *t
		}
		watches = append(watches, core.Watch{Ref: e.ref, At: at})
	}
	result, err := core.AddHistory(r.Context(), s.DB, caller(r), watches)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"added":     countsJSON(result.Added),
		"not_found": notFound(result.NotFound, nil),
	})
}

func (s *Server) removeHistory(w http.ResponseWriter, r *http.Request) {
	var in syncIn
	if !readJSON(w, r, &in) {
		return
	}
	var refs []core.Ref
	for _, e := range in.entries(caller(r).ProfileID) {
		refs = append(refs, e.ref)
	}
	result, err := core.RemoveHistory(r.Context(), s.DB, caller(r).ProfileID, in.IDs, refs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	missing := result.NotIDs
	if missing == nil {
		missing = []int64{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"deleted":   countsJSON(result.Deleted),
		"not_found": notFound(result.NotFound, missing),
	})
}

func (s *Server) watchlist(w http.ResponseWriter, r *http.Request) {
	kind := singular(r.PathValue("type"))
	if t := r.PathValue("type"); t != "" && t != "all" && kind == "" {
		writeError(w, http.StatusNotFound, "not_found", "Watchlist types are movies, shows, seasons and episodes.")
		return
	}
	entries, err := core.Watchlist(r.Context(), s.DB, caller(r).ProfileID, kind)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	sortBy, sortHow := r.PathValue("sort_by"), r.PathValue("sort_how")
	if sortBy != "added" {
		sortBy = "rank"
	}
	if sortHow != "desc" {
		sortHow = "asc"
	}
	if sortBy == "added" {
		slices.SortStableFunc(entries, func(a, b core.Entry) int { return a.At.Compare(b.At) })
	}
	if sortHow == "desc" {
		slices.Reverse(entries)
	}
	total := int64(len(entries))
	if r.URL.Query().Has("page") || r.URL.Query().Has("limit") {
		page := paging(r, 10)
		start := min((page.Page-1)*page.Limit, len(entries))
		entries = entries[start:min(start+page.Limit, len(entries))]
		writePage(w, page.Page, page.Limit, total)
	}
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		out = append(out, itemFields(e.Item, map[string]any{
			"rank":      e.Rank,
			"id":        e.ID,
			"listed_at": stamp(e.At),
			"notes":     e.Notes,
			"type":      e.Item.Kind(),
		}))
	}
	w.Header().Set("X-Sort-By", sortBy)
	w.Header().Set("X-Sort-How", sortHow)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) addToWatchlist(w http.ResponseWriter, r *http.Request) {
	var in syncIn
	if !readJSON(w, r, &in) {
		return
	}
	var listings []core.Listing
	for _, e := range in.entries(caller(r).ProfileID) {
		listings = append(listings, core.Listing{Ref: e.ref, At: parseTime(e.listedAt), Notes: strings.TrimSpace(e.notes)})
	}
	result, err := core.AddToWatchlist(r.Context(), s.DB, caller(r).ProfileID, listings)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"added":     countsJSON(result.Added),
		"existing":  countsJSON(result.Existing),
		"not_found": notFound(result.NotFound, nil),
		"list":      map[string]any{"updated_at": stamp(result.ListAt), "item_count": result.Listed},
	})
}

func (s *Server) removeFromWatchlist(w http.ResponseWriter, r *http.Request) {
	var in syncIn
	if !readJSON(w, r, &in) {
		return
	}
	var refs []core.Ref
	for _, e := range in.entries(caller(r).ProfileID) {
		refs = append(refs, e.ref)
	}
	result, err := core.RemoveFromWatchlist(r.Context(), s.DB, caller(r).ProfileID, refs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"deleted":   countsJSON(result.Deleted),
		"not_found": notFound(result.NotFound, nil),
		"list":      map[string]any{"updated_at": stamp(result.ListAt), "item_count": result.Listed},
	})
}

func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	p, err := core.Profile(r.Context(), s.DB, caller(r).ProfileID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user": map[string]any{
			"username":  p.Username,
			"email":     "",
			"private":   p.Private,
			"name":      p.Name,
			"vip":       false,
			"vip_ep":    false,
			"director":  false,
			"vip_og":    false,
			"vip_years": 0,
			"ids":       map[string]string{"slug": strings.ToLower(p.Username), "uuid": p.Uuid.String()},
			"joined_at": stamp(p.CreatedAt),
			"location":  nil,
			"about":     nil,
			"gender":    nil,
			"age":       nil,
			"images":    map[string]any{"avatar": map[string]string{"full": ""}},
		},
		"permissions": map[string]bool{"commenting": false, "liking": false, "following": false},
		"account":     map[string]any{"timezone": "UTC", "date_format": "dmy", "time_24hr": true, "cover_image": nil},
		"connections": map[string]bool{
			"facebook": false, "twitter": false, "mastodon": false, "google": false, "tumblr": false,
			"medium": false, "slack": false, "apple": false, "dropbox": false, "microsoft": false,
		},
		"sharing_text": map[string]string{"watching": "", "watched": "", "rated": ""},
		"limits": map[string]any{
			"list":      map[string]int{"count": 100, "item_count": 1000},
			"watchlist": map[string]int{"item_count": 10000},
			"favorites": map[string]int{"item_count": 0},
		},
	})
}

func (s *Server) lookUp(w http.ResponseWriter, r *http.Request) {
	value := r.PathValue("id")
	var ids core.IDs
	switch r.PathValue("id_type") {
	case "trakt":
		ids.Trakt, _ = strconv.ParseInt(value, 10, 64)
	case "imdb":
		ids.IMDB = value
	case "tmdb":
		ids.TMDB, _ = strconv.ParseInt(value, 10, 64)
	case "tvdb":
		ids.TVDB, _ = strconv.ParseInt(value, 10, 64)
	default:
		writeError(w, http.StatusNotFound, "not_found", "Look up by trakt, imdb, tmdb or tvdb.")
		return
	}
	kind := r.URL.Query().Get("type")
	if kind != "" && kind != "movie" && kind != "show" && kind != "episode" {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	items, err := core.LookUp(r.Context(), s.DB, kind, ids, true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, i := range items {
		out = append(out, itemFields(i, map[string]any{"type": i.Kind(), "score": 1000}))
	}
	writePage(w, 1, 10, int64(len(out)))
	writeJSON(w, http.StatusOK, out)
}
