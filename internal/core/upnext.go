package core

import (
	"context"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

type UpNextEntry struct {
	Show          Item
	Next          store.Episode
	Last          *store.Episode
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
	var hidden []store.HiddenItem
	for _, section := range []string{"progress_watched", "dropped"} {
		items, err := q.HiddenItems(ctx, store.HiddenItemsParams{ProfileID: profileID, Section: section})
		if err != nil {
			return nil, err
		}
		hidden = append(hidden, items...)
	}
	hiddenShows := map[int64]bool{}
	hiddenSeasons := map[int64][]int32{}
	for _, h := range hidden {
		switch {
		case h.SeasonNumber == nil:
			hiddenShows[h.TitleID] = true
		case h.Section == "progress_watched":
			hiddenSeasons[h.TitleID] = append(hiddenSeasons[h.TitleID], *h.SeasonNumber)
		}
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
	outlines, err := q.OutlineEpisodes(ctx, showIDs)
	if err != nil {
		return nil, err
	}
	episodes := map[int64][]store.Episode{}
	for _, e := range outlines {
		episodes[e.ShowID] = append(episodes[e.ShowID], e)
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

	c, err := loadCatalogue(ctx, q, listed, nil)
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
