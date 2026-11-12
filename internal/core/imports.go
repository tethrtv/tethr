package core

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

var (
	ErrImportBusy  = errors.New("an import is already running for this profile")
	ErrEmptyImport = errors.New("there's nothing to import in that file")
)

const maxUnmatched = 100

type ImportItem struct {
	Ref       Ref
	Label     string
	At        *time.Time
	Precision string
	Action    string
	Notes     string
	Progress  float64
	Rating    int16
	Section   string
}

type ImportFile struct {
	History   []ImportItem
	Watchlist []ImportItem
	Playback  []ImportItem
	Ratings   []ImportItem
	Hidden    []ImportItem
}

type ImportCount struct {
	Total     int `json:"total"`
	Added     int `json:"added"`
	Skipped   int `json:"skipped"`
	Unmatched int `json:"unmatched"`
}

type ImportSummary struct {
	History   ImportCount `json:"history"`
	Watchlist ImportCount `json:"watchlist"`
	Playback  ImportCount `json:"playback"`
	Ratings   ImportCount `json:"ratings"`
	Hidden    ImportCount `json:"hidden"`
	Unmatched []string    `json:"unmatched"`
}

func (s *ImportSummary) unmatched(c *ImportCount, label string) {
	c.Unmatched++
	if len(s.Unmatched) < maxUnmatched {
		s.Unmatched = append(s.Unmatched, label)
	}
}

type Import struct {
	ID        int64
	Source    string
	Filename  string
	Status    string
	Summary   ImportSummary
	CreatedAt time.Time
	AppliedAt *time.Time
	UndoneAt  *time.Time
}

func toImport(id int64, source, filename, status string, summary []byte, created time.Time, applied, undone *time.Time) (Import, error) {
	i := Import{ID: id, Source: source, Filename: filename, Status: status, CreatedAt: created, AppliedAt: applied, UndoneAt: undone}
	return i, json.Unmarshal(summary, &i.Summary)
}

func CreateImport(ctx context.Context, db *pgxpool.Pool, profileID int64, source, filename string, f ImportFile) (Import, error) {
	summary := ImportSummary{Unmatched: []string{}}
	keep := func(items []ImportItem, c *ImportCount) []ImportItem {
		c.Total = len(items)
		var out []ImportItem
		for _, item := range items {
			if !matchable(item.Ref) {
				summary.unmatched(c, item.Label)
				continue
			}
			out = append(out, item)
		}
		return out
	}
	f.History = keep(f.History, &summary.History)
	f.Watchlist = keep(f.Watchlist, &summary.Watchlist)
	f.Playback = keep(f.Playback, &summary.Playback)
	f.Ratings = keep(f.Ratings, &summary.Ratings)
	f.Hidden = keep(f.Hidden, &summary.Hidden)
	if len(f.History)+len(f.Watchlist)+len(f.Playback)+len(f.Ratings)+len(f.Hidden) == 0 {
		return Import{}, ErrEmptyImport
	}
	payload, err := json.Marshal(f)
	if err != nil {
		return Import{}, err
	}
	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		return Import{}, err
	}
	var row store.CreateImportRow
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		if _, err := q.LockProfile(ctx, profileID); err != nil {
			return err
		}
		busy, err := q.ImportInProgress(ctx, profileID)
		if err != nil {
			return err
		}
		if busy {
			return ErrImportBusy
		}
		if err := q.DropParsedImports(ctx, profileID); err != nil {
			return err
		}
		row, err = q.CreateImport(ctx, store.CreateImportParams{
			ProfileID: profileID,
			Source:    source,
			Filename:  filename,
			Payload:   payload,
			Summary:   summaryJSON,
		})
		return err
	})
	if err != nil {
		return Import{}, err
	}
	return toImport(row.ID, row.Source, row.Filename, row.Status, row.Summary, row.CreatedAt, row.AppliedAt, row.UndoneAt)
}

func matchable(r Ref) bool {
	switch r.Kind {
	case "movie", "show":
		return !r.IDs.empty()
	case "season":
		return !r.Show.empty() && r.Season != nil
	case "episode":
		return !r.Show.empty() && r.Season != nil && r.Number != nil
	}
	return false
}

