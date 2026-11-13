package core

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

type Edits struct {
	First, Last time.Time
	LoggedOut   bool
}

func FeedCursor(ctx context.Context, db *pgxpool.Pool) (time.Time, error) {
	return store.New(db).FeedCursor(ctx)
}

func QueueWikidataEdits(ctx context.Context, db *pgxpool.Pool, edits map[string]Edits, cursor time.Time) (int64, error) {
	params := store.QueueWikidataEditsParams{}
	for q, e := range edits {
		params.Qids = append(params.Qids, q)
		params.EditedAt = append(params.EditedAt, e.First)
		params.LastEditedAt = append(params.LastEditedAt, e.Last)
		params.LoggedOut = append(params.LoggedOut, e.LoggedOut)
	}
	var queued int64
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		var err error
		if len(params.Qids) > 0 {
			if queued, err = q.QueueWikidataEdits(ctx, params); err != nil {
				return err
			}
		}
		return q.MoveFeedCursor(ctx, cursor)
	})
	return queued, err
}

func QueueAllWikidata(ctx context.Context, db *pgxpool.Pool) (int64, error) {
	return store.New(db).QueueAllWikidata(ctx)
}

const safetyPass = 180 * 24

func QueueWikidataRefreshes(ctx context.Context, db *pgxpool.Pool) (int64, error) {
	q := store.New(db)
	sizes, err := q.WikidataRefreshSizes(ctx)
	if err != nil {
		return 0, err
	}
	perHour := func(n int64) int32 { return int32((n + safetyPass - 1) / safetyPass) }
	return q.QueueWikidataRefreshes(ctx, store.QueueWikidataRefreshesParams{Titles: perHour(sizes.Titles), Classes: perHour(sizes.Classes)})
}

func QueueWikidataNow(ctx context.Context, db *pgxpool.Pool, qids []string) error {
	return store.New(db).QueueWikidataNow(ctx, qids)
}

type Pending struct {
	QID       string
	Held      Held
	LoggedOut bool
}

func DueWikidataEdits(ctx context.Context, db *pgxpool.Pool, limit int32) ([]Pending, error) {
	rows, err := store.New(db).DuePendingWikidata(ctx, limit)
	if err != nil {
		return nil, err
	}
	due := make([]Pending, len(rows))
	for i, r := range rows {
		due[i].QID, due[i].LoggedOut = r.Qid, r.LoggedOut
		if r.Held != nil {
			if err := json.Unmarshal(r.Held, &due[i].Held); err != nil {
				return nil, err
			}
		}
	}
	return due, nil
}

func PendingTitles(ctx context.Context, db *pgxpool.Pool, qids []string) ([]store.Title, error) {
	return store.New(db).PendingTitles(ctx, qids)
}

func PendingClasses(ctx context.Context, db *pgxpool.Pool, qids []string) ([]store.WikidataClass, error) {
	return store.New(db).PendingClasses(ctx, qids)
}

func HoldWikidata(ctx context.Context, db *pgxpool.Pool, qid string, h Held) error {
	raw, err := json.Marshal(h)
	if err != nil {
		return err
	}
	return store.New(db).HoldWikidata(ctx, store.HoldWikidataParams{Qid: qid, Held: raw})
}

func DoneWikidata(ctx context.Context, db *pgxpool.Pool, qid string, fetchedAt time.Time) error {
	q := store.New(db)
	if err := q.DoneWikidata(ctx, store.DoneWikidataParams{Qid: qid, FetchedAt: &fetchedAt}); err != nil {
		return err
	}
	return q.RequeueWikidata(ctx, qid)
}

func WikidataItemsOf(ctx context.Context, db *pgxpool.Pool, titleIDs []int64) ([]string, error) {
	return store.New(db).WikidataItemsOf(ctx, titleIDs)
}

func ClassChecked(ctx context.Context, db *pgxpool.Pool, qid string) error {
	return store.New(db).ClassChecked(ctx, qid)
}

