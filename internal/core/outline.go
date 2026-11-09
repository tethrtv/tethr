package core

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

type OutlineEpisode struct {
	TMDBID     int32
	Season     int32
	Number     int32
	FirstAired *time.Time
}

func DueOutlines(ctx context.Context, db *pgxpool.Pool, limit int32) ([]store.DueOutlinesRow, error) {
	return store.New(db).DueOutlines(ctx, limit)
}

func OutlineMissing(ctx context.Context, db *pgxpool.Pool, showID int64) error {
	now := time.Now()
	return store.New(db).OutlineDone(ctx, store.OutlineDoneParams{ID: showID, MissingAt: &now})
}

func SaveOutline(ctx context.Context, db *pgxpool.Pool, showID int64, outline []OutlineEpisode) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		existing, err := q.ShowEpisodes(ctx, showID)
		if err != nil {
			return err
		}
		wanted := map[int32]OutlineEpisode{}
		for _, e := range outline {
			wanted[e.TMDBID] = e
		}

		byTMDB := map[int32]store.Episode{}
		type slot struct{ season, number int32 }
		loose := map[slot]store.Episode{}
		for _, e := range existing {
			if e.TmdbEpisodeID == nil {
				if e.RemovedAt == nil {
					loose[slot{e.SeasonNumber, e.EpisodeNumber}] = e
				}
				continue
			}
			byTMDB[*e.TmdbEpisodeID] = e
			w, ok := wanted[*e.TmdbEpisodeID]
			moved := ok && (w.Season != e.SeasonNumber || w.Number != e.EpisodeNumber)
			if (!ok || moved) && e.RemovedAt == nil {
				if err := q.RemoveEpisode(ctx, e.ID); err != nil {
					return err
				}
			}
		}

		for _, w := range outline {
			tmdbID := w.TMDBID
			lazy, hasLazy := loose[slot{w.Season, w.Number}]
			e, known := byTMDB[w.TMDBID]
			switch {
			case known:
				if hasLazy {
					if err := mergeEpisode(ctx, q, lazy.ID, e.ID); err != nil {
						return err
					}
				}
				err = q.UpdateEpisode(ctx, store.UpdateEpisodeParams{ID: e.ID, SeasonNumber: w.Season, EpisodeNumber: w.Number, TmdbEpisodeID: &tmdbID, FirstAired: w.FirstAired})
			case hasLazy:
				err = q.UpdateEpisode(ctx, store.UpdateEpisodeParams{ID: lazy.ID, SeasonNumber: w.Season, EpisodeNumber: w.Number, TmdbEpisodeID: &tmdbID, FirstAired: w.FirstAired})
			default:
				err = q.AddOutlineEpisode(ctx, store.AddOutlineEpisodeParams{ShowID: showID, SeasonNumber: w.Season, EpisodeNumber: w.Number, TmdbEpisodeID: &tmdbID, FirstAired: w.FirstAired})
			}
			if err != nil {
				return err
			}
		}
		return q.OutlineDone(ctx, store.OutlineDoneParams{ID: showID})
	})
}

func mergeEpisode(ctx context.Context, q *store.Queries, from, into int64) error {
	if err := q.MergeEpisodeEvents(ctx, store.MergeEpisodeEventsParams{FromID: &from, IntoID: &into}); err != nil {
		return err
	}
	if err := q.DropClashingPlayback(ctx, store.DropClashingPlaybackParams{FromID: &from, IntoID: &into}); err != nil {
		return err
	}
	if err := q.MergeEpisodePlayback(ctx, store.MergeEpisodePlaybackParams{FromID: &from, IntoID: &into}); err != nil {
		return err
	}
	if err := q.TouchListsWithEpisodes(ctx, []int64{from, into}); err != nil {
		return err
	}
	if err := q.DropClashingListItems(ctx, store.DropClashingListItemsParams{FromID: &from, IntoID: &into}); err != nil {
		return err
	}
	if err := q.MergeEpisodeListItems(ctx, store.MergeEpisodeListItemsParams{FromID: &from, IntoID: &into}); err != nil {
		return err
	}
	if err := q.DropClashingRatings(ctx, store.DropClashingRatingsParams{FromID: &from, IntoID: &into}); err != nil {
		return err
	}
	if err := q.MergeEpisodeRatings(ctx, store.MergeEpisodeRatingsParams{FromID: &from, IntoID: &into}); err != nil {
		return err
	}
	if err := q.MergeEpisodeWatching(ctx, store.MergeEpisodeWatchingParams{FromID: &from, IntoID: &into}); err != nil {
		return err
	}
	if err := q.MergeEpisodeHeldPlays(ctx, store.MergeEpisodeHeldPlaysParams{FromID: &from, IntoID: &into}); err != nil {
		return err
	}
	return q.DeleteEpisode(ctx, from)
}