func Imports(ctx context.Context, db *pgxpool.Pool, profileID int64) ([]Import, error) {
	rows, err := store.New(db).ProfileImports(ctx, profileID)
	if err != nil {
		return nil, err
	}
	out := make([]Import, 0, len(rows))
	for _, r := range rows {
		i, err := toImport(r.ID, r.Source, r.Filename, r.Status, r.Summary, r.CreatedAt, r.AppliedAt, r.UndoneAt)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, nil
}

func ImportByID(ctx context.Context, db *pgxpool.Pool, profileID, id int64) (Import, error) {
	r, err := store.New(db).ImportByID(ctx, store.ImportByIDParams{ID: id, ProfileID: profileID})
	if err != nil {
		return Import{}, notFound(err)
	}
	return toImport(r.ID, r.Source, r.Filename, r.Status, r.Summary, r.CreatedAt, r.AppliedAt, r.UndoneAt)
}

func QueueImport(ctx context.Context, db *pgxpool.Pool, profileID, id int64) (Import, error) {
	n, err := store.New(db).QueueImport(ctx, store.QueueImportParams{ID: id, ProfileID: profileID})
	if err != nil {
		return Import{}, err
	}
	if n == 0 {
		return Import{}, ErrNotFound
	}
	return ImportByID(ctx, db, profileID, id)
}

func ExpireImports(ctx context.Context, db *pgxpool.Pool) error {
	return store.New(db).ExpireImports(ctx)
}

func ApplyNextImport(ctx context.Context, db *pgxpool.Pool) (bool, []IDConflict, error) {
	var found bool
	var failed *store.Import
	var conflicts []IDConflict
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		conflicts = nil
		q := store.New(tx)
		imp, err := q.NextQueuedImport(ctx)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		found = true
		var f ImportFile
		if err := json.Unmarshal(imp.Payload, &f); err != nil {
			failed = &imp
			return err
		}
		var summary ImportSummary
		if err := json.Unmarshal(imp.Summary, &summary); err != nil {
			failed = &imp
			return err
		}
		im, err := applyImport(ctx, q, imp, f, &summary)
		if err != nil {
			failed = &imp
			return err
		}
		conflicts = im.conflicts
		summaryJSON, err := json.Marshal(summary)
		if err != nil {
			return err
		}
		return q.FinishImport(ctx, store.FinishImportParams{ID: imp.ID, Status: "applied", Summary: summaryJSON})
	})
	if failed != nil {
		if ferr := store.New(db).FinishImport(ctx, store.FinishImportParams{ID: failed.ID, Status: "failed", Summary: failed.Summary}); ferr != nil {
			return found, nil, errors.Join(err, ferr)
		}
	}
	if err != nil {
		return found, nil, err
	}
	return found, conflicts, nil
}

type importer struct {
	ctx       context.Context
	q         *store.Queries
	imp       store.Import
	summary   *ImportSummary
	resolved  map[string]*resolved
	areas     map[string]bool
	conflicts []IDConflict
}

func refKey(r Ref) string {
	deref := func(p *int32) int32 {
		if p == nil {
			return -1
		}
		return *p
	}
	return fmt.Sprintf("%s|%v|%v|%d|%d", r.Kind, r.IDs, r.Show, deref(r.Season), deref(r.Number))
}

