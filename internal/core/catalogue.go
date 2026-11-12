package core

import (
	"context"
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/tethrtv/tethr/internal/store"
)

type IDs struct {
	Tethr int64
	Trakt int64
	Slug  string
	IMDB  string
	TMDB  int64
	TVDB  int64
}

func (ids IDs) empty() bool {
	return ids.Tethr <= 0 && ids.Trakt <= 0 && ids.Slug == "" && ids.IMDB == "" && ids.TMDB == 0 && ids.TVDB == 0
}

type Ref struct {
	Kind   string
	IDs    IDs
	Show   IDs
	Season *int32
	Number *int32
	// The profile whose chosen episode order Season and Number are in, or zero for aired order.
	OrderedFor int64
}

type Item struct {
	Title   store.CatalogueTitlesRow
	Season  *store.Season
	Episode *OrderedEpisode
}

func (i Item) Kind() string {
	switch {
	case i.Episode != nil:
		return "episode"
	case i.Season != nil:
		return "season"
	}
	return i.Title.Kind
}

type resolved struct {
	title   store.Title
	season  *store.Season
	episode *store.Episode
}

func (r resolved) seasonID() *int64 {
	if r.season == nil {
		return nil
	}
	return &r.season.ID
}

func (r resolved) seasonNumber() *int32 {
	if r.season == nil {
		return nil
	}
	return &r.season.SeasonNumber
}

func (r resolved) episodeID() *int64 {
	if r.episode == nil {
		return nil
	}
	return &r.episode.ID
}

func resolve(ctx context.Context, q *store.Queries, ref Ref, create bool) (resolved, error) {
	switch ref.Kind {
	case "movie", "show":
		t, err := resolveTitle(ctx, q, ref.Kind, ref.IDs, create)
		return resolved{title: t}, err
	case "season":
		return resolveSeason(ctx, q, ref, create)
	case "episode":
		return resolveEpisode(ctx, q, ref, create)
	}
	return resolved{}, ErrNotFound
}

func resolveTitle(ctx context.Context, q *store.Queries, kind string, ids IDs, create bool) (store.Title, error) {
	ids = ids.clean()
	t, err := findTitle(ctx, q, kind, ids)
	if err == nil {
		return t, learnIDs(ctx, q, t, ids)
	}
	if !errors.Is(err, ErrNotFound) || !create || (ids.TMDB == 0 && ids.IMDB == "" && ids.TVDB == 0) {
		return store.Title{}, err
	}
	t, err = q.CreateTitle(ctx, store.CreateTitleParams{Kind: kind, TmdbID: int32ID(ids.TMDB)})
	if err != nil {
		return store.Title{}, err
	}
	return t, learnIDs(ctx, q, t, ids)
}

func findTitle(ctx context.Context, q *store.Queries, kind string, ids IDs) (store.Title, error) {
	type lookup struct {
		byOutsideID bool
		find        func() (store.Title, error)
	}
	var lookups []lookup
	if ids.Tethr > 0 {
		lookups = append(lookups, lookup{false, func() (store.Title, error) {
			return q.TitleByID(ctx, store.TitleByIDParams{ID: ids.Tethr, Kind: kind})
		}})
	}
	if id := int32ID(ids.TMDB); id != nil {
		lookups = append(lookups, lookup{false, func() (store.Title, error) {
			return q.TitleByTMDB(ctx, store.TitleByTMDBParams{TmdbID: id, Kind: kind})
		}})
	}
	byOutsideID := func(provider, value string) {
		lookups = append(lookups, lookup{true, func() (store.Title, error) {
			return q.TitleByExternalID(ctx, store.TitleByExternalIDParams{Provider: provider, Value: value, Kind: kind})
		}})
	}
	if ids.IMDB != "" {
		byOutsideID("imdb", ids.IMDB)
	}
	if ids.TVDB > 0 {
		byOutsideID("tvdb", strconv.FormatInt(ids.TVDB, 10))
	}
	if ids.Trakt > 0 && ids.Trakt <= maxTraktID {
		byOutsideID(traktProvider(kind), strconv.FormatInt(ids.Trakt, 10))
	}
	if id, err := strconv.ParseInt(strings.TrimPrefix(ids.Slug, "t-"), 10, 64); err == nil && strings.HasPrefix(ids.Slug, "t-") {
		lookups = append(lookups, lookup{false, func() (store.Title, error) {
			return q.TitleByID(ctx, store.TitleByIDParams{ID: id, Kind: kind})
		}})
	} else if ids.Slug != "" {
		lookups = append(lookups, lookup{false, func() (store.Title, error) {
			return q.TitleBySlug(ctx, store.TitleBySlugParams{Slug: ids.Slug, Kind: kind})
		}})
	}
	for _, l := range lookups {
		t, err := l.find()
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err == nil && t.MergedIntoID != nil {
			t, err = q.TitleByID(ctx, store.TitleByIDParams{ID: *t.MergedIntoID, Kind: kind})
		}
		if err != nil {
			return store.Title{}, err
		}
		if l.byOutsideID && ids.TMDB != 0 && t.TmdbID != nil && int64(*t.TmdbID) != ids.TMDB {
			continue
		}
		return t, nil
	}
	return store.Title{}, ErrNotFound
}

