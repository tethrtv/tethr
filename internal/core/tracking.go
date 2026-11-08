package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

var ErrProgressTooLow = errors.New("progress is under 1%, so there's nothing to save")

const (
	watchedProgress  = 80
	duplicateWindow  = 10 * time.Minute
	movieWatchWindow = 3 * time.Hour
	showWatchWindow  = 90 * time.Minute
	droppedStopGrace = 30 * time.Minute
)

type AlreadyScrobbled struct {
	WatchedAt time.Time
	ExpiresAt time.Time
}

func (e AlreadyScrobbled) Error() string {
	return fmt.Sprintf("already scrobbled at %s", e.WatchedAt.Format(time.RFC3339))
}

type ScrobbleResult struct {
	ID       int64
	Action   string
	Progress float64
	Item     Item
}

type Scrobble struct {
	Ref      Ref
	Progress float64
	Duration *float64
}

func ScrobbleStart(ctx context.Context, db *pgxpool.Pool, c Caller, s Scrobble) (ScrobbleResult, error) {
	return scrobble(ctx, db, c, s, func(q *store.Queries, r resolved) (ScrobbleResult, error) {
		window := movieWatchWindow
		if r.episode != nil {
			window = showWatchWindow
		}
		if s.Duration != nil && *s.Duration > 0 {
			window = time.Duration(*s.Duration * float64(time.Second))
		}
		if err := finishDropped(ctx, q, c.ProfileID, r); err != nil {
			return ScrobbleResult{}, err
		}
		left := time.Duration(float64(window) * (100 - s.Progress) / 100)
		err := q.SetWatching(ctx, store.SetWatchingParams{
			ProfileID: c.ProfileID,
			TitleID:   r.title.ID,
			EpisodeID: r.episodeID(),
			GrantID:   grantID(c),
			Progress:  s.Progress,
			ExpiresAt: time.Now().Add(left),
		})
		return ScrobbleResult{Action: "start"}, err
	})
}

func ScrobblePause(ctx context.Context, db *pgxpool.Pool, c Caller, s Scrobble) (ScrobbleResult, error) {
	return scrobble(ctx, db, c, s, func(q *store.Queries, r resolved) (ScrobbleResult, error) {
		return pause(ctx, q, c, s, r)
	})
}

func ScrobbleStop(ctx context.Context, db *pgxpool.Pool, c Caller, s Scrobble) (ScrobbleResult, error) {
	if s.Progress < 1 {
		return ScrobbleResult{}, ErrProgressTooLow
	}
	return scrobble(ctx, db, c, s, func(q *store.Queries, r resolved) (ScrobbleResult, error) {
		if s.Progress < watchedProgress {
			return pause(ctx, q, c, s, r)
		}
		recent, err := q.RecentWatchEvent(ctx, store.RecentWatchEventParams{
			ProfileID: c.ProfileID,
			TitleID:   r.title.ID,
			EpisodeID: r.episodeID(),
			WatchedAt: time.Now().Add(-duplicateWindow),
		})
		if err == nil {
			return ScrobbleResult{}, AlreadyScrobbled{WatchedAt: recent.WatchedAt, ExpiresAt: recent.WatchedAt.Add(duplicateWindow)}
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return ScrobbleResult{}, err
		}
		event, err := watched(ctx, q, c.ProfileID, grantID(c), r, time.Now(), "scrobble", s.Duration)
		if err != nil {
			return ScrobbleResult{}, err
		}
		return ScrobbleResult{ID: event.ID, Action: "scrobble"}, q.ClearWatching(ctx, c.ProfileID)
	})
}

