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
	episodes, err := q.OutlineEpisodes(ctx, []int64{r.title.ID})
	if err != nil {
		return ShowProgress{}, err
	}
	rows, err := q.WatchedEpisodes(ctx, store.WatchedEpisodesParams{ProfileID: profileID, ShowID: &r.title.ID})
	if err != nil {
		return ShowProgress{}, err
	}
	watched := map[int64]time.Time{}
	for _, w := range rows {
		watched[*w.EpisodeID] = w.LastWatchedAt
	}
	hidden, err := q.HiddenSeasons(ctx, store.HiddenSeasonsParams{ProfileID: profileID, TitleID: r.title.ID})
	if err != nil {
		return ShowProgress{}, err
	}
	p := showProgress(episodes, watched, hidden, specials, time.Now())
	p.Show = item
	p.HiddenSeasons = hidden
	return p, nil
}

// Episodes come in the order the profile watches the show in, so next is worked out by position, not by number.
func showProgress(episodes []store.Episode, watched map[int64]time.Time, hidden []int32, specials bool, now time.Time) ShowProgress {
	p := ShowProgress{Seasons: []SeasonProgress{}}
	last := -1
	for i, e := range episodes {
		at, ok := watched[e.ID]
		if ok && (last < 0 || !at.Before(*p.LastWatchedAt)) {
			last, p.LastWatchedAt = i, &at
		}
	}
	if last >= 0 {
		p.Last = &episodes[last]
	}

	var afterLast, gap *store.Episode
	for i, e := range episodes {
		if e.FirstAired == nil || e.FirstAired.After(now) || slices.Contains(hidden, e.SeasonNumber) || (e.SeasonNumber == 0 && !specials) {
			continue
		}
		if len(p.Seasons) == 0 || p.Seasons[len(p.Seasons)-1].Number != e.SeasonNumber {
			p.Seasons = append(p.Seasons, SeasonProgress{Number: e.SeasonNumber})
		}
		s := &p.Seasons[len(p.Seasons)-1]
		ep := EpisodeProgress{Episode: e}
		if at, ok := watched[e.ID]; ok {
			ep.LastWatchedAt = &at
			s.Completed++
			p.Completed++
		} else if e.SeasonNumber > 0 {
			if gap == nil {
				gap = &episodes[i]
			}
			if afterLast == nil && last >= 0 && i > last {
				afterLast = &episodes[i]
			}
		}
		s.Aired++
		s.Episodes = append(s.Episodes, ep)
		p.Aired++
	}
	p.Next = cmp.Or(afterLast, gap)
	return p
}
