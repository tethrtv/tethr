package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/store"
	"github.com/tethrtv/tethr/internal/wikidata"
)

type Titles struct {
	DB       *pgxpool.Pool
	Wikidata *wikidata.Client
	Log      *slog.Logger
}

func (j Titles) Run(ctx context.Context) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		switch err := j.Refresh(ctx); {
		case err == nil || ctx.Err() != nil:
		case errors.Is(err, wikidata.ErrBusy):
			j.Log.Warn("wikidata is busy, trying again later", "err", err)
		default:
			j.Log.Error("looking up titles on wikidata", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (j Titles) Refresh(ctx context.Context) error {
	due, err := core.DueWikidata(ctx, j.DB, 200)
	if err != nil {
		return err
	}
	type batch struct{ kind, provider string }
	unmatched := map[batch][]store.DueWikidataRow{}
	var order []batch
	byKind := map[string][]store.DueWikidataRow{}
	for _, t := range due {
		if t.WikidataID != nil && wikidata.IsQID(*t.WikidataID) && t.WikidataDeletedAt == nil {
			byKind[t.Kind] = append(byKind[t.Kind], t)
			continue
		}
		b := batch{t.Kind, "tmdb"}
		switch {
		case t.TmdbID != nil:
		case t.Imdb != "":
			b.provider = "imdb"
		case t.Tvdb != "" && t.Kind == "show":
			b.provider = "tvdb"
		default:
			if err := core.WikidataMissing(ctx, j.DB, t.ID); err != nil {
				return err
			}
			continue
		}
		if _, ok := unmatched[b]; !ok {
			order = append(order, b)
		}
		unmatched[b] = append(unmatched[b], t)
	}

	var errs []error
	fresh := map[int64]bool{}
	for _, b := range order {
		for all := unmatched[b]; len(all) > 0; {
			n := min(len(all), 50)
			found, err := j.match(ctx, b.kind, b.provider, all[:n], fresh)
			if errs = append(errs, err); errors.Is(err, wikidata.ErrBusy) {
				return errors.Join(errs...)
			}
			byKind[b.kind] = append(byKind[b.kind], found...)
			all = all[n:]
		}
	}

	for _, kind := range []string{"movie", "show"} {
		for all := byKind[kind]; len(all) > 0; {
			n := min(len(all), 50)
			err := j.fetch(ctx, kind, all[:n], fresh)
			if errs = append(errs, err); errors.Is(err, wikidata.ErrBusy) {
				return errors.Join(errs...)
			}
			all = all[n:]
		}
	}
	return errors.Join(append(errs, j.rederive(ctx))...)
}

func (j Titles) match(ctx context.Context, kind, provider string, titles []store.DueWikidataRow, fresh map[int64]bool) ([]store.DueWikidataRow, error) {
	keys := make([]string, len(titles))
	for i, t := range titles {
		keys[i] = key(t, provider)
	}
	found, err := j.Wikidata.Find(ctx, kind, provider, keys)
	if err != nil {
		return nil, err
	}
	var matched []store.DueWikidataRow
	var errs []error
	for i, t := range titles {
		qid, ok := found[keys[i]]
		if !ok {
			if err := core.WikidataMissing(ctx, j.DB, t.ID); err != nil {
				errs = append(errs, fmt.Errorf("title %d: %w", t.ID, err))
			}
			continue
		}
		if t.WikidataID == nil || *t.WikidataID != qid {
			fresh[t.ID] = true
		}
		t.WikidataID = &qid
		matched = append(matched, t)
	}
	return matched, errors.Join(errs...)
}

func (j Titles) fetch(ctx context.Context, kind string, titles []store.DueWikidataRow, fresh map[int64]bool) error {
	qids := make([]string, len(titles))
	for i, t := range titles {
		qids[i] = *t.WikidataID
	}
	found, deleted, err := j.Wikidata.Fetch(ctx, kind, qids)
	if err != nil {
		return err
	}
	var classes []string
	for _, w := range found {
		classes = append(classes, slices.Concat(w.InstanceOf, w.Genres, w.Countries, w.Languages)...)
	}
	if err := j.fillClasses(ctx, classes); errors.Is(err, wikidata.ErrBusy) {
		return err
	} else if err != nil {
		j.Log.Warn("fetching wikidata classes, facts wait for the next run", "err", err)
	}
	var errs []error
	for _, t := range titles {
		w, ok := found[*t.WikidataID]
		switch {
		case ok:
			var conflicts []core.IDConflict
			conflicts, err = core.SaveWikidata(ctx, j.DB, t.ID, t.Kind, core.WikidataTitle(w))
			for _, c := range conflicts {
				j.Log.Warn("wikidata disagrees with an outside id", "title", t.ID, "provider", c.Provider, "value", c.Value, "other", c.Other, "outcome", c.Outcome)
			}
		case slices.Contains(deleted, *t.WikidataID) && fresh[t.ID]:
			err = core.WikidataMissing(ctx, j.DB, t.ID)
		case slices.Contains(deleted, *t.WikidataID):
			j.Log.Warn("wikidata item deleted", "title", t.ID, "item", *t.WikidataID)
			err = core.WikidataDeleted(ctx, j.DB, t.ID)
		default:
			j.Log.Warn("wikidata item unreadable", "title", t.ID, "item", *t.WikidataID)
			err = core.WikidataMissing(ctx, j.DB, t.ID)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("title %d: %w", t.ID, err))
		}
	}
	return errors.Join(errs...)
}

func key(t store.DueWikidataRow, provider string) string {
	switch provider {
	case "imdb":
		return t.Imdb
	case "tvdb":
		return t.Tvdb
	}
	return strconv.Itoa(int(*t.TmdbID))
}

func (j Titles) fillClasses(ctx context.Context, qids []string) error {
	for range 13 {
		missing, err := core.ClassesNeeded(ctx, j.DB, qids)
		if err != nil || len(missing) == 0 {
			return err
		}
		for len(missing) > 0 {
			n := min(len(missing), 50)
			found, unreadable, err := j.Wikidata.FetchClasses(ctx, missing[:n])
			if err != nil {
				return err
			}
			for _, q := range unreadable {
				j.Log.Warn("wikidata class unreadable, its facts are left out", "class", q)
			}
			batch := make([]core.WikidataClass, 0, len(found))
			for _, c := range found {
				batch = append(batch, core.WikidataClass(c))
			}
			if err := core.SaveClasses(ctx, j.DB, batch); err != nil {
				return err
			}
			missing = missing[n:]
		}
	}
	return nil
}

func (j Titles) rederive(ctx context.Context) error {
	titles, err := core.OutdatedFacts(ctx, j.DB, 200)
	if err != nil || len(titles) == 0 {
		return err
	}
	var qids []string
	for _, t := range titles {
		qids = append(qids, slices.Concat(t.WikidataInstanceOf, t.WikidataGenres, t.WikidataCountries, t.WikidataLanguages)...)
	}
	if err := j.fillClasses(ctx, qids); err != nil {
		return err
	}
	var errs []error
	for _, t := range titles {
		if err := core.RederiveFacts(ctx, j.DB, t); err != nil {
			errs = append(errs, fmt.Errorf("title %d: %w", t.ID, err))
		}
	}
	return errors.Join(errs...)
}