// Some players move on to the next item without sending a stop for the last one.
func finishDropped(ctx context.Context, q *store.Queries, profileID int64, next resolved) error {
	w, err := q.Watching(ctx, profileID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if w.TitleID == next.title.ID && sameID(w.EpisodeID, next.episodeID()) {
		return nil
	}
	now := time.Now()
	reached := w.StartedAt
	if w.Progress < watchedProgress {
		reached = reached.Add(time.Duration(float64(w.ExpiresAt.Sub(w.StartedAt)) * (watchedProgress - w.Progress) / (100 - w.Progress)))
	}
	if now.Before(reached) || now.After(w.ExpiresAt.Add(droppedStopGrace)) {
		return nil
	}
	_, err = q.RecentWatchEvent(ctx, store.RecentWatchEventParams{
		ProfileID: profileID,
		TitleID:   w.TitleID,
		EpisodeID: w.EpisodeID,
		WatchedAt: now.Add(-duplicateWindow),
	})
	if err == nil {
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	var last resolved
	kind := "movie"
	if w.EpisodeID != nil {
		kind = "show"
		e, err := q.EpisodeByID(ctx, *w.EpisodeID)
		if err != nil {
			return err
		}
		last.episode = &e
	}
	if last.title, err = q.TitleByID(ctx, store.TitleByIDParams{ID: w.TitleID, Kind: kind}); err != nil {
		return err
	}
	at := now
	if w.ExpiresAt.Before(now) {
		at = w.ExpiresAt
	}
	_, err = watched(ctx, q, profileID, w.GrantID, last, at, "scrobble", nil)
	return err
}

func sameID(a, b *int64) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func scrobble(ctx context.Context, db *pgxpool.Pool, c Caller, s Scrobble, step func(*store.Queries, resolved) (ScrobbleResult, error)) (ScrobbleResult, error) {
	s.Progress = min(max(s.Progress, 0), 100)
	var result ScrobbleResult
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		if s.Ref.Kind != "movie" && s.Ref.Kind != "episode" {
			return ErrNotFound
		}
		r, err := resolve(ctx, q, s.Ref, true)
		if err != nil {
			return err
		}
		result, err = step(q, r)
		if err != nil {
			return err
		}
		result.Progress = s.Progress
		result.Item, err = describe(ctx, q, c.ProfileID, r)
		return err
	})
	return result, err
}

func pause(ctx context.Context, q *store.Queries, c Caller, s Scrobble, r resolved) (ScrobbleResult, error) {
	p, err := q.SavePlayback(ctx, store.SavePlaybackParams{
		ProfileID:       c.ProfileID,
		TitleID:         r.title.ID,
		EpisodeID:       r.episodeID(),
		Progress:        s.Progress,
		GrantID:         grantID(c),
		DurationSeconds: s.Duration,
	})
	if err != nil {
		return ScrobbleResult{}, err
	}
	if err := mark(ctx, q, c.ProfileID, area(r, "paused_at")); err != nil {
		return ScrobbleResult{}, err
	}
	return ScrobbleResult{ID: p.ID, Action: "pause"}, q.ClearWatching(ctx, c.ProfileID)
}

func watched(ctx context.Context, q *store.Queries, profileID int64, grant *int64, r resolved, at time.Time, action string, duration *float64) (store.WatchEvent, error) {
	event, err := q.CreateWatchEvent(ctx, store.CreateWatchEventParams{
		ProfileID:       profileID,
		TitleID:         r.title.ID,
		EpisodeID:       r.episodeID(),
		WatchedAt:       at,
		Action:          action,
		GrantID:         grant,
		DurationSeconds: duration,
	})
	if err != nil {
		return event, err
	}
	dropped, err := q.DeletePlaybackForItem(ctx, store.DeletePlaybackForItemParams{ProfileID: profileID, TitleID: r.title.ID, EpisodeID: r.episodeID()})
	if err != nil {
		return event, err
	}
	areas := []string{area(r, "watched_at")}
	if dropped > 0 {
		areas = append(areas, area(r, "paused_at"))
	}
	list, err := q.Watchlist(ctx, profileID)
	if err != nil {
		return event, err
	}
	unlisted, err := q.RemoveListItem(ctx, store.RemoveListItemParams{ListID: list.ID, TitleID: r.title.ID, EpisodeID: r.episodeID()})
	if err != nil {
		return event, err
	}
	if unlisted > 0 {
		if _, err := q.TouchList(ctx, list.ID); err != nil {
			return event, err
		}
		areas = append(areas, "watchlist.updated_at", area(r, "watchlisted_at"))
	}
	return event, mark(ctx, q, profileID, areas...)
}

func area(r resolved, field string) string {
	switch {
	case r.episode != nil:
		return "episodes." + field
	case r.season != nil:
		return "seasons." + field
	case r.title.Kind == "show":
		return "shows." + field
	}
	return "movies." + field
}

