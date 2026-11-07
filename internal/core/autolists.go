package core

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

var AutoListKinds = []string{"stopped", "favourites", "rewatch", "household"}

const (
	stoppedAfter = 90 * 24 * time.Hour
	rewatchAfter = 365 * 24 * time.Hour
)

type AutoList struct {
	Kind  string
	Count int
	Cover []Item
}

type AutoEntry struct {
	ListEntry
	Rating *int16
}

type autoItem struct {
	titleID int64
	at      time.Time
}

func AutoLists(ctx context.Context, db *pgxpool.Pool, profileID int64) ([]AutoList, error) {
	q := store.New(db)
	found := map[string][]autoItem{}
	var titleIDs []int64
	for _, kind := range AutoListKinds {
		items, ok, err := autoItems(ctx, q, profileID, kind)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		found[kind] = items
		for _, i := range items[:min(coverItems, len(items))] {
			titleIDs = append(titleIDs, i.titleID)
		}
	}
	c, err := loadCatalogue(ctx, q, profileID, titleIDs, nil)
	if err != nil {
		return nil, err
	}
	lists := make([]AutoList, 0, len(found))
	for _, kind := range AutoListKinds {
		items, ok := found[kind]
		if !ok {
			continue
		}
		l := AutoList{Kind: kind, Count: len(items), Cover: []Item{}}
		for _, i := range items[:min(coverItems, len(items))] {
			l.Cover = append(l.Cover, c.item(i.titleID, nil, nil))
		}
		lists = append(lists, l)
	}
	return lists, nil
}

func AutoListEntries(ctx context.Context, db *pgxpool.Pool, profileID int64, kind string, query ListQuery) (AutoList, []AutoEntry, int, error) {
	q := store.New(db)
	items, ok, err := autoItems(ctx, q, profileID, kind)
	if err != nil {
		return AutoList{}, nil, 0, err
	}
	if !ok {
		return AutoList{}, nil, 0, ErrNotFound
	}
	titleIDs := make([]int64, 0, len(items))
	for _, i := range items {
		titleIDs = append(titleIDs, i.titleID)
	}
	c, err := loadCatalogue(ctx, q, profileID, titleIDs, nil)
	if err != nil {
		return AutoList{}, nil, 0, err
	}
	rows := make([]store.ListItemsInRow, 0, len(items))
	for n, i := range items {
		rows = append(rows, store.ListItemsInRow{ID: int64(n + 1), TitleID: i.titleID, Rank: int32(n + 1), ListedAt: i.at, TitleKind: c.item(i.titleID, nil, nil).Title.Kind})
	}
	progress, err := itemProgress(ctx, q, profileID, rows)
	if err != nil {
		return AutoList{}, nil, 0, err
	}
	ratings, err := q.Ratings(ctx, store.RatingsParams{ProfileID: profileID, Ratings: []int16{}})
	if err != nil {
		return AutoList{}, nil, 0, err
	}
	rated := map[int64]int16{}
	for _, r := range ratings {
		if r.SeasonNumber == nil && r.EpisodeID == nil {
			rated[r.TitleID] = r.Rating
		}
	}

	list := AutoList{Kind: kind, Count: len(items), Cover: []Item{}}
	var entries []ListEntry
	for _, row := range rows {
		item := c.item(row.TitleID, nil, nil)
		if len(list.Cover) < coverItems {
			list.Cover = append(list.Cover, item)
		}
		e := ListEntry{Entry: Entry{ID: row.ID, At: row.ListedAt, Rank: row.Rank, Item: item}}
		if query.Kind != "" && item.Kind() != query.Kind {
			continue
		}
		if p, ok := progress[row.ID]; ok {
			if query.HideWatched && p.Done() {
				continue
			}
			e.Progress = &p
		}
		entries = append(entries, e)
	}
	sortEntries(entries, cmp.Or(query.SortBy, "rank"), cmp.Or(query.SortHow, "asc"))
	total := len(entries)
	if query.Page.Limit > 0 {
		start := min((max(query.Page.Page, 1)-1)*query.Page.Limit, total)
		entries = entries[start:min(start+query.Page.Limit, total)]
	}
	out := make([]AutoEntry, 0, len(entries))
	for _, e := range entries {
		a := AutoEntry{ListEntry: e}
		if r, ok := rated[e.Item.Title.ID]; ok {
			a.Rating = &r
		}
		out = append(out, a)
	}
	return list, out, total, nil
}

func CopyAutoList(ctx context.Context, db *pgxpool.Pool, profileID int64, kind, name string) (List, error) {
	q := store.New(db)
	items, ok, err := autoItems(ctx, q, profileID, kind)
	if err != nil {
		return List{}, err
	}
	if !ok {
		return List{}, ErrNotFound
	}
	var out List
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		id, err := createList(ctx, q, profileID, store.List{Name: name, Privacy: "private", SortBy: "rank", SortHow: "asc"})
		if err != nil {
			return err
		}
		for _, i := range items[:min(maxListItems, len(items))] {
			if _, err := q.AddListItem(ctx, store.AddListItemParams{ListID: id, TitleID: i.titleID}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		out, err = getList(ctx, q, profileID, id)
		return err
	})
	return out, err
}

