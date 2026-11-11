package core

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

type TitleState struct {
	Item        Item
	Watches     []Entry
	Resume      *Entry
	Rating      *Entry
	Listing     *Entry
	Lists       []InList
	Order       string
	Seasons     []SeasonState
	RewatchFrom *time.Time
}

type InList struct {
	ListID int64
	ItemID int64
}

type SeasonState struct {
	Number   int32
	Episodes []EpisodeState
}

type EpisodeState struct {
	Episode OrderedEpisode
	Watches []Entry
	Resume  *Entry
}

func Title(ctx context.Context, db *pgxpool.Pool, profileID int64, id, kind string) (TitleState, error) {
	q := store.New(db)
	t, err := titleByPath(ctx, q, id, kind)
	if err != nil {
		return TitleState{}, err
	}
	item, err := describe(ctx, q, 0, resolved{title: t})
	if err != nil {
		return TitleState{}, err
	}
	s := TitleState{Item: item}

	rating, err := q.TitleRating(ctx, store.TitleRatingParams{ProfileID: profileID, TitleID: t.ID})
	if err == nil {
		s.Rating = &Entry{ID: rating.ID, At: rating.RatedAt, Rating: rating.Rating}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return s, err
	}
	listing, err := q.TitleListing(ctx, store.TitleListingParams{ProfileID: profileID, TitleID: t.ID})
	if err == nil {
		s.Listing = &Entry{ID: listing.ID, At: listing.ListedAt}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return s, err
	}

	inLists, err := q.TitleLists(ctx, store.TitleListsParams{ProfileID: profileID, TitleID: t.ID})
	if err != nil {
		return s, err
	}
	for _, l := range inLists {
		s.Lists = append(s.Lists, InList{ListID: l.ListID, ItemID: l.ID})
	}

	watches, err := q.TitleWatches(ctx, store.TitleWatchesParams{ProfileID: profileID, TitleID: t.ID})
	if err != nil {
		return s, err
	}
	playback, err := q.TitlePlayback(ctx, store.TitlePlaybackParams{ProfileID: profileID, TitleID: t.ID})
	if err != nil {
		return s, err
	}
	plays := map[int64][]Entry{}
	resume := map[int64]*Entry{}
	for _, w := range watches {
		play := Entry{ID: w.ID, At: w.WatchedAt, Precision: w.Precision, Action: w.Action}
		if w.EpisodeID == nil {
			s.Watches = append(s.Watches, play)
		} else {
			plays[*w.EpisodeID] = append(plays[*w.EpisodeID], play)
		}
	}
	for _, p := range playback {
		point := &Entry{ID: p.ID, Progress: p.Progress}
		if p.EpisodeID == nil {
			s.Resume = point
		} else {
			resume[*p.EpisodeID] = point
		}
	}
	if t.Kind != "show" {
		return s, nil
	}

	s.Order, err = q.ShowOrder(ctx, store.ShowOrderParams{ProfileID: profileID, ShowID: t.ID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return s, err
	}
	resets, err := rewatches(ctx, q, profileID)
	if err != nil {
		return s, err
	}
	if reset, ok := resets[t.ID]; ok {
		s.RewatchFrom = &reset
	}
	episodes, err := showEpisodes(ctx, q, profileID, []int64{t.ID})
	if err != nil {
		return s, err
	}
	shown := episodes[t.ID]
	special := func(e OrderedEpisode) int {
		if e.Season == 0 {
			return 1
		}
		return 0
	}
	slices.SortStableFunc(shown, func(a, b OrderedEpisode) int { return cmp.Compare(special(a), special(b)) })
	for _, e := range shown {
		if len(s.Seasons) == 0 || s.Seasons[len(s.Seasons)-1].Number != e.Season {
			s.Seasons = append(s.Seasons, SeasonState{Number: e.Season})
		}
		season := &s.Seasons[len(s.Seasons)-1]
		season.Episodes = append(season.Episodes, EpisodeState{Episode: e, Watches: plays[e.ID], Resume: resume[e.ID]})
	}
	return s, nil
}

func titleByPath(ctx context.Context, q *store.Queries, id, kind string) (store.Title, error) {
	ids := IDs{Slug: id}
	if n, err := strconv.ParseInt(id, 10, 64); err == nil {
		ids = IDs{Tethr: n}
	} else if n, err := strconv.ParseInt(strings.TrimPrefix(id, "tmdb-"), 10, 64); err == nil && strings.HasPrefix(id, "tmdb-") {
		ids = IDs{TMDB: n}
	}
	kinds := []string{"movie", "show"}
	if kind != "" {
		kinds = []string{kind}
	}
	var t store.Title
	var err error
	for _, kind := range kinds {
		if t, err = findTitle(ctx, q, kind, ids); !errors.Is(err, ErrNotFound) {
			break
		}
	}
	return t, err
}

func Search(ctx context.Context, db *pgxpool.Pool, profileID int64, words, kind string) ([]Item, error) {
	words = strings.TrimSpace(words)
	if words == "" {
		return []Item{}, nil
	}
	q := store.New(db)
	params := store.SearchLibraryParams{ProfileID: profileID, Words: likeEscaper.Replace(words)}
	if kind != "" {
		params.Kind = &kind
	}
	ids, err := q.SearchLibrary(ctx, params)
	if err != nil {
		return nil, err
	}
	c, err := loadCatalogue(ctx, q, 0, ids, nil)
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(ids))
	for _, id := range ids {
		items = append(items, c.item(id, nil, nil))
	}
	return items, nil
}

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
