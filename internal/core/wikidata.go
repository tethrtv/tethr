package core

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
	"golang.org/x/text/unicode/norm"
)

type WikidataTitle struct {
	QID      string
	Name     string
	Original string
	Year     int
	Runtime  int
	TMDB     int32
	IMDB     string
	TVDB     string
}

func DueWikidata(ctx context.Context, db *pgxpool.Pool, limit int32) ([]store.DueWikidataRow, error) {
	return store.New(db).DueWikidata(ctx, limit)
}

func WikidataMissing(ctx context.Context, db *pgxpool.Pool, titleID int64) error {
	return store.New(db).WikidataMissing(ctx, titleID)
}

func SaveWikidata(ctx context.Context, db *pgxpool.Pool, titleID int64, kind string, w WikidataTitle) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		params := store.SaveWikidataParams{ID: titleID, WikidataID: &w.QID, Name: text(w.Name), OriginalName: text(w.Original)}
		if w.Year > 0 {
			year := int16(w.Year)
			params.Year = &year
		}
		if w.Runtime > 0 {
			runtime := int16(w.Runtime)
			params.RuntimeMinutes = &runtime
		}
		if err := q.SaveWikidata(ctx, params); err != nil {
			return err
		}
		t, err := q.TitleByID(ctx, store.TitleByIDParams{ID: titleID, Kind: kind})
		if err != nil {
			return err
		}
		if err := nameSlug(ctx, q, t, w.Name, w.Year); err != nil {
			return err
		}
		if err := learnIDs(ctx, q, t, IDs{TMDB: int64(w.TMDB)}); err != nil {
			return err
		}
		for provider, value := range map[string]string{"imdb": w.IMDB, "tvdb": w.TVDB} {
			if value == "" {
				continue
			}
			if err := q.AddMissingExternalID(ctx, store.AddMissingExternalIDParams{TitleID: titleID, Provider: provider, Value: value}); err != nil {
				return err
			}
		}

		t, err = q.TitleByID(ctx, store.TitleByIDParams{ID: titleID, Kind: kind})
		if err != nil || t.MergedIntoID != nil {
			return err
		}
		other, err := q.TitleSharingWikidata(ctx, store.TitleSharingWikidataParams{Kind: kind, WikidataID: &w.QID, ID: titleID})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		from, into := t, other
		if (from.TmdbID != nil && into.TmdbID == nil) || ((from.TmdbID == nil) == (into.TmdbID == nil) && from.ID < into.ID) {
			from, into = into, from
		}
		return mergeTitle(ctx, q, kind, from.ID, into.ID)
	})
}

func mergeTitle(ctx context.Context, q *store.Queries, kind string, from, into int64) error {
	areas := []string{"movies.watched_at", "movies.paused_at", "movies.rated_at", "movies.watchlisted_at", "watchlist.updated_at"}
	if kind == "show" {
		areas = []string{
			"episodes.watched_at", "episodes.paused_at", "shows.rated_at", "seasons.rated_at", "episodes.rated_at",
			"shows.watchlisted_at", "seasons.watchlisted_at", "episodes.watchlisted_at", "shows.hidden_at", "watchlist.updated_at",
		}
	}
	if err := q.MarkMergedActivity(ctx, store.MarkMergedActivityParams{FromID: from, Areas: areas}); err != nil {
		return err
	}
	if err := q.MergeTitleEvents(ctx, store.MergeTitleEventsParams{FromID: from, IntoID: into}); err != nil {
		return err
	}
	if err := q.DropClashingTitlePlayback(ctx, store.DropClashingTitlePlaybackParams{FromID: from, IntoID: into}); err != nil {
		return err
	}
	if err := q.MergeTitlePlayback(ctx, store.MergeTitlePlaybackParams{FromID: from, IntoID: into}); err != nil {
		return err
	}
	if err := q.MergeTitleWatching(ctx, store.MergeTitleWatchingParams{FromID: from, IntoID: into}); err != nil {
		return err
	}
	if err := q.DropClashingTitleListItems(ctx, store.DropClashingTitleListItemsParams{FromID: from, IntoID: into}); err != nil {
		return err
	}
	if err := q.MergeTitleListItems(ctx, store.MergeTitleListItemsParams{FromID: from, IntoID: into}); err != nil {
		return err
	}
	if err := q.DropClashingTitleRatings(ctx, store.DropClashingTitleRatingsParams{FromID: from, IntoID: into}); err != nil {
		return err
	}
	if err := q.MergeTitleRatings(ctx, store.MergeTitleRatingsParams{FromID: from, IntoID: into}); err != nil {
		return err
	}
	if err := q.DropClashingTitleHidden(ctx, store.DropClashingTitleHiddenParams{FromID: from, IntoID: into}); err != nil {
		return err
	}
	if err := q.MergeTitleHidden(ctx, store.MergeTitleHiddenParams{FromID: from, IntoID: into}); err != nil {
		return err
	}

	episodes, err := q.ShowEpisodes(ctx, from)
	if err != nil {
		return err
	}
	for _, e := range episodes {
		if e.RemovedAt == nil {
			same, err := q.EpisodeByNumber(ctx, store.EpisodeByNumberParams{ShowID: into, SeasonNumber: e.SeasonNumber, EpisodeNumber: e.EpisodeNumber})
			if err == nil {
				if err := mergeEpisode(ctx, q, e.ID, same.ID); err != nil {
					return err
				}
				continue
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		if err := q.MoveEpisode(ctx, store.MoveEpisodeParams{ID: e.ID, IntoID: into}); err != nil {
			return err
		}
	}

	if err := q.MergeTitleExternalIDs(ctx, store.MergeTitleExternalIDsParams{FromID: from, IntoID: into}); err != nil {
		return err
	}
	return q.MarkTitleMerged(ctx, store.MarkTitleMergedParams{FromID: from, IntoID: &into})
}

func nameSlug(ctx context.Context, q *store.Queries, t store.Title, name string, year int) error {
	base := slugFor(name, year)
	if base == "" || t.Slug != "t-"+strconv.FormatInt(t.ID, 10) {
		return nil
	}
	for n := 1; n <= 100; n++ {
		slug := base
		if n > 1 {
			slug = base + "-" + strconv.Itoa(n)
		}
		if set, err := q.NameSlug(ctx, store.NameSlugParams{ID: t.ID, Slug: slug}); err != nil || set == 1 {
			return err
		}
	}
	return nil
}

func slugFor(name string, year int) string {
	if year == 0 {
		return ""
	}
	var b strings.Builder
	gap := false
	for _, r := range norm.NFKD.String(strings.ToLower(name)) {
		switch {
		case unicode.Is(unicode.Mn, r), r == '\'', r == '’':
		case r < utf8.RuneSelf && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			if gap && b.Len() > 0 {
				b.WriteByte('-')
			}
			gap = false
			b.WriteRune(r)
		default:
			gap = true
		}
	}
	if b.Len() == 0 {
		return ""
	}
	return b.String() + "-" + strconv.Itoa(year)
}

func text(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
