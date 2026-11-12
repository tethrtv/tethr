package core

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"
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
	Released time.Time
	Runtime  int
	TMDB     int32
	IDs      map[string][]string
}

type IDConflict struct {
	Provider string
	Value    string
	Other    int64
	Outcome  string
}

func DueWikidata(ctx context.Context, db *pgxpool.Pool, limit int32) ([]store.DueWikidataRow, error) {
	return store.New(db).DueWikidata(ctx, limit)
}

func WikidataMissing(ctx context.Context, db *pgxpool.Pool, titleID int64) error {
	return store.New(db).WikidataMissing(ctx, titleID)
}

func WikidataDeleted(ctx context.Context, db *pgxpool.Pool, titleID int64) error {
	return store.New(db).WikidataDeleted(ctx, titleID)
}

func SaveWikidata(ctx context.Context, db *pgxpool.Pool, titleID int64, kind string, w WikidataTitle) ([]IDConflict, error) {
	var conflicts []IDConflict
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		conflicts = nil
		q := store.New(tx)
		before, err := q.TitleByID(ctx, store.TitleByIDParams{ID: titleID, Kind: kind})
		if err != nil {
			return err
		}
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
		if settled(before, w) {
			if err := nameSlug(ctx, q, t, w.Name, w.Year); err != nil {
				return err
			}
		}
		if err := learnIDs(ctx, q, t, IDs{TMDB: int64(w.TMDB)}); err != nil {
			return err
		}
		tmdb := strconv.Itoa(int(w.TMDB))
		switch {
		case w.TMDB == 0:
		case t.TmdbID != nil && *t.TmdbID != w.TMDB:
			conflicts = append(conflicts, IDConflict{Provider: "tmdb", Value: tmdb, Outcome: "kept the title's own"})
		case t.TmdbID == nil:
			holder, err := q.TitleByTMDB(ctx, store.TitleByTMDBParams{TmdbID: &w.TMDB, Kind: kind})
			switch {
			case errors.Is(err, pgx.ErrNoRows):
			case err != nil:
				return err
			case holder.ID != t.ID && holder.MergedIntoID == nil && (holder.WikidataID == nil || *holder.WikidataID != w.QID):
				conflicts = append(conflicts, IDConflict{Provider: "tmdb", Value: tmdb, Other: holder.ID, Outcome: "left with another title"})
			}
		}
		if conflicts, err = syncIDs(ctx, q, t, w, conflicts); err != nil {
			return err
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
		if t.TmdbID != nil && other.TmdbID != nil && *t.TmdbID != *other.TmdbID {
			conflicts = append(conflicts, IDConflict{Provider: "tmdb", Value: strconv.Itoa(int(*other.TmdbID)), Other: other.ID, Outcome: "merged two of one item's"})
		}
		return mergeTitle(ctx, q, kind, max(t.ID, other.ID), min(t.ID, other.ID))
	})
	if err != nil {
		return nil, err
	}
	return conflicts, nil
}