func mark(ctx context.Context, q *store.Queries, profileID int64, areas ...string) error {
	for _, a := range areas {
		if err := q.MarkActivity(ctx, store.MarkActivityParams{ProfileID: profileID, Area: a}); err != nil {
			return err
		}
	}
	return nil
}

func grantID(c Caller) *int64 {
	if c.GrantID == 0 {
		return nil
	}
	return &c.GrantID
}

type Page struct {
	Page  int
	Limit int
}

func (p Page) offset() int32 { return int32((p.Page - 1) * p.Limit) }

type Range struct {
	Start *time.Time
	End   *time.Time
}

type Entry struct {
	ID        int64
	At        time.Time
	Precision string
	Action    string
	Progress  float64
	Rank      int32
	Rating    int16
	Notes     *string
	Item      Item
}

type HistoryFilter struct {
	Kind  string
	ID    int64
	Range Range
}

func History(ctx context.Context, db *pgxpool.Pool, profileID int64, f HistoryFilter, p Page) ([]Entry, int64, error) {
	q := store.New(db)
	params := store.HistoryParams{
		ProfileID: profileID,
		Limit:     int32(p.Limit),
		Offset:    p.offset(),
		Kind:      optional(f.Kind),
		StartAt:   f.Range.Start,
		EndAt:     f.Range.End,
	}
	if f.ID != 0 {
		params.TitleID = &f.ID
	}
	events, err := q.History(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountHistory(ctx, store.CountHistoryParams{
		ProfileID: params.ProfileID,
		Kind:      params.Kind,
		TitleID:   params.TitleID,
		StartAt:   params.StartAt,
		EndAt:     params.EndAt,
	})
	if err != nil {
		return nil, 0, err
	}
	var titleIDs, episodeIDs []int64
	for _, e := range events {
		titleIDs = append(titleIDs, e.TitleID)
		if e.EpisodeID != nil {
			episodeIDs = append(episodeIDs, *e.EpisodeID)
		}
	}
	c, err := loadCatalogue(ctx, q, profileID, titleIDs, episodeIDs)
	if err != nil {
		return nil, 0, err
	}
	entries := make([]Entry, 0, len(events))
	for _, e := range events {
		entries = append(entries, Entry{ID: e.ID, At: e.WatchedAt, Precision: e.Precision, Action: e.Action, Item: c.item(e.TitleID, nil, e.EpisodeID)})
	}
	return entries, total, nil
}

var UnknownDate = time.Unix(0, 0).UTC()

type Watch struct {
	Ref Ref
	At  time.Time
}

type Counts struct {
	Movies   int
	Shows    int
	Seasons  int
	Episodes int
}

func (c *Counts) add(kind string) {
	switch kind {
	case "movie":
		c.Movies++
	case "show":
		c.Shows++
	case "season":
		c.Seasons++
	case "episode":
		c.Episodes++
	}
}

type SyncResult struct {
	Added    Counts
	Existing Counts
	Deleted  Counts
	NotFound []Ref
	NotIDs   []int64
	ListAt   time.Time
	Listed   int64
}

func AddHistory(ctx context.Context, db *pgxpool.Pool, c Caller, watches []Watch) (SyncResult, error) {
	var result SyncResult
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		for _, w := range watches {
			if w.Ref.Kind != "movie" && w.Ref.Kind != "episode" {
				result.NotFound = append(result.NotFound, w.Ref)
				continue
			}
			r, err := resolve(ctx, q, w.Ref, true)
			if errors.Is(err, ErrNotFound) {
				result.NotFound = append(result.NotFound, w.Ref)
				continue
			}
			if err != nil {
				return err
			}
			exists, err := q.WatchEventExists(ctx, store.WatchEventExistsParams{ProfileID: c.ProfileID, TitleID: r.title.ID, EpisodeID: r.episodeID(), WatchedAt: w.At})
			if err != nil {
				return err
			}
			if exists {
				continue
			}
			if _, err := watched(ctx, q, c.ProfileID, grantID(c), r, w.At, "watch", nil); err != nil {
				return err
			}
			result.Added.add(w.Ref.Kind)
		}
		return nil
	})
	return result, err
}