func (im *importer) resolve(ref Ref) (*resolved, error) {
	key := refKey(ref)
	if r, ok := im.resolved[key]; ok {
		return r, nil
	}
	r, err := resolve(im.ctx, im.q, ref, true)
	if errors.Is(err, ErrNotFound) {
		im.resolved[key] = nil
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := im.learnTraktID(ref, r); err != nil {
		return nil, err
	}
	im.resolved[key] = &r
	return &r, nil
}

func (im *importer) learnTraktID(ref Ref, r resolved) error {
	ids := ref.Show
	if ref.Kind == "movie" || ref.Kind == "show" {
		ids = ref.IDs
	}
	ids = ids.clean()
	if ids.Trakt == 0 {
		return nil
	}
	t := r.title
	provider, value := traktProvider(t.Kind), strconv.FormatInt(ids.Trakt, 10)
	conflict := func(other int64, outcome string) error {
		im.conflicts = append(im.conflicts, IDConflict{Import: im.imp.ID, Title: t.ID, Provider: provider, Value: value, Other: other, Outcome: outcome})
		return nil
	}

	rows, err := im.q.TitleExternalIDs(im.ctx, t.ID)
	if err != nil {
		return err
	}
	held := map[string][]string{}
	for _, h := range rows {
		held[h.Provider] = append(held[h.Provider], h.Value)
	}
	if slices.Contains(held[provider], value) {
		return nil
	}
	disagrees := func(provider, value string) bool {
		return value != "" && len(held[provider]) > 0 && !slices.Contains(held[provider], value)
	}
	tvdb := ""
	if ids.TVDB != 0 {
		tvdb = strconv.FormatInt(ids.TVDB, 10)
	}
	switch {
	case ids.TMDB != 0 && t.TmdbID != nil && int64(*t.TmdbID) != ids.TMDB, disagrees("imdb", ids.IMDB), disagrees("tvdb", tvdb):
		return conflict(0, "ignored an import's, its other ids disagree")
	case len(held[provider]) > 0:
		return conflict(0, "ignored an import's, the title has one")
	}
	owner, err := im.q.ExternalIDOwner(im.ctx, store.ExternalIDOwnerParams{Provider: provider, Value: value})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return err
	default:
		return conflict(owner.TitleID, "ignored an import's, another title has it")
	}
	return im.q.AddExternalID(im.ctx, store.AddExternalIDParams{TitleID: t.ID, Provider: provider, Value: value})
}

func (im *importer) each(items []ImportItem, c *ImportCount, kinds []string, field func(ImportItem) string, write func(ImportItem, resolved) (int64, error)) error {
	for _, item := range items {
		if !slices.Contains(kinds, item.Ref.Kind) {
			im.summary.unmatched(c, item.Label)
			continue
		}
		r, err := im.resolve(item.Ref)
		if err != nil {
			return err
		}
		if r == nil {
			im.summary.unmatched(c, item.Label)
			continue
		}
		n, err := write(item, *r)
		if err != nil {
			return err
		}
		if n == 0 {
			c.Skipped++
			continue
		}
		c.Added++
		im.areas[area(*r, field(item))] = true
	}
	return nil
}

func field(name string) func(ImportItem) string {
	return func(ImportItem) string { return name }
}

func hiddenField(item ImportItem) string {
	if item.Section == "dropped" {
		return "dropped_at"
	}
	return "hidden_at"
}