const maxTraktID = 999_999_999

func traktProvider(kind string) string {
	if kind == "show" {
		return "trakt_show"
	}
	return "trakt_film"
}

var imdbID = regexp.MustCompile(`^tt[0-9]{7,9}$`)

func (ids IDs) clean() IDs {
	if !imdbID.MatchString(ids.IMDB) {
		ids.IMDB = ""
	}
	if ids.TMDB < 0 || ids.TMDB > math.MaxInt32 {
		ids.TMDB = 0
	}
	if ids.TVDB < 0 || ids.TVDB > math.MaxInt32 {
		ids.TVDB = 0
	}
	if ids.Trakt < 0 || ids.Trakt > maxTraktID {
		ids.Trakt = 0
	}
	return ids
}

func fillTMDB(ctx context.Context, q *store.Queries, t store.Title, tmdb int32) error {
	if t.TmdbID != nil || tmdb == 0 {
		return nil
	}
	_, err := q.TitleByTMDB(ctx, store.TitleByTMDBParams{TmdbID: &tmdb, Kind: t.Kind})
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	return q.SetTitleTMDB(ctx, store.SetTitleTMDBParams{ID: t.ID, TmdbID: &tmdb})
}

func learnIDs(ctx context.Context, q *store.Queries, t store.Title, ids IDs) error {
	if ids.IMDB != "" {
		if err := q.AddExternalID(ctx, store.AddExternalIDParams{TitleID: t.ID, Provider: "imdb", Value: ids.IMDB}); err != nil {
			return err
		}
	}
	if ids.TVDB > 0 {
		if err := q.AddExternalID(ctx, store.AddExternalIDParams{TitleID: t.ID, Provider: "tvdb", Value: strconv.FormatInt(ids.TVDB, 10)}); err != nil {
			return err
		}
	}
	return nil
}

func resolveSeason(ctx context.Context, q *store.Queries, ref Ref, create bool) (resolved, error) {
	if ref.IDs.Tethr > 0 {
		s, err := q.SeasonByID(ctx, ref.IDs.Tethr)
		switch {
		case err == nil:
			ref.Show, ref.Season = IDs{Tethr: s.ShowID}, &s.SeasonNumber
		case !errors.Is(err, pgx.ErrNoRows):
			return resolved{}, err
		}
	}
	if ref.Season == nil || *ref.Season < 0 {
		return resolved{}, ErrNotFound
	}
	show, err := resolveTitle(ctx, q, "show", ref.Show, create)
	if err != nil {
		return resolved{}, err
	}
	s, err := q.SeasonByNumber(ctx, store.SeasonByNumberParams{ShowID: show.ID, SeasonNumber: *ref.Season})
	if errors.Is(err, pgx.ErrNoRows) && create {
		s, err = q.CreateSeason(ctx, store.CreateSeasonParams{ShowID: show.ID, SeasonNumber: *ref.Season})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return resolved{}, ErrNotFound
	}
	if err != nil {
		return resolved{}, err
	}
	return resolved{title: show, season: &s}, nil
}

func resolveEpisode(ctx context.Context, q *store.Queries, ref Ref, create bool) (resolved, error) {
	e, err := findEpisode(ctx, q, ref, create)
	if errors.Is(err, pgx.ErrNoRows) {
		return resolved{}, ErrNotFound
	}
	if err != nil {
		return resolved{}, err
	}
	show, err := q.TitleByID(ctx, store.TitleByIDParams{ID: e.ShowID, Kind: "show"})
	if err != nil {
		return resolved{}, err
	}
	return resolved{title: show, episode: &e}, nil
}

