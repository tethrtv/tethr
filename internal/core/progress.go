package core

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

type Plays struct {
	Plays         int64
	LastWatchedAt time.Time
}

type WatchedEpisode struct {
	Plays
	Episode store.Episode
}

type WatchedTitle struct {
	Plays
	Item     Item
	Episodes []WatchedEpisode
}

func Watched(ctx context.Context, db *pgxpool.Pool, profileID int64, kind string) ([]WatchedTitle, error) {
	q := store.New(db)
	var out []WatchedTitle
	if kind == "movie" {
		rows, err := q.WatchedMovies(ctx, profileID)
		if err != nil {
			return nil, err
		}
		var ids []int64
		for _, r := range rows {
			ids = append(ids, r.TitleID)
		}
		c, err := loadCatalogue(ctx, q, ids, nil)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, WatchedTitle{Plays: Plays{r.Plays, r.LastWatchedAt}, Item: c.item(r.TitleID, nil, nil)})
		}
		return out, nil
	}
	rows, err := q.WatchedEpisodes(ctx, store.WatchedEpisodesParams{ProfileID: profileID})
	if err != nil {
		return nil, err
	}
	var titleIDs, episodeIDs []int64
	for _, r := range rows {
		titleIDs = append(titleIDs, r.TitleID)
		episodeIDs = append(episodeIDs, *r.EpisodeID)
	}
	c, err := loadCatalogue(ctx, q, titleIDs, episodeIDs)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if len(out) == 0 || out[len(out)-1].Item.Title.ID != r.TitleID {
			out = append(out, WatchedTitle{Item: c.item(r.TitleID, nil, nil)})
		}
		show := &out[len(out)-1]
		show.Plays.Plays += r.Plays
		if r.LastWatchedAt.After(show.LastWatchedAt) {
			show.LastWatchedAt = r.LastWatchedAt
		}
		show.Episodes = append(show.Episodes, WatchedEpisode{Plays: Plays{r.Plays, r.LastWatchedAt}, Episode: c.episodes[*r.EpisodeID]})
	}
	slices.SortStableFunc(out, func(a, b WatchedTitle) int { return b.LastWatchedAt.Compare(a.LastWatchedAt) })
	return out, nil
}

type EpisodeProgress struct {
	Episode       store.Episode
	LastWatchedAt *time.Time
}

type SeasonProgress struct {
	Number    int32
	Aired     int
	Completed int
	Episodes  []EpisodeProgress
}

type ShowProgress struct {
	Show          Item
	Aired         int
	Completed     int
	LastWatchedAt *time.Time
	Seasons       []SeasonProgress
	HiddenSeasons []int32
	Next          *store.Episode
	Last          *store.Episode
}

func Progress(ctx context.Context, db *pgxpool.Pool, profileID int64, show Ref, specials bool) (ShowProgress, error) {
	q := store.New(db)
	r, err := resolve(ctx, q, show, false)
	if err != nil {
		return ShowProgress{}, err
	}
	if r.title.Kind != "show" {
		return ShowProgress{}, ErrNotFound
	}
	item, err := describe(ctx, q, resolved{title: r.title})
	if err != nil {
		return ShowProgress{}, err
	}
	p := ShowProgress{Show: item, Seasons: []SeasonProgress{}}
	aired, err := q.AiredEpisodes(ctx, store.AiredEpisodesParams{ShowID: r.title.ID, Specials: specials})
	if err != nil {
		return p, err
	}
	watched, err := q.WatchedEpisodes(ctx, store.WatchedEpisodesParams{ProfileID: profileID, ShowID: &r.title.ID})
	if err != nil {
		return p, err
	}
	p.HiddenSeasons, err = q.HiddenSeasons(ctx, store.HiddenSeasonsParams{ProfileID: profileID, TitleID: r.title.ID})
	if err != nil {
		return p, err
	}
	seen := map[int64]time.Time{}
	var last *store.WatchedEpisodesRow
	for i, w := range watched {
		seen[*w.EpisodeID] = w.LastWatchedAt
		if last == nil || w.LastWatchedAt.After(last.LastWatchedAt) {
			last = &watched[i]
		}
	}
	if last != nil {
		p.LastWatchedAt = &last.LastWatchedAt
		episodes, err := q.CatalogueEpisodes(ctx, []int64{*last.EpisodeID})
		if err != nil {
			return p, err
		}
		if len(episodes) == 1 {
			p.Last = &episodes[0]
		}
	}

	var afterLast, gap *store.Episode
	for i, e := range aired {
		if slices.Contains(p.HiddenSeasons, e.SeasonNumber) {
			continue
		}
		if len(p.Seasons) == 0 || p.Seasons[len(p.Seasons)-1].Number != e.SeasonNumber {
			p.Seasons = append(p.Seasons, SeasonProgress{Number: e.SeasonNumber})
		}
		s := &p.Seasons[len(p.Seasons)-1]
		ep := EpisodeProgress{Episode: e}
		if at, ok := seen[e.ID]; ok {
			ep.LastWatchedAt = &at
			s.Completed++
		} else if e.SeasonNumber > 0 {
			if gap == nil {
				gap = &aired[i]
			}
			if afterLast == nil && last != nil && laterThan(e, last.SeasonNumber, last.EpisodeNumber) {
				afterLast = &aired[i]
			}
		}
		s.Aired++
		s.Episodes = append(s.Episodes, ep)
	}
	for _, s := range p.Seasons {
		if s.Number > 0 || specials {
			p.Aired += s.Aired
			p.Completed += s.Completed
		}
	}
	p.Next = cmp.Or(afterLast, gap)
	return p, nil
}

func laterThan(e store.Episode, season, number int32) bool {
	return e.SeasonNumber > season || (e.SeasonNumber == season && e.EpisodeNumber > number)
}
