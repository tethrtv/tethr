package core

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

type UpNextEntry struct {
	Show          Item
	Next          OrderedEpisode
	Last          *OrderedEpisode
	LastWatchedAt time.Time
	Aired         int64
	Watched       int64
	Resume        *Entry
}

func UpNext(ctx context.Context, db *pgxpool.Pool, profileID int64) ([]UpNextEntry, error) {
	q := store.New(db)
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
		if hiddenShows[r.TitleID] {
			continue
		}
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

	now := time.Now()
	var entries []UpNextEntry
	var listed, nextIDs []int64
	for _, id := range showIDs {
		p := showProgress(episodes[id], watched[id], hiddenSeasons[id], false, now)
		if p.Next == nil {
			continue
		}
		e := UpNextEntry{Next: *p.Next, Last: p.Last, Aired: int64(p.Aired), Watched: int64(p.Completed)}
		for _, at := range watched[id] {
			if at.After(e.LastWatchedAt) {
				e.LastWatchedAt = at
			}
		}
		e.Show.Title.ID = id
		entries = append(entries, e)
		listed = append(listed, id)
		nextIDs = append(nextIDs, p.Next.ID)
	}
	slices.SortStableFunc(entries, func(a, b UpNextEntry) int { return b.LastWatchedAt.Compare(a.LastWatchedAt) })

	c, err := loadCatalogue(ctx, q, 0, listed, nil)
	if err != nil {
		return nil, err
	}
	playback, err := q.EpisodePlayback(ctx, store.EpisodePlaybackParams{ProfileID: profileID, EpisodeIds: nextIDs})
	if err != nil {
		return nil, err
	}
	resume := map[int64]*Entry{}
	for _, pb := range playback {
		resume[*pb.EpisodeID] = &Entry{ID: pb.ID, Progress: pb.Progress}
	}
	for i := range entries {
		entries[i].Show = c.item(entries[i].Show.Title.ID, nil, nil)
		entries[i].Resume = resume[entries[i].Next.ID]
	}
	return entries, nil
}

func hiddenFromUpNext(ctx context.Context, q *store.Queries, profileID int64) (map[int64]bool, map[int64][]int32, error) {
	shows := map[int64]bool{}
	seasons := map[int64][]int32{}
	for _, section := range []string{"progress_watched", "dropped"} {
		items, err := q.HiddenItems(ctx, store.HiddenItemsParams{ProfileID: profileID, Section: section})
		if err != nil {
			return nil, nil, err
		}
		for _, h := range items {
			switch {
			case h.SeasonNumber == nil:
				shows[h.TitleID] = true
			case section == "progress_watched":
				seasons[h.TitleID] = append(seasons[h.TitleID], *h.SeasonNumber)
			}
		}
	}
	return shows, seasons, nil
}

func HideFromUpNext(ctx context.Context, db *pgxpool.Pool, profileID, showID int64) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		if _, err := q.TitleByID(ctx, store.TitleByIDParams{ID: showID, Kind: "show"}); err != nil {
			return notFound(err)
		}
		if err := q.HideItem(ctx, store.HideItemParams{ProfileID: profileID, Section: "progress_watched", TitleID: showID}); err != nil {
			return err
		}
		return mark(ctx, q, profileID, "shows.hidden_at")
	})
}

func ShowInUpNext(ctx context.Context, db *pgxpool.Pool, profileID, showID int64) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		n, err := q.UnhideItem(ctx, store.UnhideItemParams{ProfileID: profileID, Section: "progress_watched", TitleID: showID})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		return mark(ctx, q, profileID, "shows.hidden_at")
	})
}

const maxUpNextLists = 50

var ErrUpNextFull = errors.New("up to 50 lists can be in up next")

type UpNextList struct {
	List          List
	ItemID        int64
	Position      int
	Next          Item
	LastWatchedAt *time.Time
	Resume        *Entry
}

