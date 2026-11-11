package core

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

// A rewatch is Trakt's progress reset: only plays after it count towards progress.
func StartRewatch(ctx context.Context, db *pgxpool.Pool, profileID, showID int64) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		if _, err := q.TitleByID(ctx, store.TitleByIDParams{ID: showID, Kind: "show"}); err != nil {
			return notFound(err)
		}
		if err := q.StartRewatch(ctx, store.StartRewatchParams{ProfileID: profileID, TitleID: showID, HiddenAt: time.Now()}); err != nil {
			return err
		}
		return mark(ctx, q, profileID, "shows.hidden_at")
	})
}

func StopRewatch(ctx context.Context, db *pgxpool.Pool, profileID, showID int64) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		n, err := q.UnhideItem(ctx, store.UnhideItemParams{ProfileID: profileID, Section: "progress_watched_reset", TitleID: showID})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		return mark(ctx, q, profileID, "shows.hidden_at")
	})
}

func rewatches(ctx context.Context, q *store.Queries, profileID int64) (map[int64]time.Time, error) {
	items, err := q.HiddenItems(ctx, store.HiddenItemsParams{ProfileID: profileID, Section: "progress_watched_reset"})
	if err != nil {
		return nil, err
	}
	out := map[int64]time.Time{}
	for _, h := range items {
		if h.SeasonID == nil {
			out[h.TitleID] = h.HiddenAt
		}
	}
	return out, nil
}
