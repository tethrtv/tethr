package jobs

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/store"
	"github.com/tethrtv/tethr/internal/wikidata"
)

const refreshJob = "wikidata refresh queue"

func (j Titles) queue(ctx context.Context) error {
	ran, err := core.JobRanSince(ctx, j.DB, refreshJob, time.Now().Add(-time.Hour))
	if err != nil || ran {
		return err
	}
	queued, err := core.QueueWikidataRefreshes(ctx, j.DB)
	if err != nil {
		return err
	}
	j.Log.Debug("queued wikidata refreshes", "items", queued)
	return core.MarkJobRun(ctx, j.DB, refreshJob)
}

func (j Titles) edits(ctx context.Context) error {
	due, err := core.DueWikidataEdits(ctx, j.DB, 200)
	if err != nil {
		return err
	}
	var errs []error
	for len(due) > 0 {
		n := min(len(due), 50)
		err := j.refetch(ctx, due[:n])
		if errs = append(errs, err); errors.Is(err, wikidata.ErrBusy) || ctx.Err() != nil {
			break
		}
		due = due[n:]
	}
	return errors.Join(errs...)
}

func (j Titles) refetch(ctx context.Context, due []core.Pending) error {
	qids := make([]string, len(due))
	for i, p := range due {
		qids[i] = p.QID
	}
	titles, err := core.PendingTitles(ctx, j.DB, qids)
	if err != nil {
		return err
	}
	byItem := map[string][]store.Title{}
	for _, t := range titles {
		byItem[*t.WikidataID] = append(byItem[*t.WikidataID], t)
	}
	classes, err := core.PendingClasses(ctx, j.DB, qids)
	if err != nil {
		return err
	}
	fetchedAt := time.Now().Add(-time.Second)
	var items []string
	for _, q := range qids {
		if len(byItem[q]) > 0 {
			items = append(items, q)
		}
	}
	found, deleted, unreadable, err := j.Wikidata.FetchItems(ctx, items)
	if err != nil {
		return err
	}
	if err := j.refetchClasses(ctx, classes); err != nil {
		return err
	}
	var instanceOf []string
	for _, it := range found {
		instanceOf = append(instanceOf, it.InstanceOf()...)
	}
	if err := fillClasses(ctx, j.DB, j.Wikidata, j.Log, instanceOf); err != nil {
		return err
	}

	var errs []error
	for _, p := range due {
		var err error
		held := false
		switch it, ok := found[p.QID]; {
		case ok:
			held, err = j.refetchItem(ctx, p, it, byItem[p.QID])
		case slices.Contains(deleted, p.QID):
			for _, t := range byItem[p.QID] {
				j.Log.Warn("wikidata item deleted", "title", t.ID, "item", p.QID)
				err = errors.Join(err, core.WikidataDeleted(ctx, j.DB, t.ID))
			}
		case slices.Contains(unreadable, p.QID):
			for _, t := range byItem[p.QID] {
				j.Log.Warn("wikidata item unreadable, kept the title as it was", "title", t.ID, "item", p.QID)
				err = errors.Join(err, core.WikidataMissing(ctx, j.DB, t.ID))
			}
		}
		if err == nil && !held {
			err = core.DoneWikidata(ctx, j.DB, p.QID, fetchedAt)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("item %s: %w", p.QID, err))
			if ctx.Err() == nil {
				errs = append(errs, core.RetryWikidataLater(ctx, j.DB, p.QID))
			}
		}
	}
	return errors.Join(errs...)
}