func UpNextLists(ctx context.Context, db *pgxpool.Pool, profileID int64) ([]UpNextList, error) {
	q := store.New(db)
	rows, err := q.UpNextListIDs(ctx, profileID)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	added := map[int64]time.Time{}
	for _, r := range rows {
		ids = append(ids, r.ListID)
		added[r.ListID] = r.AddedAt
	}
	lists, err := visibleLists(ctx, q, profileID, store.VisibleListsParams{ListIds: ids})
	if err != nil {
		return nil, err
	}
	items, err := q.ListItemsIn(ctx, ids)
	if err != nil {
		return nil, err
	}
	progress, err := itemProgress(ctx, q, profileID, items)
	if err != nil {
		return nil, err
	}
	var titleIDs, episodeIDs []int64
	for _, item := range items {
		titleIDs = append(titleIDs, item.TitleID)
		if item.EpisodeID != nil {
			episodeIDs = append(episodeIDs, *item.EpisodeID)
		}
		if next := progress[item.ID].Next; next != nil {
			episodeIDs = append(episodeIDs, next.ID)
		}
	}
	c, err := loadCatalogue(ctx, q, profileID, titleIDs, episodeIDs)
	if err != nil {
		return nil, err
	}
	entries := map[int64][]ListEntry{}
	for _, item := range items {
		p := progress[item.ID]
		e := Entry{ID: item.ID, At: item.ListedAt, Rank: item.Rank, Item: c.item(item.TitleID, item.SeasonNumber, item.EpisodeID)}
		entries[item.ListID] = append(entries[item.ListID], ListEntry{Entry: e, Progress: &p})
	}

	var out []UpNextList
	var nextTitles []int64
	for _, l := range lists {
		sortEntries(entries[l.ID], l.SortBy, l.SortHow)
		u := UpNextList{List: l}
		for i, e := range entries[l.ID] {
			p := e.Progress
			if p.LastWatchedAt != nil && (u.LastWatchedAt == nil || p.LastWatchedAt.After(*u.LastWatchedAt)) {
				u.LastWatchedAt = p.LastWatchedAt
			}
			if u.Position > 0 || p.Total == 0 || p.Done() {
				continue
			}
			u.ItemID, u.Position, u.Next = e.ID, i+1, e.Item
			if p.Next != nil {
				u.Next = c.item(e.Item.Title.ID, nil, &p.Next.ID)
			}
		}
		if u.Position > 0 {
			out = append(out, u)
			nextTitles = append(nextTitles, u.Next.Title.ID)
		}
	}
	if len(out) == 0 {
		return out, nil
	}

	playback, err := q.ItemPlayback(ctx, store.ItemPlaybackParams{ProfileID: profileID, TitleIds: nextTitles})
	if err != nil {
		return nil, err
	}
	for i := range out {
		var episodeID *int64
		if out[i].Next.Episode != nil {
			episodeID = &out[i].Next.Episode.ID
		}
		for _, pb := range playback {
			if pb.TitleID == out[i].Next.Title.ID && equalIDs(pb.EpisodeID, episodeID) {
				out[i].Resume = &Entry{ID: pb.ID, Progress: pb.Progress}
			}
		}
	}
	latest := func(u UpNextList) time.Time {
		if u.LastWatchedAt != nil {
			return *u.LastWatchedAt
		}
		return added[u.List.ID]
	}
	slices.SortStableFunc(out, func(a, b UpNextList) int { return latest(b).Compare(latest(a)) })
	return out, nil
}

func equalIDs(a, b *int64) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func AddListToUpNext(ctx context.Context, db *pgxpool.Pool, profileID, listID int64) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		l, err := getList(ctx, q, profileID, listID)
		if err != nil || l.InUpNext {
			return err
		}
		n, err := q.CountUpNextLists(ctx, profileID)
		if err != nil {
			return err
		}
		if n >= maxUpNextLists {
			return ErrUpNextFull
		}
		if err := q.AddUpNextList(ctx, store.AddUpNextListParams{ProfileID: profileID, ListID: listID}); err != nil {
			return err
		}
		return mark(ctx, q, profileID, "lists.updated_at")
	})
}

func RemoveListFromUpNext(ctx context.Context, db *pgxpool.Pool, profileID, listID int64) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		n, err := q.RemoveUpNextList(ctx, store.RemoveUpNextListParams{ProfileID: profileID, ListID: listID})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		return mark(ctx, q, profileID, "lists.updated_at")
	})
}