func syncIDs(ctx context.Context, q *store.Queries, t store.Title, w WikidataTitle, conflicts []IDConflict) ([]IDConflict, error) {
	want := map[[2]string]bool{}
	var claims [][2]string
	for provider, values := range w.IDs {
		for _, v := range values {
			want[[2]string{provider, v}] = true
			claims = append(claims, [2]string{provider, v})
		}
	}
	slices.SortFunc(claims, func(a, b [2]string) int { return strings.Compare(a[0]+" "+a[1], b[0]+" "+b[1]) })

	claimed := map[string]bool{}
	for _, c := range claims {
		owner, err := q.ExternalIDOwner(ctx, store.ExternalIDOwnerParams{Provider: c[0], Value: c[1]})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
		case err != nil:
			return nil, err
		case owner.TitleID == t.ID && owner.Source == "wikidata":
			claimed[c[0]] = true
			continue
		case owner.TitleID == t.ID, owner.Kind == t.Kind && owner.WikidataID != nil && *owner.WikidataID == w.QID:
		case owner.Source == "wikidata":
			conflicts = append(conflicts, IDConflict{Provider: c[0], Value: c[1], Other: owner.TitleID, Outcome: "left with another item's title"})
			continue
		default:
			conflicts = append(conflicts, IDConflict{Provider: c[0], Value: c[1], Other: owner.TitleID, Outcome: "took from an app's pairing"})
		}
		if err := q.ClaimExternalID(ctx, store.ClaimExternalIDParams{TitleID: t.ID, Provider: c[0], Value: c[1]}); err != nil {
			return nil, err
		}
		claimed[c[0]] = true
	}

	have, err := q.TitleExternalIDs(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	for _, id := range have {
		if want[[2]string{id.Provider, id.Value}] || (id.Source == "app" && !claimed[id.Provider]) {
			continue
		}
		if err := q.DropExternalID(ctx, store.DropExternalIDParams{TitleID: t.ID, Provider: id.Provider, Value: id.Value}); err != nil {
			return nil, err
		}
		if id.Source == "app" {
			conflicts = append(conflicts, IDConflict{Provider: id.Provider, Value: id.Value, Outcome: "dropped an app's"})
		}
	}
	return conflicts, nil
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
	if err := q.TouchListsWithTitles(ctx, []int64{from, into}); err != nil {
		return err
	}
	if err := q.CopyTitleSeasons(ctx, store.CopyTitleSeasonsParams{FromID: from, IntoID: into}); err != nil {
		return err
	}
	if err := q.MergeTitleSeasons(ctx, store.MergeTitleSeasonsParams{FromID: from, IntoID: into}); err != nil {
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
	if err := q.MergeTitleHeldPlays(ctx, store.MergeTitleHeldPlaysParams{FromID: from, IntoID: into}); err != nil {
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

	tmdb, err := q.TakeTitleTMDB(ctx, store.TakeTitleTMDBParams{FromID: from, IntoID: into})
	switch {
	case err == nil:
		if err := q.SetTitleTMDB(ctx, store.SetTitleTMDBParams{ID: into, TmdbID: tmdb}); err != nil {
			return err
		}
	case !errors.Is(err, pgx.ErrNoRows):
		return err
	}
	if err := q.MergeTitleExternalIDs(ctx, store.MergeTitleExternalIDsParams{FromID: from, IntoID: into}); err != nil {
		return err
	}
	if err := q.DropTitleShowSettings(ctx, from); err != nil {
		return err
	}
	if err := q.DropTitleEpisodeOrders(ctx, from); err != nil {
		return err
	}
	return q.MarkTitleMerged(ctx, store.MarkTitleMergedParams{FromID: from, IntoID: &into})
}

func settled(before store.Title, w WikidataTitle) bool {
	now := time.Now()
	if w.Released.IsZero() || w.Released.After(now) || before.WikidataCheckedAt == nil || before.WikidataCheckedAt.After(now.Add(-6*24*time.Hour)) {
		return false
	}
	if before.WikidataID == nil || *before.WikidataID != w.QID || before.Name == nil || before.Year == nil {
		return false
	}
	return slugFor(*before.Name, int(*before.Year)) == slugFor(w.Name, w.Year)
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
	base := slugify(name)
	if year == 0 || base == "" {
		return ""
	}
	return base + "-" + strconv.Itoa(year)
}

var folds = strings.NewReplacer("ı", "i", "ø", "o", "æ", "ae", "œ", "oe", "ß", "ss", "đ", "d", "ð", "d", "ł", "l", "þ", "th", "ə", "e", "ħ", "h", "ŋ", "ng")

func slugify(name string) string {
	var b strings.Builder
	gap := false
	for _, r := range norm.NFKD.String(folds.Replace(strings.ToLower(name))) {
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
	return b.String()
}

func text(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
