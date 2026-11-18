package core

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

var (
	ErrTitleInUse = errors.New("someone uses this title, so it stays")
	ErrTMDBTaken  = errors.New("another title has that TMDB ID")
)

func RetireTitle(ctx context.Context, db *pgxpool.Pool, titleID int64) (bool, error) {
	retired := false
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		switch err := retireTitle(ctx, store.New(tx), titleID); {
		case errors.Is(err, ErrTitleInUse):
			return nil
		case errors.Is(err, pgx.ErrNoRows):
			retired = true
			return nil
		case err != nil:
			return err
		}
		retired = true
		return nil
	})
	return retired, err
}

func retireTitle(ctx context.Context, q *store.Queries, titleID int64) error {
	if err := unusedTitle(ctx, q, titleID); err != nil {
		return err
	}
	if err := q.DetachTitleReview(ctx, &titleID); err != nil {
		return err
	}
	if err := q.DeleteTitleEpisodes(ctx, titleID); err != nil {
		return err
	}
	if err := q.DeleteTitleSeasons(ctx, titleID); err != nil {
		return err
	}
	if err := q.DeleteTitleExternalIDs(ctx, titleID); err != nil {
		return err
	}
	return q.DeleteTitle(ctx, titleID)
}

func DropTitleTMDB(ctx context.Context, db *pgxpool.Pool, titleID int64, tmdb int32) (bool, error) {
	dropped := false
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		t, err := q.LockTitle(ctx, titleID)
		if err != nil || t.TmdbID == nil || *t.TmdbID != tmdb {
			return err
		}
		switch err := unusedTitle(ctx, q, titleID); {
		case errors.Is(err, ErrTitleInUse):
			return nil
		case err != nil:
			return err
		}
		dropped = true
		return setTMDB(ctx, q, t, nil)
	})
	return dropped, err
}

func takeWikidataTMDB(ctx context.Context, q *store.Queries, titleID int64, tmdb int32) error {
	t, err := q.LockTitle(ctx, titleID)
	if err != nil {
		return err
	}
	return setTMDB(ctx, q, t, &tmdb)
}

func setTMDB(ctx context.Context, q *store.Queries, t store.Title, tmdb *int32) error {
	n, err := q.SetTitleTMDBFromWikidata(ctx, store.SetTitleTMDBFromWikidataParams{ID: t.ID, TmdbID: tmdb})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrTMDBTaken
	}
	if t.Kind != "show" {
		return nil
	}
	// Episodes keep their numbers but lose the old show's episode IDs, so the next outline matches them by number and keeps their watches.
	return q.ForgetEpisodeTMDB(ctx, t.ID)
}

func unusedTitle(ctx context.Context, q *store.Queries, titleID int64) error {
	if _, err := q.LockTitle(ctx, titleID); err != nil {
		return err
	}
	used, err := q.TitleInUse(ctx, titleID)
	if err != nil {
		return err
	}
	if used {
		return ErrTitleInUse
	}
	return nil
}

func TitleInUse(ctx context.Context, db *pgxpool.Pool, titleID int64) (bool, error) {
	return store.New(db).TitleInUse(ctx, titleID)
}