func RetryWikidataLater(ctx context.Context, db *pgxpool.Pool, qid string) error {
	return store.New(db).RetryWikidataLater(ctx, qid)
}

func ForgetFactsUnder(ctx context.Context, db *pgxpool.Pool, class string) (int64, error) {
	return store.New(db).ForgetFactsUnder(ctx, class)
}

type Held struct {
	Name  string   `json:"name,omitempty"`
	Year  int      `json:"year,omitempty"`
	Kinds []string `json:"kinds,omitempty"`
	IDs   []string `json:"ids,omitempty"`
}

func (h Held) Empty() bool {
	return h.Name == "" && h.Year == 0 && len(h.Kinds) == 0 && len(h.IDs) == 0
}

func (h Held) Within(before Held) bool {
	return (h.Name == "" || h.Name == before.Name) && (h.Year == 0 || h.Year == before.Year) &&
		(len(h.Kinds) == 0 || slices.Equal(h.Kinds, before.Kinds)) && !slices.ContainsFunc(h.IDs, func(id string) bool { return !slices.Contains(before.IDs, id) })
}

const wellKnown = 30

func Suspicious(before store.Title, w WikidataTitle) Held {
	var h Held
	if before.Name != nil && w.Name != "" && before.Sitelinks >= wellKnown && !shareWord(*before.Name, w.Name) {
		h.Name = w.Name
	}
	if before.Year != nil && w.Year > 0 && max(int(*before.Year)-w.Year, w.Year-int(*before.Year)) > 1 {
		h.Year = w.Year
	}
	return h
}

func shareWord(a, b string) bool {
	words := func(s string) []string {
		if slug := slugify(s); slug != "" {
			return strings.Split(slug, "-")
		}
		return strings.Fields(strings.ToLower(s))
	}
	other := words(b)
	for _, w := range words(a) {
		if slices.Contains(other, w) {
			return true
		}
	}
	return false
}

var scripts = []struct {
	name   string
	tables []*unicode.RangeTable
}{
	{"latin", []*unicode.RangeTable{unicode.Latin}},
	{"cjk", []*unicode.RangeTable{unicode.Han, unicode.Hiragana, unicode.Katakana}},
	{"hangul", []*unicode.RangeTable{unicode.Hangul}},
	{"cyrillic", []*unicode.RangeTable{unicode.Cyrillic}},
	{"greek", []*unicode.RangeTable{unicode.Greek}},
	{"arabic", []*unicode.RangeTable{unicode.Arabic}},
	{"hebrew", []*unicode.RangeTable{unicode.Hebrew}},
	{"devanagari", []*unicode.RangeTable{unicode.Devanagari}},
	{"thai", []*unicode.RangeTable{unicode.Thai}},
}

func script(name string) string {
	counts := map[string]int{}
	best := ""
	for _, r := range name {
		if !unicode.IsLetter(r) {
			continue
		}
		s := "other"
		for _, sc := range scripts {
			if unicode.In(r, sc.tables...) {
				s = sc.name
				break
			}
		}
		if counts[s]++; best == "" || counts[s] > counts[best] {
			best = s
		}
	}
	return best
}

type Refreshed struct {
	Conflicts []IDConflict
	KeptName  string
	KeptWhy   string
}

func RefreshWikidata(ctx context.Context, db *pgxpool.Pool, titleID int64, kind string, w WikidataTitle, loggedOut bool) (Refreshed, error) {
	var res Refreshed
	keep := func(old, new string) bool {
		switch {
		case script(old) == "latin" && script(new) != "" && script(new) != "latin":
			res.KeptWhy = "out of latin script"
		case loggedOut && script(old) == script(new) && !shareWord(old, new):
			res.KeptWhy = "renamed by a logged-out editor"
		}
		return res.KeptWhy != ""
	}
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		var err error
		res.Conflicts, res.KeptName, err = saveWikidataIn(ctx, store.New(tx), titleID, kind, w, false, keep)
		return err
	})
	if err != nil {
		return Refreshed{}, err
	}
	return res, nil
}
