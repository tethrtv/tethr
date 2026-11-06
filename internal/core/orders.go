package core

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

var ErrWrongShow = errors.New("that episode order is for another show")

type OrderedEpisode struct {
	store.Episode
	Season int32
	Number int32
}

type EpisodeOrder struct {
	TMDBGroupID string
	TMDBType    int16
	Episodes    []OrderEntry
}

type OrderEntry struct {
	TMDBID     int32
	ShowTMDBID int32
	Season     int32
	Number     int32
}

func PickEpisodeOrder(ctx context.Context, db *pgxpool.Pool, profileID int64, id string, order *EpisodeOrder) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		show, err := titleByPath(ctx, q, id, "show")
		if err != nil {
			return err
		}
		var orderID *int64
		if order != nil {
			if show.TmdbID == nil || len(order.Episodes) == 0 {
				return ErrWrongShow
			}
			for _, e := range order.Episodes {
				if int64(e.ShowTMDBID) != int64(*show.TmdbID) {
					return ErrWrongShow
				}
			}
			saved, err := saveOrder(ctx, q, show.ID, *order)
			if err != nil {
				return err
			}
			orderID = &saved
		}
		if err := q.SetShowOrder(ctx, store.SetShowOrderParams{ProfileID: profileID, ShowID: show.ID, EpisodeOrderID: orderID}); err != nil {
			return err
		}
		return q.DropUnusedEpisodeOrders(ctx)
	})
}

func DueEpisodeOrders(ctx context.Context, db *pgxpool.Pool, limit int32) ([]string, error) {
	return store.New(db).DueEpisodeOrders(ctx, limit)
}

func RefreshEpisodeOrder(ctx context.Context, db *pgxpool.Pool, order EpisodeOrder) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		showID, err := q.EpisodeOrderShow(ctx, order.TMDBGroupID)
		if err != nil {
			return err
		}
		_, err = saveOrder(ctx, q, showID, order)
		return err
	})
}

func EpisodeOrderMissing(ctx context.Context, db *pgxpool.Pool, groupID string) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.ForgetEpisodeOrder(ctx, groupID); err != nil {
			return err
		}
		return q.DropUnusedEpisodeOrders(ctx)
	})
}

func saveOrder(ctx context.Context, q *store.Queries, showID int64, order EpisodeOrder) (int64, error) {
	saved, err := q.SaveEpisodeOrder(ctx, store.SaveEpisodeOrderParams{ShowID: showID, TmdbGroupID: order.TMDBGroupID, TmdbType: order.TMDBType})
	if err != nil {
		return 0, err
	}
	if saved.ShowID != showID {
		return 0, ErrWrongShow
	}
	if err := q.ClearEpisodeOrder(ctx, saved.ID); err != nil {
		return 0, err
	}
	seen := map[int32]bool{}
	var rows []store.AddEpisodeOrderEntriesParams
	for _, e := range order.Episodes {
		if seen[e.TMDBID] {
			continue
		}
		seen[e.TMDBID] = true
		rows = append(rows, store.AddEpisodeOrderEntriesParams{OrderID: saved.ID, TmdbEpisodeID: e.TMDBID, SeasonNumber: e.Season, EpisodeNumber: e.Number, Position: int32(len(rows))})
	}
	_, err = q.AddEpisodeOrderEntries(ctx, rows)
	return saved.ID, err
}

func showEpisodes(ctx context.Context, q *store.Queries, profileID int64, showIDs []int64) (map[int64][]OrderedEpisode, error) {
	aired, err := q.OutlineEpisodes(ctx, showIDs)
	if err != nil {
		return nil, err
	}
	ordered, err := q.OrderedEpisodes(ctx, store.OrderedEpisodesParams{ProfileID: profileID, ShowIds: showIDs})
	if err != nil {
		return nil, err
	}
	out := map[int64][]OrderedEpisode{}
	for _, e := range ordered {
		out[e.ShowID] = append(out[e.ShowID], OrderedEpisode{Episode: e.Episode, Season: e.OrderSeason, Number: e.OrderNumber})
	}
	inOrder := map[int64]bool{}
	for id := range out {
		inOrder[id] = true
	}
	for _, e := range aired {
		if !inOrder[e.ShowID] {
			out[e.ShowID] = append(out[e.ShowID], OrderedEpisode{Episode: e, Season: e.SeasonNumber, Number: e.EpisodeNumber})
		}
	}
	return out, nil
}
