package core

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

const upcomingDays = 7

type UpcomingEntry struct {
	Show      Item
	Episode   OrderedEpisode
	Watchlist bool
}

func Upcoming(ctx context.Context, db *pgxpool.Pool, profileID int64) ([]UpcomingEntry, error) {
	q := store.New(db)
	rows, err := q.WatchedEpisodes(ctx, store.WatchedEpisodesParams{ProfileID: profileID})
	if err != nil {
		return nil, err
	}
	hidden, _, err := hiddenFromUpNext(ctx, q, profileID)
	if err != nil {
		return nil, err
	}
	watched := map[int64]bool{}
	started := map[int64]bool{}
	var showIDs []int64
	for _, r := range rows {
		watched[*r.EpisodeID] = true
		if !started[r.TitleID] && !hidden[r.TitleID] {
			started[r.TitleID] = true
			showIDs = append(showIDs, r.TitleID)
		}
	}
	listed, err := q.WatchlistShows(ctx, profileID)
	if err != nil {
		return nil, err
	}
	for _, id := range listed {
		if !started[id] && !hidden[id] {
			showIDs = append(showIDs, id)
		}
	}
	episodes, err := showEpisodes(ctx, q, profileID, showIDs)
	if err != nil {
		return nil, err
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	week := today.AddDate(0, 0, upcomingDays)
	var entries []UpcomingEntry
	for _, id := range showIDs {
		var soon []OrderedEpisode
		var later *OrderedEpisode
		for _, e := range episodes[id] {
			switch {
			case e.SeasonNumber == 0, e.FirstAired == nil, e.FirstAired.Before(today), watched[e.ID]:
				continue
			case !started[id] && e.EpisodeNumber != 1:
				continue
			case !e.FirstAired.After(week):
				soon = append(soon, e)
			case later == nil || e.FirstAired.Before(*later.FirstAired):
				later = &e
			}
		}
		if len(soon) == 0 && later != nil {
			soon = append(soon, *later)
		}
		for _, e := range soon {
			entries = append(entries, UpcomingEntry{Show: Item{Title: store.CatalogueTitlesRow{ID: id}}, Episode: e, Watchlist: !started[id]})
		}
	}
	slices.SortStableFunc(entries, func(a, b UpcomingEntry) int {
		return cmp.Or(a.Episode.FirstAired.Compare(*b.Episode.FirstAired), cmp.Compare(a.Show.Title.ID, b.Show.Title.ID))
	})

	c, err := loadCatalogue(ctx, q, 0, showIDs, nil)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		entries[i].Show = c.item(entries[i].Show.Title.ID, nil, nil)
	}
	return entries, nil
}
