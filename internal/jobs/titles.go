package jobs

import (
	"context"
	"log/slog"
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
		if err := j.Refresh(ctx); err != nil && ctx.Err() == nil {
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
	batches := map[batch][]store.DueWikidataRow{}
	var order []batch
	for _, t := range due {
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
		if _, ok := batches[b]; !ok {
			order = append(order, b)
		}
		batches[b] = append(batches[b], t)
	}

	for _, b := range order {
		for all := batches[b]; len(all) > 0; {
			n := min(len(all), 50)
			if err := j.lookUp(ctx, b.kind, b.provider, all[:n]); err != nil {
				return err
			}
			all = all[n:]
		}
	}
	return nil
}

func (j Titles) lookUp(ctx context.Context, kind, provider string, titles []store.DueWikidataRow) error {
	keys := make([]string, len(titles))
	for i, t := range titles {
		keys[i] = key(t, provider)
	}
	found, err := j.Wikidata.Find(ctx, kind, provider, keys)
	if err != nil {
		return err
	}
	for i, t := range titles {
		w, ok := found[keys[i]]
		if !ok {
			err = core.WikidataMissing(ctx, j.DB, t.ID)
		} else {
			err = core.SaveWikidata(ctx, j.DB, t.ID, t.Kind, core.WikidataTitle(w))
		}
		if err != nil {
			return err
		}
	}
	return nil
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