func RemoveHistory(ctx context.Context, db *pgxpool.Pool, profileID int64, ids []int64, refs []Ref) (SyncResult, error) {
	var result SyncResult
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		count := func(episodeID *int64) {
			if episodeID != nil {
				result.Deleted.Episodes++
			} else {
				result.Deleted.Movies++
			}
		}
		if len(ids) > 0 {
			rows, err := q.DeleteWatchEventsByID(ctx, store.DeleteWatchEventsByIDParams{ProfileID: profileID, Ids: ids})
			if err != nil {
				return err
			}
			gone := map[int64]bool{}
			for _, row := range rows {
				gone[row.ID] = true
				count(row.EpisodeID)
			}
			for _, id := range ids {
				if !gone[id] {
					result.NotIDs = append(result.NotIDs, id)
				}
			}
		}
		for _, ref := range refs {
			r, err := resolve(ctx, q, ref, false)
			if errors.Is(err, ErrNotFound) {
				result.NotFound = append(result.NotFound, ref)
				continue
			}
			if err != nil {
				return err
			}
			rows, err := q.DeleteWatchEventsForItem(ctx, store.DeleteWatchEventsForItemParams{
				ProfileID: profileID,
				TitleID:   r.title.ID,
				EpisodeID: r.episodeID(),
				Season:    r.season,
			})
			if err != nil {
				return err
			}
			for _, row := range rows {
				count(row.EpisodeID)
			}
		}
		if result.Deleted.Movies > 0 {
			if err := mark(ctx, q, profileID, "movies.watched_at"); err != nil {
				return err
			}
		}
		if result.Deleted.Episodes > 0 {
			return mark(ctx, q, profileID, "episodes.watched_at")
		}
		return nil
	})
	return result, err
}

func Playback(ctx context.Context, db *pgxpool.Pool, profileID int64, kind string, rng Range, p Page) ([]Entry, error) {
	q := store.New(db)
	rows, err := q.ListPlayback(ctx, store.ListPlaybackParams{
		ProfileID: profileID,
		Limit:     int32(p.Limit),
		Offset:    p.offset(),
		Kind:      optional(kind),
		StartAt:   rng.Start,
		EndAt:     rng.End,
	})
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
	c, err := loadCatalogue(ctx, q, profileID, titleIDs, episodeIDs)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, Entry{ID: row.ID, At: row.PausedAt, Progress: row.Progress, Item: c.item(row.TitleID, nil, row.EpisodeID)})
	}
	return entries, nil
}

func RemovePlayback(ctx context.Context, db *pgxpool.Pool, profileID, id int64) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		episodeID, err := q.DeletePlayback(ctx, store.DeletePlaybackParams{ID: id, ProfileID: profileID})
		if err != nil {
			return notFound(err)
		}
		if episodeID != nil {
			return mark(ctx, q, profileID, "episodes.paused_at")
		}
		return mark(ctx, q, profileID, "movies.paused_at")
	})
}

func Watchlist(ctx context.Context, db *pgxpool.Pool, profileID int64, kind string) ([]Entry, error) {
	q := store.New(db)
	list, err := q.Watchlist(ctx, profileID)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListItems(ctx, store.ListItemsParams{ListID: list.ID, Kind: optional(kind)})
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
	c, err := loadCatalogue(ctx, q, profileID, titleIDs, episodeIDs)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, Entry{ID: row.ID, At: row.ListedAt, Rank: row.Rank, Notes: row.Notes, Item: c.item(row.TitleID, row.SeasonNumber, row.EpisodeID)})
	}
	return entries, nil
}

type Listing struct {
	Ref   Ref
	At    *time.Time
	Notes string
}

func AddToWatchlist(ctx context.Context, db *pgxpool.Pool, profileID int64, items []Listing) (SyncResult, error) {
	return changeWatchlist(ctx, db, profileID, func(q *store.Queries, list store.List, result *SyncResult) error {
		for _, l := range items {
			r, err := resolve(ctx, q, l.Ref, true)
			if errors.Is(err, ErrNotFound) {
				result.NotFound = append(result.NotFound, l.Ref)
				continue
			}
			if err != nil {
				return err
			}
			_, err = q.AddListItem(ctx, store.AddListItemParams{
				ListID:       list.ID,
				TitleID:      r.title.ID,
				SeasonNumber: r.season,
				EpisodeID:    r.episodeID(),
				Notes:        optional(l.Notes),
				ListedAt:     l.At,
			})
			if errors.Is(err, pgx.ErrNoRows) {
				result.Existing.add(l.Ref.Kind)
				continue
			}
			if err != nil {
				return err
			}
			result.Added.add(l.Ref.Kind)
			if err := mark(ctx, q, profileID, area(r, "watchlisted_at")); err != nil {
				return err
			}
		}
		return nil
	})
}