func findEpisode(ctx context.Context, q *store.Queries, ref Ref, create bool) (store.Episode, error) {
	if ref.IDs.Tethr > 0 {
		return q.EpisodeByID(ctx, ref.IDs.Tethr)
	}
	if ref.OrderedFor != 0 && ref.IDs.TMDB != 0 {
		e, err := q.EpisodeByTMDB(ctx, int32ID(ref.IDs.TMDB))
		if !errors.Is(err, pgx.ErrNoRows) {
			return e, err
		}
	}
	if ref.Season == nil || ref.Number == nil || ref.Show.empty() {
		if ref.IDs.TMDB == 0 {
			return store.Episode{}, ErrNotFound
		}
		return q.EpisodeByTMDB(ctx, int32ID(ref.IDs.TMDB))
	}
	show, err := resolveTitle(ctx, q, "show", ref.Show, create)
	if err != nil {
		return store.Episode{}, err
	}
	if ref.OrderedFor != 0 {
		_, err := q.ShowOrder(ctx, store.ShowOrderParams{ProfileID: ref.OrderedFor, ShowID: show.ID})
		if err == nil {
			e, err := q.EpisodeInOrder(ctx, store.EpisodeInOrderParams{ProfileID: ref.OrderedFor, ShowID: show.ID, SeasonNumber: *ref.Season, EpisodeNumber: *ref.Number})
			return e.Episode, err
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return store.Episode{}, err
		}
	}
	e, err := q.EpisodeByNumber(ctx, store.EpisodeByNumberParams{ShowID: show.ID, SeasonNumber: *ref.Season, EpisodeNumber: *ref.Number})
	if !errors.Is(err, pgx.ErrNoRows) || !create {
		return e, err
	}
	e, err = q.CreateEpisode(ctx, store.CreateEpisodeParams{
		ShowID:        show.ID,
		SeasonNumber:  *ref.Season,
		EpisodeNumber: *ref.Number,
		TmdbEpisodeID: int32ID(ref.IDs.TMDB),
	})
	if err != nil {
		return e, err
	}
	return e, q.AddShowSeasons(ctx, show.ID)
}

func int32ID(id int64) *int32 {
	if id <= 0 || id > math.MaxInt32 {
		return nil
	}
	v := int32(id)
	return &v
}

type catalogue struct {
	titles   map[int64]store.CatalogueTitlesRow
	episodes map[int64]OrderedEpisode
}

func loadCatalogue(ctx context.Context, q *store.Queries, profileID int64, titleIDs, episodeIDs []int64) (catalogue, error) {
	c := catalogue{titles: map[int64]store.CatalogueTitlesRow{}, episodes: map[int64]OrderedEpisode{}}
	titles, err := q.CatalogueTitles(ctx, titleIDs)
	if err != nil {
		return c, err
	}
	for _, t := range titles {
		c.titles[t.ID] = t
	}
	episodes, err := q.CatalogueEpisodes(ctx, episodeIDs)
	if err != nil {
		return c, err
	}
	for _, e := range episodes {
		c.episodes[e.ID] = OrderedEpisode{Episode: e, Season: e.SeasonNumber, Number: e.EpisodeNumber}
	}
	if profileID == 0 || len(episodes) == 0 {
		return c, nil
	}
	ordered, err := q.EpisodeOrderNumbers(ctx, store.EpisodeOrderNumbersParams{ProfileID: profileID, EpisodeIds: episodeIDs})
	if err != nil {
		return c, err
	}
	for _, o := range ordered {
		e := c.episodes[o.ID]
		e.Season, e.Number = o.SeasonNumber, o.EpisodeNumber
		c.episodes[o.ID] = e
	}
	return c, nil
}

func seasonOf(showID int64, id *int64, number *int32) *store.Season {
	if id == nil || number == nil {
		return nil
	}
	return &store.Season{ID: *id, ShowID: showID, SeasonNumber: *number}
}

func (c catalogue) item(titleID int64, season *store.Season, episodeID *int64) Item {
	item := Item{Title: c.titles[titleID], Season: season}
	if episodeID != nil {
		e := c.episodes[*episodeID]
		item.Episode = &e
	}
	return item
}

func describe(ctx context.Context, q *store.Queries, profileID int64, r resolved) (Item, error) {
	var episodeIDs []int64
	if r.episode != nil {
		episodeIDs = []int64{r.episode.ID}
	}
	c, err := loadCatalogue(ctx, q, profileID, []int64{r.title.ID}, episodeIDs)
	if err != nil {
		return Item{}, err
	}
	return c.item(r.title.ID, r.season, r.episodeID()), nil
}

func LookUp(ctx context.Context, db store.DBTX, kind string, ids IDs, create bool) ([]Item, error) {
	q := store.New(db)
	kinds := []string{"movie", "show"}
	if kind != "" {
		kinds = []string{kind}
	}
	var items []Item
	for _, k := range kinds {
		r, err := resolve(ctx, q, Ref{Kind: k, IDs: ids}, create && kind != "")
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		item, err := describe(ctx, q, 0, r)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}
