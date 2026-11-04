package core

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/tethrtv/tethr/internal/store"
)

const firstCatalogueID = 1_000_000_000

type IDs struct {
	Trakt int64
	Slug  string
	IMDB  string
	TMDB  int64
	TVDB  int64
}

func (ids IDs) empty() bool {
	return ids.Trakt < firstCatalogueID && ids.Slug == "" && ids.IMDB == "" && ids.TMDB == 0 && ids.TVDB == 0
}

type Ref struct {
	Kind   string
	IDs    IDs
	Show   IDs
	Season *int32
	Number *int32
}

type Item struct {
	Title   store.CatalogueTitlesRow
	Season  *int32
	Episode *store.Episode
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
	season  *int32
	episode *store.Episode
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
		if ref.Season == nil {
			return resolved{}, ErrNotFound
		}
		t, err := resolveTitle(ctx, q, "show", ref.Show, create)
		return resolved{title: t, season: ref.Season}, err
	case "episode":
		return resolveEpisode(ctx, q, ref, create)
	}
	return resolved{}, ErrNotFound
}

func resolveTitle(ctx context.Context, q *store.Queries, kind string, ids IDs, create bool) (store.Title, error) {
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
	lookups := []func() (store.Title, error){}
	if ids.Trakt >= firstCatalogueID {
		lookups = append(lookups, func() (store.Title, error) {
			return q.TitleByID(ctx, store.TitleByIDParams{ID: ids.Trakt, Kind: kind})
		})
	}
	if id := int32ID(ids.TMDB); id != nil {
		lookups = append(lookups, func() (store.Title, error) {
			return q.TitleByTMDB(ctx, store.TitleByTMDBParams{TmdbID: id, Kind: kind})
		})
	}
	if ids.IMDB != "" {
		lookups = append(lookups, func() (store.Title, error) {
			return q.TitleByExternalID(ctx, store.TitleByExternalIDParams{Provider: "imdb", Value: ids.IMDB, Kind: kind})
		})
	}
	if ids.TVDB > 0 {
		lookups = append(lookups, func() (store.Title, error) {
			return q.TitleByExternalID(ctx, store.TitleByExternalIDParams{Provider: "tvdb", Value: strconv.FormatInt(ids.TVDB, 10), Kind: kind})
		})
	}
	if id, err := strconv.ParseInt(strings.TrimPrefix(ids.Slug, "t-"), 10, 64); err == nil && strings.HasPrefix(ids.Slug, "t-") {
		lookups = append(lookups, func() (store.Title, error) {
			return q.TitleByID(ctx, store.TitleByIDParams{ID: id, Kind: kind})
		})
	} else if ids.Slug != "" {
		lookups = append(lookups, func() (store.Title, error) {
			return q.TitleBySlug(ctx, store.TitleBySlugParams{Slug: ids.Slug, Kind: kind})
		})
	}
	for _, lookup := range lookups {
		t, err := lookup()
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err == nil && t.MergedIntoID != nil {
			return q.TitleByID(ctx, store.TitleByIDParams{ID: *t.MergedIntoID, Kind: kind})
		}
		return t, err
	}
	return store.Title{}, ErrNotFound
}

func learnIDs(ctx context.Context, q *store.Queries, t store.Title, ids IDs) error {
	if t.TmdbID == nil && ids.TMDB != 0 {
		if _, err := q.TitleByTMDB(ctx, store.TitleByTMDBParams{TmdbID: int32ID(ids.TMDB), Kind: t.Kind}); errors.Is(err, pgx.ErrNoRows) {
			if err := q.SetTitleTMDB(ctx, store.SetTitleTMDBParams{ID: t.ID, TmdbID: int32ID(ids.TMDB)}); err != nil {
				return err
			}
		}
	}
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

func resolveEpisode(ctx context.Context, q *store.Queries, ref Ref, create bool) (resolved, error) {
	var e store.Episode
	var err error
	switch {
	case ref.IDs.Trakt >= firstCatalogueID:
		e, err = q.EpisodeByID(ctx, ref.IDs.Trakt)
	case ref.Season != nil && ref.Number != nil && !ref.Show.empty():
		var show store.Title
		show, err = resolveTitle(ctx, q, "show", ref.Show, create)
		if err != nil {
			return resolved{}, err
		}
		e, err = q.EpisodeByNumber(ctx, store.EpisodeByNumberParams{ShowID: show.ID, SeasonNumber: *ref.Season, EpisodeNumber: *ref.Number})
		if errors.Is(err, pgx.ErrNoRows) && create {
			e, err = q.CreateEpisode(ctx, store.CreateEpisodeParams{
				ShowID:        show.ID,
				SeasonNumber:  *ref.Season,
				EpisodeNumber: *ref.Number,
				TmdbEpisodeID: int32ID(ref.IDs.TMDB),
			})
		}
	case ref.IDs.TMDB != 0:
		e, err = q.EpisodeByTMDB(ctx, int32ID(ref.IDs.TMDB))
	default:
		return resolved{}, ErrNotFound
	}
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

func int32ID(id int64) *int32 {
	if id <= 0 || id > math.MaxInt32 {
		return nil
	}
	v := int32(id)
	return &v
}

type catalogue struct {
	titles   map[int64]store.CatalogueTitlesRow
	episodes map[int64]store.Episode
}

func loadCatalogue(ctx context.Context, q *store.Queries, titleIDs, episodeIDs []int64) (catalogue, error) {
	c := catalogue{titles: map[int64]store.CatalogueTitlesRow{}, episodes: map[int64]store.Episode{}}
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
		c.episodes[e.ID] = e
	}
	return c, nil
}

func (c catalogue) item(titleID int64, season *int32, episodeID *int64) Item {
	item := Item{Title: c.titles[titleID], Season: season}
	if episodeID != nil {
		e := c.episodes[*episodeID]
		item.Episode = &e
	}
	return item
}

func describe(ctx context.Context, q *store.Queries, r resolved) (Item, error) {
	c, err := loadCatalogue(ctx, q, []int64{r.title.ID}, nil)
	if err != nil {
		return Item{}, err
	}
	return Item{Title: c.titles[r.title.ID], Season: r.season, Episode: r.episode}, nil
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
		item, err := describe(ctx, q, r)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}