func applyImport(ctx context.Context, q *store.Queries, imp store.Import, f ImportFile, summary *ImportSummary) (*importer, error) {
	im := &importer{ctx: ctx, q: q, imp: imp, summary: summary, resolved: map[string]*resolved{}, areas: map[string]bool{}}
	profileID, importID := imp.ProfileID, &imp.ID

	err := im.each(f.History, &summary.History, []string{"movie", "episode"}, field("watched_at"), func(item ImportItem, r resolved) (int64, error) {
		at, precision := UnknownDate, "none"
		if item.At != nil {
			at, precision = *item.At, cmp.Or(item.Precision, "time")
		}
		action := item.Action
		if action != "scrobble" && action != "checkin" {
			action = "watch"
		}
		return q.ImportWatchEvent(ctx, store.ImportWatchEventParams{
			ProfileID: profileID,
			TitleID:   r.title.ID,
			EpisodeID: r.episodeID(),
			WatchedAt: at,
			Precision: precision,
			Action:    action,
			ImportID:  importID,
		})
	})
	if err != nil {
		return nil, err
	}

	list, err := q.Watchlist(ctx, profileID)
	if err != nil {
		return nil, err
	}
	err = im.each(f.Watchlist, &summary.Watchlist, []string{"movie", "show", "season", "episode"}, field("watchlisted_at"), func(item ImportItem, r resolved) (int64, error) {
		at := time.Now()
		if item.At != nil {
			at = *item.At
		}
		return q.ImportListItem(ctx, store.ImportListItemParams{
			ListID:    list.ID,
			TitleID:   r.title.ID,
			SeasonID:  r.seasonID(),
			EpisodeID: r.episodeID(),
			Notes:     optional(item.Notes),
			ListedAt:  at,
			ImportID:  importID,
		})
	})
	if err != nil {
		return nil, err
	}

	err = im.each(f.Playback, &summary.Playback, []string{"movie", "episode"}, field("paused_at"), func(item ImportItem, r resolved) (int64, error) {
		if item.At == nil || item.Progress < 1 {
			return 0, nil
		}
		return q.ImportPlayback(ctx, store.ImportPlaybackParams{
			ProfileID: profileID,
			TitleID:   r.title.ID,
			EpisodeID: r.episodeID(),
			Progress:  min(item.Progress, 100),
			PausedAt:  *item.At,
			ImportID:  importID,
		})
	})
	if err != nil {
		return nil, err
	}

	err = im.each(f.Ratings, &summary.Ratings, []string{"movie", "show", "season", "episode"}, field("rated_at"), func(item ImportItem, r resolved) (int64, error) {
		if item.Rating < 1 || item.Rating > 10 {
			return 0, nil
		}
		at := time.Now()
		if item.At != nil {
			at = *item.At
		}
		return q.ImportRating(ctx, store.ImportRatingParams{
			ProfileID: profileID,
			TitleID:   r.title.ID,
			SeasonID:  r.seasonID(),
			EpisodeID: r.episodeID(),
			Rating:    item.Rating,
			RatedAt:   at,
			ImportID:  importID,
		})
	})
	if err != nil {
		return nil, err
	}

	err = im.each(f.Hidden, &summary.Hidden, []string{"movie", "show", "season"}, hiddenField, func(item ImportItem, r resolved) (int64, error) {
		if !slices.Contains(HiddenSections, item.Section) || item.Section == "progress_watched_reset" && item.At == nil {
			return 0, nil
		}
		at := time.Now()
		if item.At != nil {
			at = *item.At
		}
		return q.ImportHiddenItem(ctx, store.ImportHiddenItemParams{
			ProfileID: profileID,
			Section:   item.Section,
			TitleID:   r.title.ID,
			SeasonID:  r.seasonID(),
			HiddenAt:  at,
			ImportID:  importID,
		})
	})
	if err != nil {
		return nil, err
	}

	if summary.Watchlist.Added > 0 {
		if _, err := q.TouchList(ctx, list.ID); err != nil {
			return nil, err
		}
		im.areas["watchlist.updated_at"] = true
	}
	areas := make([]string, 0, len(im.areas))
	for a := range im.areas {
		areas = append(areas, a)
	}
	slices.Sort(areas)
	return im, mark(ctx, q, profileID, areas...)
}

func UndoImport(ctx context.Context, db *pgxpool.Pool, profileID, id int64) (Import, error) {
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		n, err := q.MarkImportUndone(ctx, store.MarkImportUndoneParams{ID: id, ProfileID: profileID})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		importID := &id
		if _, err := q.UndoImportedWatchEvents(ctx, importID); err != nil {
			return err
		}
		if _, err := q.UndoImportedPlayback(ctx, importID); err != nil {
			return err
		}
		listed, err := q.UndoImportedListItems(ctx, importID)
		if err != nil {
			return err
		}
		if _, err := q.UndoImportedRatings(ctx, importID); err != nil {
			return err
		}
		if _, err := q.UndoImportedHiddenItems(ctx, importID); err != nil {
			return err
		}
		if listed > 0 {
			list, err := q.Watchlist(ctx, profileID)
			if err != nil {
				return err
			}
			if _, err := q.TouchList(ctx, list.ID); err != nil {
				return err
			}
		}
		return mark(ctx, q, profileID,
			"movies.watched_at", "episodes.watched_at", "movies.paused_at", "episodes.paused_at",
			"watchlist.updated_at", "movies.watchlisted_at", "shows.watchlisted_at", "seasons.watchlisted_at", "episodes.watchlisted_at",
			"movies.rated_at", "shows.rated_at", "seasons.rated_at", "episodes.rated_at",
			"movies.hidden_at", "shows.hidden_at", "seasons.hidden_at", "shows.dropped_at")
	})
	if err != nil {
		return Import{}, err
	}
	return ImportByID(ctx, db, profileID, id)
}
