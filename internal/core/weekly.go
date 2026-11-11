package core

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

const weeklyListItems = 3

var ErrInvalidTimeZone = errors.New("that isn't a time zone we know")

type AccountChanges struct {
	TimeZone    *string
	WeeklyEmail *bool
}

func UpdateAccount(ctx context.Context, db *pgxpool.Pool, accountID int64, c AccountChanges) (store.Account, error) {
	if c.TimeZone != nil {
		if _, err := ParseTimeZone(*c.TimeZone); err != nil {
			return store.Account{}, err
		}
	}
	return store.New(db).UpdateAccountSettings(ctx, store.UpdateAccountSettingsParams{ID: accountID, TimeZone: c.TimeZone, WeeklyEmail: c.WeeklyEmail})
}

func ParseTimeZone(name string) (*time.Location, error) {
	if name == "" || name == "Local" || len(name) > 64 {
		return nil, ErrInvalidTimeZone
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, ErrInvalidTimeZone
	}
	return loc, nil
}

func Unsubscribe(ctx context.Context, db *pgxpool.Pool, token string) error {
	var id pgtype.UUID
	if err := id.Scan(token); err != nil {
		return ErrNotFound
	}
	n, err := store.New(db).Unsubscribe(ctx, id)
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

type Weekly struct {
	From      time.Time
	Episodes  []UpcomingEntry
	Premieres []UpcomingEntry
	Popular   []PopularTitle
	Lists     []WeeklyList
	Watched   WeeklyWatched
}

type WeeklyList struct {
	List  List
	Added []Item
	More  int
}

type WeeklyWatched struct {
	Episodes int
	Films    int
	Minutes  int
}

func (w Weekly) Empty() bool {
	return len(w.Episodes) == 0 && len(w.Premieres) == 0 && len(w.Lists) == 0 && w.Watched.Episodes+w.Watched.Films == 0
}

func WeekAhead(ctx context.Context, db *pgxpool.Pool, profileID int64, now time.Time, loc *time.Location) (Weekly, error) {
	q := store.New(db)
	local := now.In(loc)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	w := Weekly{From: today.AddDate(0, 0, 1)}
	end := w.From.AddDate(0, 0, 7)

	entries, err := upcoming(ctx, q, profileID, today)
	if err != nil {
		return w, err
	}
	for _, e := range entries {
		if e.Episode.FirstAired.Before(w.From) || !e.Episode.FirstAired.Before(end) {
			continue
		}
		if e.Watchlist {
			w.Premieres = append(w.Premieres, e)
		} else {
			w.Episodes = append(w.Episodes, e)
		}
	}

	if w.Popular, err = Popular(ctx, db, profileID, now); err != nil {
		return w, err
	}
	if w.Lists, err = weeklyLists(ctx, db, q, profileID, now.AddDate(0, 0, -7)); err != nil {
		return w, err
	}
	w.Watched, err = weeklyWatched(ctx, q, profileID, now)
	return w, err
}

func weeklyLists(ctx context.Context, db *pgxpool.Pool, q *store.Queries, profileID int64, since time.Time) ([]WeeklyList, error) {
	lists, err := FollowedLists(ctx, db, profileID)
	if err != nil || len(lists) == 0 {
		return nil, err
	}
	ids := make([]int64, len(lists))
	for i, l := range lists {
		ids[i] = l.ID
	}
	added, err := q.ListItemsAddedSince(ctx, store.ListItemsAddedSinceParams{ListIds: ids, Since: since})
	if err != nil {
		return nil, err
	}
	byList := map[int64][]int64{}
	var titleIDs []int64
	for _, a := range added {
		byList[a.ListID] = append(byList[a.ListID], a.TitleID)
		titleIDs = append(titleIDs, a.TitleID)
	}
	c, err := loadCatalogue(ctx, q, profileID, titleIDs, nil)
	if err != nil {
		return nil, err
	}
	var out []WeeklyList
	for _, l := range lists {
		titles := byList[l.ID]
		if len(titles) == 0 {
			continue
		}
		wl := WeeklyList{List: l, More: max(0, len(titles)-weeklyListItems)}
		for _, id := range titles[:min(len(titles), weeklyListItems)] {
			wl.Added = append(wl.Added, c.item(id, nil, nil))
		}
		out = append(out, wl)
	}
	return out, nil
}

func weeklyWatched(ctx context.Context, q *store.Queries, profileID int64, now time.Time) (WeeklyWatched, error) {
	var w WeeklyWatched
	plays, err := q.YearPlays(ctx, store.YearPlaysParams{ProfileID: profileID, StartAt: now.AddDate(0, 0, -7), EndAt: now})
	if err != nil {
		return w, err
	}
	var titleIDs []int64
	for _, p := range plays {
		titleIDs = append(titleIDs, p.TitleID)
	}
	c, err := loadCatalogue(ctx, q, profileID, titleIDs, nil)
	if err != nil {
		return w, err
	}
	for _, p := range plays {
		if p.EpisodeID != nil {
			w.Episodes++
		} else {
			w.Films++
		}
		switch t := c.titles[p.TitleID]; {
		case p.DurationSeconds != nil && *p.DurationSeconds > 0:
			w.Minutes += int(math.Round(*p.DurationSeconds / 60))
		case t.RuntimeMinutes != nil:
			w.Minutes += int(*t.RuntimeMinutes)
		}
	}
	return w, nil
}