func autoItems(ctx context.Context, q *store.Queries, profileID int64, kind string) ([]autoItem, bool, error) {
	now := time.Now()
	switch kind {
	case "stopped":
		items, err := stoppedShows(ctx, q, profileID, now)
		return items, true, err
	case "favourites":
		ratings, err := q.Ratings(ctx, store.RatingsParams{ProfileID: profileID, Ratings: []int16{9, 10}})
		if err != nil {
			return nil, false, err
		}
		slices.SortStableFunc(ratings, func(a, b store.Rating) int { return cmp.Compare(b.Rating, a.Rating) })
		var items []autoItem
		for _, r := range ratings {
			if r.SeasonNumber == nil && r.EpisodeID == nil {
				items = append(items, autoItem{titleID: r.TitleID, at: r.RatedAt})
			}
		}
		return items, true, nil
	case "rewatch":
		items, err := dueRewatch(ctx, q, profileID, now)
		return items, true, err
	case "household":
		raters, err := q.HouseholdRaters(ctx, profileID)
		if err != nil || raters < 2 {
			return nil, false, err
		}
		rows, err := q.HouseholdTopRatings(ctx, profileID)
		if err != nil {
			return nil, false, err
		}
		var items []autoItem
		count := map[int64]int{}
		for _, r := range rows {
			if count[r.TitleID] == 0 {
				items = append(items, autoItem{titleID: r.TitleID, at: r.RatedAt})
			}
			count[r.TitleID]++
		}
		slices.SortStableFunc(items, func(a, b autoItem) int { return cmp.Compare(count[b.titleID], count[a.titleID]) })
		return items, true, nil
	}
	return nil, false, nil
}

func stoppedShows(ctx context.Context, q *store.Queries, profileID int64, now time.Time) ([]autoItem, error) {
	rows, err := q.WatchedEpisodes(ctx, store.WatchedEpisodesParams{ProfileID: profileID})
	if err != nil {
		return nil, err
	}
	hiddenShows, hiddenSeasons, err := hiddenFromUpNext(ctx, q, profileID)
	if err != nil {
		return nil, err
	}
	watched := map[int64]map[int64]time.Time{}
	var showIDs []int64
	for _, r := range rows {
		if watched[r.TitleID] == nil {
			watched[r.TitleID] = map[int64]time.Time{}
			showIDs = append(showIDs, r.TitleID)
		}
		watched[r.TitleID][*r.EpisodeID] = r.LastWatchedAt
	}
	episodes, err := showEpisodes(ctx, q, profileID, showIDs)
	if err != nil {
		return nil, err
	}
	var items []autoItem
	for _, id := range showIDs {
		p := showProgress(episodes[id], watched[id], hiddenSeasons[id], false, now)
		if p.Next == nil || p.LastWatchedAt == nil {
			continue
		}
		if hiddenShows[id] || now.Sub(*p.LastWatchedAt) >= stoppedAfter {
			items = append(items, autoItem{titleID: id, at: *p.LastWatchedAt})
		}
	}
	slices.SortStableFunc(items, func(a, b autoItem) int { return b.at.Compare(a.at) })
	return items, nil
}

func dueRewatch(ctx context.Context, q *store.Queries, profileID int64, now time.Time) ([]autoItem, error) {
	ratings, err := q.Ratings(ctx, store.RatingsParams{ProfileID: profileID, Ratings: []int16{8, 9, 10}})
	if err != nil {
		return nil, err
	}
	last := map[int64]time.Time{}
	films, err := q.WatchedMovies(ctx, profileID)
	if err != nil {
		return nil, err
	}
	for _, f := range films {
		last[f.TitleID] = f.LastWatchedAt
	}
	episodes, err := q.WatchedEpisodes(ctx, store.WatchedEpisodesParams{ProfileID: profileID})
	if err != nil {
		return nil, err
	}
	for _, e := range episodes {
		if e.LastWatchedAt.After(last[e.TitleID]) {
			last[e.TitleID] = e.LastWatchedAt
		}
	}
	var items []autoItem
	for _, r := range ratings {
		at, ok := last[r.TitleID]
		if r.SeasonNumber == nil && r.EpisodeID == nil && ok && now.Sub(at) >= rewatchAfter {
			items = append(items, autoItem{titleID: r.TitleID, at: at})
		}
	}
	slices.SortStableFunc(items, func(a, b autoItem) int { return a.at.Compare(b.at) })
	return items, nil
}
