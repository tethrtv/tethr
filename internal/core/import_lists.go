package core

import (
	"cmp"
	"context"
	"errors"
	"html"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/tethrtv/tethr/internal/store"
)

type ImportList struct {
	Name        string
	Description string
	Privacy     string
	SortBy      string
	SortHow     string
	Snapshot    bool
	Items       []ImportItem
}

type ImportedList struct {
	ImportCount
	Name     string `json:"name"`
	Privacy  string `json:"privacy"`
	Snapshot bool   `json:"snapshot"`
	Full     int    `json:"full"`
	Existing bool   `json:"existing"`
	Chosen   bool   `json:"chosen"`
	Created  bool   `json:"created"`
}

var (
	lineTags   = regexp.MustCompile(`(?i)<br\s*/?>|</p>`)
	tags       = regexp.MustCompile(`<[^>]*>`)
	blankLines = regexp.MustCompile(`\n{3,}`)
	markdown   = []struct {
		re   *regexp.Regexp
		with string
	}{
		{regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`), "$1"},
		{regexp.MustCompile(`\[([^\]]+)\]\([^)\s]*\)`), "$1"},
		{regexp.MustCompile(`\*\*(\S(?:[^*]*\S)?)\*\*`), "$1"},
		{regexp.MustCompile(`__(\S(?:[^_]*\S)?)__`), "$1"},
		{regexp.MustCompile(`(^|[\s(])\*(\S(?:[^*]*\S)?)\*`), "$1$2"},
		{regexp.MustCompile(`(^|[\s(])_(\S(?:[^_]*\S)?)_`), "$1$2"},
		{regexp.MustCompile("`([^`]+)`"), "$1"},
		{regexp.MustCompile(`(?m)^#{1,6}\s+`), ""},
		{regexp.MustCompile(`(?m)^>\s?`), ""},
	}
)

func plainText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = lineTags.ReplaceAllString(s, "\n")
	s = tags.ReplaceAllString(s, "")
	for _, m := range markdown {
		s = m.re.ReplaceAllString(s, m.with)
	}
	s = html.UnescapeString(s)
	return strings.TrimSpace(blankLines.ReplaceAllString(s, "\n\n"))
}

func cut(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return strings.TrimSpace(string([]rune(s)[:n-1])) + "…"
}

func cleanList(l ImportList) ImportList {
	l.Name = cmp.Or(cut(l.Name, 100), "Untitled list")
	l.Description = cut(plainText(l.Description), 1000)
	if l.Privacy != "public" {
		l.Privacy = "private"
	}
	l.SortBy = map[string]string{"rank": "rank", "added": "added", "title": "title", "released": "year"}[l.SortBy]
	if l.SortBy == "" || l.SortHow != "desc" {
		l.SortHow = "asc"
	}
	l.SortBy = cmp.Or(l.SortBy, "rank")
	items := make([]ImportItem, len(l.Items))
	for i, item := range l.Items {
		item.Notes = cut(item.Notes, 500)
		items[i] = item
	}
	l.Items = items
	return l
}

func findLists(ctx context.Context, q *store.Queries, profileID int64, summary *ImportSummary) error {
	if len(summary.Lists) == 0 {
		return nil
	}
	n, err := q.CountLists(ctx, profileID)
	if err != nil {
		return err
	}
	summary.ListRoom = max(maxLists-int(n), 0)
	p, err := q.ProfileByID(ctx, profileID)
	if err != nil {
		return err
	}
	if p.Kind == "kids" || p.Kind == "guest" {
		for i := range summary.Lists {
			summary.Lists[i].Privacy = "private"
		}
	}
	return markExisting(ctx, q, profileID, summary.Lists, false)
}

func markExisting(ctx context.Context, q *store.Queries, profileID int64, lists []ImportedList, earlier bool) error {
	made := map[string]bool{}
	for i := range lists {
		l := &lists[i]
		name := strings.ToLower(l.Name)
		l.Existing = made[name]
		if !l.Existing {
			_, err := q.ListNamed(ctx, store.ListNamedParams{ProfileID: profileID, Name: l.Name})
			switch {
			case errors.Is(err, pgx.ErrNoRows):
			case err != nil:
				return err
			default:
				l.Existing = true
			}
		}
		if earlier && l.Chosen && l.Unmatched < l.Total {
			made[name] = true
		}
	}
	return nil
}

func chooseLists(ctx context.Context, q *store.Queries, profileID int64, summary *ImportSummary, chosen []int) error {
	for _, i := range chosen {
		if i < 0 || i >= len(summary.Lists) {
			return ErrImportLists
		}
	}
	for i := range summary.Lists {
		summary.Lists[i].Chosen = chosen == nil || slices.Contains(chosen, i)
	}
	if err := markExisting(ctx, q, profileID, summary.Lists, true); err != nil {
		return err
	}
	added := 0
	summary.ListEntries = ImportCount{}
	for _, l := range summary.Lists {
		if !l.Chosen {
			continue
		}
		summary.ListEntries.Total += l.Total
		summary.ListEntries.Unmatched += l.Unmatched
		if !l.Existing && l.Unmatched < l.Total {
			added++
		}
	}
	if added == 0 {
		return nil
	}
	n, err := q.CountLists(ctx, profileID)
	if err != nil {
		return err
	}
	if int(n)+added > maxLists {
		return ErrTooManyLists
	}
	return nil
}

func (im *importer) lists(lists []ImportList) error {
	changed := false
	for i, l := range lists {
		if i >= len(im.summary.Lists) || !im.summary.Lists[i].Chosen {
			continue
		}
		s := &im.summary.Lists[i]
		type found struct {
			item ImportItem
			r    resolved
		}
		var items []found
		for _, item := range l.Items {
			var r *resolved
			if slices.Contains(listKinds, item.Ref.Kind) {
				var err error
				if r, err = im.resolve(item.Ref); err != nil {
					return err
				}
			}
			if r == nil {
				s.Unmatched++
				im.summary.unmatched(&im.summary.ListEntries, item.Label)
				continue
			}
			items = append(items, found{item, *r})
		}
		if len(items) == 0 {
			continue
		}

		list, err := im.q.ListNamed(im.ctx, store.ListNamedParams{ProfileID: im.imp.ProfileID, Name: l.Name})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			list.ID, err = createList(im.ctx, im.q, im.imp.ProfileID, store.List{
				Name:        l.Name,
				Description: optional(l.Description),
				Privacy:     cmp.Or(s.Privacy, "private"),
				SortBy:      cmp.Or(l.SortBy, "rank"),
				SortHow:     cmp.Or(l.SortHow, "asc"),
			})
			if errors.Is(err, ErrTooManyLists) {
				s.Full += len(items)
				continue
			}
			if err != nil {
				return err
			}
			if err := im.q.SetListImport(im.ctx, store.SetListImportParams{ID: list.ID, ImportID: &im.imp.ID}); err != nil {
				return err
			}
			s.Created, changed = true, true
		case err != nil:
			return err
		}

		count, err := im.q.CountListItems(im.ctx, list.ID)
		if err != nil {
			return err
		}
		for _, f := range items {
			if count >= maxListItems {
				s.Full++
				continue
			}
			at := time.Now()
			if f.item.At != nil {
				at = *f.item.At
			}
			n, err := im.q.ImportListItem(im.ctx, store.ImportListItemParams{
				ListID:    list.ID,
				TitleID:   f.r.title.ID,
				SeasonID:  f.r.seasonID(),
				EpisodeID: f.r.episodeID(),
				Notes:     optional(f.item.Notes),
				ListedAt:  at,
				ImportID:  &im.imp.ID,
			})
			if err != nil {
				return err
			}
			if n == 0 {
				s.Skipped++
				im.summary.ListEntries.Skipped++
				continue
			}
			count++
			s.Added++
			im.summary.ListEntries.Added++
		}
		if s.Added > 0 && !s.Created {
			if _, err := im.q.TouchList(im.ctx, list.ID); err != nil {
				return err
			}
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return markLists(im.ctx, im.q, im.imp.ProfileID)
}
