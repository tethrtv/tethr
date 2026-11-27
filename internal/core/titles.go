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
	Facts       TitleFacts
	Watches     []Entry
	Resume      *Entry
	Rating      *Entry
	Listing     *Entry
	Lists       []InList
	Order       string
	Seasons     []SeasonState
	RewatchFrom *time.Time
}

type TitleFacts struct {
	Genres, Countries, Languages, TypeFlags []string
	Started, Ended                          *string
	Studios, Networks                       []Company
}

type Company struct {
	QID, Name string
}

type InList struct {
	ListID int64
	ItemID int64
}

type SeasonState struct {
	ID       int64
	Number   int32
	Rating   *Entry
	Episodes []EpisodeState
}

type EpisodeState struct {
	Episode OrderedEpisode
	Watches []Entry
	Resume  *Entry
	Rating  *Entry
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
	s := TitleState{Item: item, Facts: TitleFacts{
		Genres: t.Genres, Countries: t.Countries, Languages: t.Languages, TypeFlags: t.TypeFlags, Started: t.Started, Ended: t.Ended,
	}}
	if s.Facts.Studios, s.Facts.Networks, err = companies(ctx, q, t.Studios, t.Networks); err != nil {
		return s, err
	}

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
	byRule, err := ruleListsWith(ctx, q, profileID, t.ID)
	if err != nil {
		return s, err
	}
	s.Lists = append(s.Lists, byRule...)
	slices.SortFunc(s.Lists, func(a, b InList) int { return cmp.Compare(a.ListID, b.ListID) })

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
	parts, err := q.ShowPartRatings(ctx, store.ShowPartRatingsParams{ProfileID: profileID, TitleID: t.ID})
	if err != nil {
		return s, err
	}
	episodeRatings, seasonRatings := map[int64]*Entry{}, map[int64]*Entry{}
	for _, r := range parts {
		entry := &Entry{ID: r.ID, At: r.RatedAt, Rating: r.Rating}
		if r.EpisodeID != nil {
			episodeRatings[*r.EpisodeID] = entry
		} else if r.SeasonID != nil {
			seasonRatings[*r.SeasonID] = entry
		}
	}
	for _, e := range shown {
		if len(s.Seasons) == 0 || s.Seasons[len(s.Seasons)-1].Number != e.Season {
			s.Seasons = append(s.Seasons, SeasonState{Number: e.Season})
		}
		season := &s.Seasons[len(s.Seasons)-1]
		season.Episodes = append(season.Episodes, EpisodeState{Episode: e, Watches: plays[e.ID], Resume: resume[e.ID], Rating: episodeRatings[e.ID]})
	}
	if s.Order != "" {
		return s, nil
	}
	seasons, err := q.ShowSeasons(ctx, t.ID)
	if err != nil {
		return s, err
	}
	ids := map[int32]int64{}
	for _, season := range seasons {
		ids[season.SeasonNumber] = season.ID
	}
	for i := range s.Seasons {
		s.Seasons[i].ID = ids[s.Seasons[i].Number]
		s.Seasons[i].Rating = seasonRatings[s.Seasons[i].ID]
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

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func companies(ctx context.Context, q *store.Queries, studios, networks []string) ([]Company, []Company, error) {
	if len(studios)+len(networks) == 0 {
		return nil, nil, nil
	}
	labels, err := q.ClassLabels(ctx, slices.Concat(studios, networks))
	if err != nil {
		return nil, nil, err
	}
	names := map[string]string{}
	for _, l := range labels {
		names[l.Qid] = l.Label
	}
	named := func(qids []string) []Company {
		var out []Company
		for _, qid := range qids {
			if name := names[qid]; name != "" {
				out = append(out, Company{QID: qid, Name: name})
			}
		}
		return out
	}
	return named(studios), named(networks), nil
}
