package traktcompat

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/tethrtv/tethr/internal/core"
)

func (s *Server) withOptionalCaller(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			next(w, r)
			return
		}
		s.withCaller(next)(w, r)
	}
}

func (s *Server) userLists(w http.ResponseWriter, r *http.Request) {
	lists, ok := s.profileLists(w, r)
	if !ok {
		return
	}
	out := make([]map[string]any, 0, len(lists))
	for _, l := range lists {
		out = append(out, s.listJSON(l))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) userList(w http.ResponseWriter, r *http.Request) {
	l, ok := s.findList(w, r)
	if ok {
		writeJSON(w, http.StatusOK, s.listJSON(l))
	}
}

func (s *Server) listItems(w http.ResponseWriter, r *http.Request) {
	kind := singular(r.PathValue("type"))
	if t := r.PathValue("type"); t != "" && t != "all" && kind == "" {
		writeError(w, http.StatusNotFound, "not_found", "List item types are movies, shows, seasons and episodes.")
		return
	}
	l, ok := s.findList(w, r)
	if !ok {
		return
	}
	query := core.ListQuery{Kind: kind}
	if r.URL.Query().Has("page") || r.URL.Query().Has("limit") {
		query.Page = paging(r, 10)
	}
	_, entries, total, err := core.ListEntries(r.Context(), s.DB, caller(r).ProfileID, l.ID, query)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if query.Page.Limit > 0 {
		writePage(w, query.Page.Page, query.Page.Limit, int64(total))
	}
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		out = append(out, itemFields(e.Item, map[string]any{
			"rank":      e.Rank,
			"id":        traktID(e.ID, entryOffset),
			"listed_at": stamp(e.At),
			"notes":     e.Notes,
			"type":      e.Item.Kind(),
		}))
	}
	w.Header().Set("X-Sort-By", traktSort(l.SortBy))
	w.Header().Set("X-Sort-How", l.SortHow)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) profileLists(w http.ResponseWriter, r *http.Request) ([]core.List, bool) {
	viewer := caller(r).ProfileID
	username := r.PathValue("id")
	if username == "me" {
		if viewer == 0 {
			writeError(w, http.StatusUnauthorized, "invalid_token", "Sign in first.")
			return nil, false
		}
		p, err := core.Profile(r.Context(), s.DB, viewer)
		if err != nil {
			s.fail(w, r, err)
			return nil, false
		}
		username = p.Username
	}
	lists, err := core.ProfileLists(r.Context(), s.DB, viewer, username)
	if errors.Is(err, core.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "There’s no one with that username.")
		return nil, false
	}
	if err != nil {
		s.fail(w, r, err)
		return nil, false
	}
	return lists, true
}

func (s *Server) findList(w http.ResponseWriter, r *http.Request) (core.List, bool) {
	if r.PathValue("list_id") == "" {
		return s.listByID(w, r, r.PathValue("id"))
	}
	lists, ok := s.profileLists(w, r)
	if !ok {
		return core.List{}, false
	}
	key := strings.ToLower(r.PathValue("list_id"))
	for _, l := range lists {
		if strconv.FormatInt(traktID(l.ID, listOffset), 10) == key || l.Slug == key {
			return l, true
		}
	}
	writeError(w, http.StatusNotFound, "not_found", "There’s no list with that ID, or it isn’t shared with you.")
	return core.List{}, false
}

func (s *Server) listByID(w http.ResponseWriter, r *http.Request, id string) (core.List, bool) {
	n, _ := strconv.ParseInt(id, 10, 64)
	listID := tethrID(n, listOffset)
	if listID == 0 {
		writeError(w, http.StatusNotFound, "not_found", "There’s no list with that ID, or it isn’t shared with you.")
		return core.List{}, false
	}
	l, err := core.GetList(r.Context(), s.DB, caller(r).ProfileID, listID)
	if errors.Is(err, core.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "There’s no list with that ID, or it isn’t shared with you.")
		return core.List{}, false
	}
	if err != nil {
		s.fail(w, r, err)
		return core.List{}, false
	}
	return l, true
}

func (s *Server) listJSON(l core.List) map[string]any {
	privacy := l.Privacy
	if privacy == "household" {
		privacy = "friends"
	}
	return map[string]any{
		"name":            l.Name,
		"description":     l.Description,
		"privacy":         privacy,
		"share_link":      s.AppURL + l.Path(),
		"type":            "personal",
		"display_numbers": l.SortBy == "rank",
		"allow_comments":  false,
		"sort_by":         traktSort(l.SortBy),
		"sort_how":        l.SortHow,
		"created_at":      stamp(l.CreatedAt),
		"updated_at":      stamp(l.UpdatedAt),
		"item_count":      l.ItemCount,
		"comment_count":   0,
		"likes":           0,
		"ids":             map[string]any{"trakt": traktID(l.ID, listOffset), "slug": l.Slug},
		"user": map[string]any{
			"username": l.Owner.Username,
			"private":  l.Owner.Private,
			"deleted":  false,
			"name":     l.Owner.Name,
			"vip":      false,
			"vip_ep":   false,
			"director": false,
			"ids":      map[string]any{"trakt": l.Owner.ID, "slug": strings.ToLower(l.Owner.Username)},
		},
	}
}

func traktSort(by string) string {
	if by == "year" {
		return "released"
	}
	return by
}