func RemoveFromWatchlist(ctx context.Context, db *pgxpool.Pool, profileID int64, refs []Ref) (SyncResult, error) {
	return changeWatchlist(ctx, db, profileID, func(q *store.Queries, list store.List, result *SyncResult) error {
		for _, ref := range refs {
			r, err := resolve(ctx, q, ref, false)
			if errors.Is(err, ErrNotFound) {
				result.NotFound = append(result.NotFound, ref)
				continue
			}
			if err != nil {
				return err
			}
			n, err := q.RemoveListItem(ctx, store.RemoveListItemParams{ListID: list.ID, TitleID: r.title.ID, SeasonNumber: r.season, EpisodeID: r.episodeID()})
			if err != nil {
				return err
			}
			if n == 0 {
				result.NotFound = append(result.NotFound, ref)
				continue
			}
			result.Deleted.add(ref.Kind)
			if err := mark(ctx, q, profileID, area(r, "watchlisted_at")); err != nil {
				return err
			}
		}
		return nil
	})
}

func changeWatchlist(ctx context.Context, db *pgxpool.Pool, profileID int64, change func(*store.Queries, store.List, *SyncResult) error) (SyncResult, error) {
	var result SyncResult
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		list, err := q.Watchlist(ctx, profileID)
		if err != nil {
			return err
		}
		if err := change(q, list, &result); err != nil {
			return err
		}
		result.ListAt = list.UpdatedAt
		if result.Added != (Counts{}) || result.Deleted != (Counts{}) {
			if result.ListAt, err = q.TouchList(ctx, list.ID); err != nil {
				return err
			}
			if err := mark(ctx, q, profileID, "watchlist.updated_at"); err != nil {
				return err
			}
		}
		result.Listed, err = q.CountListItems(ctx, list.ID)
		return err
	})
	return result, err
}

type Activity struct {
	Since time.Time
	Marks map[string]time.Time
}

func LastActivities(ctx context.Context, db *pgxpool.Pool, profileID int64) (Activity, error) {
	q := store.New(db)
	profile, err := q.ProfileByID(ctx, profileID)
	if err != nil {
		return Activity{}, err
	}
	rows, err := q.ActivityMarks(ctx, profileID)
	if err != nil {
		return Activity{}, err
	}
	a := Activity{Since: profile.CreatedAt, Marks: map[string]time.Time{}}
	for _, row := range rows {
		a.Marks[row.Area] = row.At
	}
	return a, nil
}

func Profile(ctx context.Context, db *pgxpool.Pool, profileID int64) (store.Profile, error) {
	return store.New(db).ProfileByID(ctx, profileID)
}

func RemoveListing(ctx context.Context, db *pgxpool.Pool, profileID, id int64) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.DeleteListItem(ctx, store.DeleteListItemParams{ID: id, ProfileID: profileID})
		if err != nil {
			return notFound(err)
		}
		if _, err := q.TouchList(ctx, row.ListID); err != nil {
			return err
		}
		r := resolved{title: store.Title{Kind: row.Kind}, season: row.SeasonNumber}
		if row.EpisodeID != nil {
			r.episode = &store.Episode{ID: *row.EpisodeID}
		}
		return mark(ctx, q, profileID, "watchlist.updated_at", area(r, "watchlisted_at"))
	})
}

func AccountCaller(ctx context.Context, db *pgxpool.Pool, accountID int64) (Caller, error) {
	owner, err := store.New(db).OwnerProfile(ctx, accountID)
	return Caller{ProfileID: owner.ID}, err
}

func HouseholdCaller(ctx context.Context, db *pgxpool.Pool, accountID, profileID int64) (Caller, error) {
	profiles, err := store.New(db).HouseholdProfiles(ctx, accountID)
	if err != nil {
		return Caller{}, err
	}
	for _, p := range profiles {
		if p.ID == profileID {
			return Caller{ProfileID: p.ID}, nil
		}
	}
	return Caller{}, ErrNotFound
}
