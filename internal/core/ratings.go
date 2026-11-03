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

var HiddenSections = []string{"calendar", "progress_watched", "progress_watched_reset", "progress_collected", "recommendations", "comments", "dropped"}

type Rating struct {
	Ref    Ref
	Rating int16
	At     *time.Time
}

func Ratings(ctx context.Context, db *pgxpool.Pool, profileID int64, kind string, ratings []int16) ([]Entry, error) {
	q := store.New(db)
	if ratings == nil {
		ratings = []int16{}
	}
	rows, err := q.Ratings(ctx, store.RatingsParams{ProfileID: profileID, Kind: optional(kind), Ratings: ratings})
	if err != nil {
		return nil, err
	}
	var titleIDs, episodeIDs []int64
	for _, row := range rows {
		titleIDs = append(titleIDs, row.TitleID)
		if row.EpisodeID != nil {
			episodeIDs = append(episodeIDs, *row.EpisodeID)
		}
	}
	c, err := loadCatalogue(ctx, q, titleIDs, episodeIDs)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, Entry{ID: row.ID, At: row.RatedAt, Rating: row.Rating, Item: c.item(row.TitleID, row.SeasonNumber, row.EpisodeID)})
	}
	return entries, nil
}

func Rate(ctx context.Context, db *pgxpool.Pool, profileID int64, ratings []Rating) (SyncResult, error) {
	var result SyncResult
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		for _, rt := range ratings {
			if rt.Rating < 1 || rt.Rating > 10 {
				result.NotFound = append(result.NotFound, rt.Ref)
				continue
			}
			r, err := resolve(ctx, q, rt.Ref, true)
			if errors.Is(err, ErrNotFound) {
				result.NotFound = append(result.NotFound, rt.Ref)
				continue
			}
			if err != nil {
				return err
			}
			n, err := q.RateItem(ctx, store.RateItemParams{
				ProfileID:    profileID,
				TitleID:      r.title.ID,
				SeasonNumber: r.season,
				EpisodeID:    r.episodeID(),
				Rating:       rt.Rating,
				RatedAt:      rt.At,
			})
			if err != nil {
				return err
			}
			if n == 0 {
				result.Existing.add(rt.Ref.Kind)
				continue
			}
			result.Added.add(rt.Ref.Kind)
			if err := mark(ctx, q, profileID, area(r, "rated_at")); err != nil {
				return err
			}
		}
		return nil
	})
	return result, err
}

func Unrate(ctx context.Context, db *pgxpool.Pool, profileID int64, refs []Ref) (SyncResult, error) {
	var result SyncResult
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		for _, ref := range refs {
			r, err := resolve(ctx, q, ref, false)
			if errors.Is(err, ErrNotFound) {
				result.NotFound = append(result.NotFound, ref)
				continue
			}
			if err != nil {
				return err
			}
			n, err := q.UnrateItem(ctx, store.UnrateItemParams{ProfileID: profileID, TitleID: r.title.ID, SeasonNumber: r.season, EpisodeID: r.episodeID()})
			if err != nil {
				return err
			}
			if n == 0 {
				result.NotFound = append(result.NotFound, ref)
				continue
			}
			result.Deleted.add(ref.Kind)
			if err := mark(ctx, q, profileID, area(r, "rated_at")); err != nil {
				return err
			}
		}
		return nil
	})
	return result, err
}

func RemoveRating(ctx context.Context, db *pgxpool.Pool, profileID, id int64) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.DeleteRating(ctx, store.DeleteRatingParams{ID: id, ProfileID: profileID})
		if err != nil {
			return notFound(err)
		}
		r := resolved{title: store.Title{Kind: row.Kind}, season: row.SeasonNumber}
		if row.EpisodeID != nil {
			r.episode = &store.Episode{ID: *row.EpisodeID}
		}
		return mark(ctx, q, profileID, area(r, "rated_at"))
	})
}

func Hidden(ctx context.Context, db *pgxpool.Pool, profileID int64, section, kind string) ([]Entry, error) {
	q := store.New(db)
	rows, err := q.HiddenItems(ctx, store.HiddenItemsParams{ProfileID: profileID, Section: section})
	if err != nil {
		return nil, err
	}
	var titleIDs []int64
	for _, row := range rows {
		titleIDs = append(titleIDs, row.TitleID)
	}
	c, err := loadCatalogue(ctx, q, titleIDs, nil)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(rows))
	for _, row := range rows {
		e := Entry{At: row.HiddenAt, Item: c.item(row.TitleID, row.SeasonNumber, nil)}
		if kind == "" || e.Item.Kind() == kind {
			entries = append(entries, e)
		}
	}
	return entries, nil
}

func Hide(ctx context.Context, db *pgxpool.Pool, profileID int64, section string, refs []Ref) (SyncResult, error) {
	return changeHidden(ctx, db, profileID, section, refs, false, func(q *store.Queries, r resolved) (int64, error) {
		return q.ImportHiddenItem(ctx, store.ImportHiddenItemParams{
			ProfileID:    profileID,
			Section:      section,
			TitleID:      r.title.ID,
			SeasonNumber: r.season,
			HiddenAt:     time.Now(),
		})
	})
}

func Unhide(ctx context.Context, db *pgxpool.Pool, profileID int64, section string, refs []Ref) (SyncResult, error) {
	return changeHidden(ctx, db, profileID, section, refs, true, func(q *store.Queries, r resolved) (int64, error) {
		return q.UnhideItem(ctx, store.UnhideItemParams{ProfileID: profileID, Section: section, TitleID: r.title.ID, SeasonNumber: r.season})
	})
}

func changeHidden(ctx context.Context, db *pgxpool.Pool, profileID int64, section string, refs []Ref, removing bool, change func(*store.Queries, resolved) (int64, error)) (SyncResult, error) {
	var result SyncResult
	if !slices.Contains(HiddenSections, section) {
		return result, ErrNotFound
	}
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		for _, ref := range refs {
			r, err := resolve(ctx, q, ref, !removing)
			if errors.Is(err, ErrNotFound) || ref.Kind == "episode" {
				result.NotFound = append(result.NotFound, ref)
				continue
			}
			if err != nil {
				return err
			}
			n, err := change(q, r)
			if err != nil {
				return err
			}
			switch {
			case n == 0 && removing:
				result.NotFound = append(result.NotFound, ref)
				continue
			case n == 0:
				result.Existing.add(ref.Kind)
				continue
			case removing:
				result.Deleted.add(ref.Kind)
			default:
				result.Added.add(ref.Kind)
			}
			field := "hidden_at"
			if section == "dropped" {
				field = "dropped_at"
			}
			if err := mark(ctx, q, profileID, area(r, field)); err != nil {
				return err
			}
		}
		return nil
	})
	return result, err
}
