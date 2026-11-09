package jobs

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/tmdb"
)

type Outlines struct {
	DB   *pgxpool.Pool
	TMDB *tmdb.Client
	Log  *slog.Logger
}

func (o Outlines) Run(ctx context.Context) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		if err := o.Refresh(ctx); err != nil && ctx.Err() == nil {
			o.Log.Error("refreshing episode outlines", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (o Outlines) Refresh(ctx context.Context) error {
	due, err := core.DueOutlines(ctx, o.DB, 20)
	if err != nil {
		return err
	}
	for _, show := range due {
		episodes, err := o.TMDB.Outline(ctx, *show.TmdbID)
		if errors.Is(err, tmdb.ErrNotFound) {
			err = core.OutlineMissing(ctx, o.DB, show.ID)
		} else if err == nil {
			err = core.SaveOutline(ctx, o.DB, show.ID, outline(episodes))
		}
		if err != nil && ctx.Err() == nil {
			o.Log.Error("refreshing an episode outline", "show", show.ID, "err", err)
		}
	}

	groups, err := core.DueEpisodeOrders(ctx, o.DB, 20)
	if err != nil {
		return err
	}
	for _, id := range groups {
		order, err := o.Order(ctx, id)
		if errors.Is(err, core.ErrWrongShow) {
			err = core.EpisodeOrderMissing(ctx, o.DB, id)
		} else if err == nil {
			err = core.RefreshEpisodeOrder(ctx, o.DB, order)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (o Outlines) Order(ctx context.Context, groupID string) (core.EpisodeOrder, error) {
	g, err := o.TMDB.EpisodeGroup(ctx, groupID)
	if errors.Is(err, tmdb.ErrNotFound) {
		return core.EpisodeOrder{}, core.ErrWrongShow
	}
	if err != nil {
		return core.EpisodeOrder{}, err
	}
	order := core.EpisodeOrder{TMDBGroupID: g.ID, TMDBType: g.Type}
	for _, season := range g.Seasons {
		for _, e := range season.Episodes {
			order.Episodes = append(order.Episodes, core.OrderEntry{TMDBID: e.ID, ShowTMDBID: e.ShowID, Season: season.Number, Number: e.Place})
		}
	}
	return order, nil
}

func outline(episodes []tmdb.Episode) []core.OutlineEpisode {
	out := make([]core.OutlineEpisode, 0, len(episodes))
	for _, e := range episodes {
		out = append(out, core.OutlineEpisode{TMDBID: e.ID, Season: e.Season, Number: e.Number, FirstAired: e.AirDate})
	}
	return out
}
