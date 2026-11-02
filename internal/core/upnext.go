package core

import (
	"context"
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
	rows, err := q.UpNext(ctx, profileID)
	if err != nil {
		return nil, err
	}
	var titleIDs, episodeIDs []int64
	for _, r := range rows {
		titleIDs = append(titleIDs, r.TitleID)
		episodeIDs = append(episodeIDs, r.EpisodeID)
		if r.LastEpisodeID != nil {
			episodeIDs = append(episodeIDs, *r.LastEpisodeID)
		}
	}
	c, err := loadCatalogue(ctx, q, titleIDs, episodeIDs)
	if err != nil {
		return nil, err
	}
	entries := make([]UpNextEntry, 0, len(rows))
	for _, r := range rows {
		e := UpNextEntry{
			Show:          c.item(r.TitleID, nil, nil),
			Next:          c.episodes[r.EpisodeID],
			LastWatchedAt: r.LastWatchedAt,
			Aired:         r.Aired,
			Watched:       r.Watched,
		}
		if r.LastEpisodeID != nil {
			last := c.episodes[*r.LastEpisodeID]
			e.Last = &last
		}
		if r.PlaybackID != nil {
			e.Resume = &Entry{ID: *r.PlaybackID, Progress: *r.PlaybackProgress}
		}
		entries = append(entries, e)
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
