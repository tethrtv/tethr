package core

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

type TitleState struct {
	Item    Item
	Watches []Entry
	Resume  *Entry
	Rating  *Entry
	Listing *Entry
	Seasons []SeasonState
}

type SeasonState struct {
	Number   int32
	Episodes []EpisodeState
}

type EpisodeState struct {
	Episode store.Episode
	Watches []Entry
	Resume  *Entry
}

func Title(ctx context.Context, db *pgxpool.Pool, profileID int64, id string) (TitleState, error) {
	q := store.New(db)
	ids := IDs{Slug: id}
	if n, err := strconv.ParseInt(id, 10, 64); err == nil {
		ids = IDs{Trakt: n}
	}
	var t store.Title
	var err error
	for _, kind := range []string{"movie", "show"} {
		if t, err = findTitle(ctx, q, kind, ids); !errors.Is(err, ErrNotFound) {
			break
		}
	}
	if err != nil {
		return TitleState{}, err
	}
	item, err := describe(ctx, q, resolved{title: t})
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
		play := Entry{ID: w.ID, At: w.WatchedAt, Action: w.Action}
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

	episodes, err := q.ShowOutline(ctx, t.ID)
	if err != nil {
		return s, err
	}
	for _, e := range episodes {
		if len(s.Seasons) == 0 || s.Seasons[len(s.Seasons)-1].Number != e.SeasonNumber {
			s.Seasons = append(s.Seasons, SeasonState{Number: e.SeasonNumber})
		}
		season := &s.Seasons[len(s.Seasons)-1]
		season.Episodes = append(season.Episodes, EpisodeState{Episode: e, Watches: plays[e.ID], Resume: resume[e.ID]})
	}
	return s, nil
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
	c, err := loadCatalogue(ctx, q, ids, nil)
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