func (j Titles) refetchItem(ctx context.Context, p core.Pending, it wikidata.Item, titles []store.Title) (bool, error) {
	kinds, complete, err := core.SeedKinds(ctx, j.DB, it.TMDBKinds(), it.InstanceOf())
	if err != nil {
		return false, err
	}
	var held core.Held
	for _, t := range titles {
		if h := core.Suspicious(t, core.WikidataTitle(it.Title(t.Kind))); !h.Empty() {
			held.Name, held.Year = h.Name, h.Year
		}
		if p.LoggedOut {
			held.Name = ""
		}
		if complete && !slices.Contains(kinds, t.Kind) {
			held.Kinds = kinds
			if len(kinds) == 0 {
				held.Kinds = []string{"none"}
			}
		}
	}
	if !held.Empty() && !held.Within(p.Held) {
		j.Log.Warn("holding a wikidata edit for a week", "item", p.QID, "name", held.Name, "year", held.Year, "kinds", held.Kinds)
		return true, core.HoldWikidata(ctx, j.DB, p.QID, held)
	}

	var moved []string
	var others []int64
	have := map[string]bool{}
	for _, t := range titles {
		have[t.Kind] = true
		if complete && !slices.Contains(kinds, t.Kind) {
			j.Log.Warn("wikidata item is no longer this kind, kept the title as it was", "title", t.ID, "item", p.QID, "kind", t.Kind, "now", kinds)
			if err := core.WikidataMissing(ctx, j.DB, t.ID); err != nil {
				return false, err
			}
			continue
		}
		res, err := core.RefreshWikidata(ctx, j.DB, t.ID, t.Kind, core.WikidataTitle(it.Title(t.Kind)), p.LoggedOut)
		if err != nil {
			return false, fmt.Errorf("title %d: %w", t.ID, err)
		}
		if res.KeptName != "" {
			j.Log.Warn("kept a title's old name", "title", t.ID, "item", p.QID, "name", res.KeptName, "why", res.KeptWhy)
		}
		for _, c := range res.Conflicts {
			j.Log.Warn("wikidata disagrees with an outside id", "title", t.ID, "provider", c.Provider, "value", c.Value, "other", c.Other, "outcome", c.Outcome)
			id := c.Provider + " " + c.Value
			sameItem := slices.ContainsFunc(titles, func(o store.Title) bool { return o.ID == c.Other })
			if c.Outcome == core.LeftWithAnotherItem && !sameItem && !slices.Contains(p.Held.IDs, id) {
				moved = append(moved, id)
				others = append(others, c.Other)
			}
		}
	}
	for _, kind := range kinds {
		if have[kind] {
			continue
		}
		res, err := core.SeedTitle(ctx, j.DB, kind, core.WikidataTitle(it.Title(kind)), 0, false)
		if err != nil {
			return false, err
		}
		j.Log.Warn("wikidata item gained a kind", "item", p.QID, "kind", kind, "title", res.Title, "outcome", res.Outcome)
	}
	if len(moved) == 0 {
		return false, nil
	}
	holders, err := core.WikidataItemsOf(ctx, j.DB, others)
	if err != nil {
		return false, err
	}
	if err := core.QueueWikidataNow(ctx, j.DB, holders); err != nil {
		return false, err
	}
	j.Log.Warn("holding outside ids that moved between items for a week", "item", p.QID, "ids", moved, "holders", holders)
	return true, core.HoldWikidata(ctx, j.DB, p.QID, core.Held{IDs: append(slices.Clone(p.Held.IDs), moved...)})
}

func (j Titles) refetchClasses(ctx context.Context, before []store.WikidataClass) error {
	if len(before) == 0 {
		return nil
	}
	qids := make([]string, len(before))
	meant := map[string]string{}
	for i, c := range before {
		qids[i] = c.Qid
		m, err := core.ClassMeaning(ctx, j.DB, c.Qid)
		if err != nil {
			return err
		}
		meant[c.Qid] = m
	}
	found, unreadable, err := j.Wikidata.FetchClasses(ctx, qids)
	if err != nil {
		return err
	}
	var changed, parents []string
	var batch []core.WikidataClass
	for _, b := range before {
		if slices.Contains(unreadable, b.Qid) {
			j.Log.Warn("wikidata class unreadable, kept it as it was", "class", b.Qid)
			if err := core.ClassChecked(ctx, j.DB, b.Qid); err != nil {
				return err
			}
			continue
		}
		c := found[b.Qid]
		batch = append(batch, core.WikidataClass(c))
		if c.Country != b.Country || c.Language != b.Language || !slices.Equal(c.Parents, b.Parents) {
			changed = append(changed, b.Qid)
			parents = append(parents, c.Parents...)
		}
	}
	if err := core.SaveClasses(ctx, j.DB, batch); err != nil {
		return err
	}
	if err := fillClasses(ctx, j.DB, j.Wikidata, j.Log, parents); err != nil {
		return err
	}
	for _, q := range changed {
		m, err := core.ClassMeaning(ctx, j.DB, q)
		if err != nil {
			return err
		}
		if m == meant[q] {
			continue
		}
		n, err := core.ForgetFactsUnder(ctx, j.DB, q)
		if err != nil {
			return err
		}
		j.Log.Info("wikidata class changed, working its titles out again", "class", q, "titles", n)
	}
	return nil
}
