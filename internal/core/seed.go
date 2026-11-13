package core

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

var kindMap = loadMap("kinds")

func SeedKinds(ctx context.Context, db *pgxpool.Pool, tmdbKinds, instanceOf []string) ([]string, bool, error) {
	cs, err := loadClasses(ctx, store.New(db), rawFacts{InstanceOf: instanceOf})
	if err != nil {
		return nil, false, err
	}
	tags, complete := cs.tags(kindMap, instanceOf)
	if len(tmdbKinds) > 0 {
		if slices.Equal(tags, []string{"none"}) {
			return nil, complete, nil
		}
		return tmdbKinds, complete, nil
	}
	if len(tags) > 1 {
		tags = slices.DeleteFunc(tags, func(t string) bool { return t == "none" })
	}
	switch {
	case slices.Equal(tags, []string{"film"}):
		return []string{"movie"}, complete, nil
	case slices.Equal(tags, []string{"show"}):
		return []string{"show"}, complete, nil
	}
	return nil, complete, nil
}

func AddMissingClass(ctx context.Context, db *pgxpool.Pool, qid string) error {
	return store.New(db).AddMissingClass(ctx, qid)
}

type Seeded string

const (
	SeedCreated Seeded = "created"
	SeedUpdated Seeded = "updated"
	SeedSkipped Seeded = "skipped"
)

type SeedResult struct {
	Outcome   Seeded
	Title     int64
	Conflicts []IDConflict
}

func SeedTitle(ctx context.Context, db *pgxpool.Pool, kind string, w WikidataTitle, sharedTMDB int32, nameHeld bool) (SeedResult, error) {
	var res SeedResult
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		res = SeedResult{Outcome: SeedUpdated}
		q := store.New(tx)
		t, err := q.TitleByWikidata(ctx, store.TitleByWikidataParams{Kind: kind, WikidataID: &w.QID})
		if errors.Is(err, pgx.ErrNoRows) {
			if sharedTMDB != 0 {
				held, err := q.TitleByTMDB(ctx, store.TitleByTMDBParams{TmdbID: &sharedTMDB, Kind: kind})
				if err == nil && held.WikidataID == nil {
					res = SeedResult{Outcome: SeedSkipped, Title: held.ID}
					return nil
				}
				if err != nil && !errors.Is(err, pgx.ErrNoRows) {
					return err
				}
			}
			t, err = findSeedTitle(ctx, q, kind, w)
			switch {
			case errors.Is(err, ErrNotFound):
				res.Outcome = SeedCreated
				t, err = q.CreateTitle(ctx, store.CreateTitleParams{Kind: kind, TmdbID: int32ID(int64(w.TMDB))})
			case err == nil && t.WikidataID != nil && *t.WikidataID != w.QID:
				res = SeedResult{Outcome: SeedSkipped, Title: t.ID}
				return nil
			}
		}
		if err != nil {
			return err
		}
		res.Title = t.ID
		res.Conflicts, _, err = saveWikidataIn(ctx, q, t.ID, kind, w, nameHeld, nil)
		return err
	})
	if err != nil {
		return SeedResult{}, err
	}
	return res, nil
}

func findSeedTitle(ctx context.Context, q *store.Queries, kind string, w WikidataTitle) (store.Title, error) {
	tries := []IDs{{TMDB: int64(w.TMDB)}}
	for _, v := range w.IDs["imdb"] {
		tries = append(tries, IDs{TMDB: int64(w.TMDB), IMDB: v})
	}
	for _, v := range w.IDs["tvdb"] {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			tries = append(tries, IDs{TMDB: int64(w.TMDB), TVDB: n})
		}
	}
	for _, ids := range tries {
		if ids = ids.clean(); ids.empty() {
			continue
		}
		t, err := findTitle(ctx, q, kind, ids)
		if !errors.Is(err, ErrNotFound) {
			return t, err
		}
	}
	return store.Title{}, ErrNotFound
}

var listedMap = loadMap("listed")

func ListedClasses() []string {
	var out []string
	for q := range listedMap {
		out = append(out, q)
	}
	slices.Sort(out)
	return out
}

func KnownWikidataItems(ctx context.Context, db *pgxpool.Pool) (map[string]bool, error) {
	rows, err := store.New(db).KnownWikidataItems(ctx)
	if err != nil {
		return nil, err
	}
	known := make(map[string]bool, len(rows))
	for _, q := range rows {
		known[q] = true
	}
	return known, nil
}

func LeaveOutWikidata(ctx context.Context, db *pgxpool.Pool, qid, reason string) error {
	return store.New(db).LeaveOutWikidata(ctx, store.LeaveOutWikidataParams{Qid: qid, Reason: reason})
}

func JobRanSince(ctx context.Context, db *pgxpool.Pool, name string, since time.Time) (bool, error) {
	return store.New(db).JobRanSince(ctx, store.JobRanSinceParams{Name: name, RanAt: since})
}

func MarkJobRun(ctx context.Context, db *pgxpool.Pool, name string) error {
	return store.New(db).MarkJobRun(ctx, name)
}
