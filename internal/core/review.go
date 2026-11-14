package core

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

const (
	KeptName        = "kept_name"
	HeldChange      = "held_change"
	KindChange      = "kind_change"
	SharedID        = "shared_id"
	TMDBDiffers     = "tmdb_differs"
	DeletedItem     = "deleted_item"
	FilmAndShow     = "film_and_show"
	UnreadableClass = "unreadable_class"
	UnmappedValue   = "unmapped_value"
)

var ItemKinds = []string{KeptName, HeldChange, KindChange, SharedID, TMDBDiffers, DeletedItem}

const reviewThreshold = 5

var (
	ErrReviewAccept = errors.New("only a kept-back name can be accepted")
	ErrReviewReason = errors.New("say why it's being dismissed")
)

type ReviewFlag struct {
	Kind     string
	QID      string
	Title    int64
	Subject  string
	Tethr    string
	Wikidata string
	Why      string
	Other    int64
	Batch    string
}

func (f ReviewFlag) key() string {
	return fmt.Sprintf("%s %s %d %s", f.Kind, f.QID, f.Title, f.Subject)
}

type ReviewEntry struct {
	ID        int64
	Kind      string
	QID       string
	Title     int64
	TitleName string
	TitleYear int
	TitleKind string
	TitleSlug string
	TitleTMDB int32
	Sitelinks int
	Subject   string
	Tethr     string
	Wikidata  string
	Why       string
	Other     int64
	OtherName string
	OtherQID  string
	Batch     string
	CreatedAt time.Time
}

func FlagForReview(ctx context.Context, db *pgxpool.Pool, flags []ReviewFlag) error {
	return flagReview(ctx, store.New(db), flags)
}

func flagReview(ctx context.Context, q *store.Queries, flags []ReviewFlag) error {
	if len(flags) == 0 {
		return nil
	}
	var p store.FlagReviewParams
	seen := map[string]int{}
	for _, f := range flags {
		k := f.key()
		if i, ok := seen[k]; ok {
			p.TethrValues[i], p.WikidataValues[i], p.Whys[i], p.OtherIds[i] = f.Tethr, f.Wikidata, f.Why, f.Other
			continue
		}
		seen[k] = len(p.Keys)
		p.Kinds = append(p.Kinds, f.Kind)
		p.Keys = append(p.Keys, k)
		p.Qids = append(p.Qids, f.QID)
		p.TitleIds = append(p.TitleIds, f.Title)
		p.Subjects = append(p.Subjects, f.Subject)
		p.TethrValues = append(p.TethrValues, f.Tethr)
		p.WikidataValues = append(p.WikidataValues, f.Wikidata)
		p.Whys = append(p.Whys, f.Why)
		p.OtherIds = append(p.OtherIds, f.Other)
		p.Batches = append(p.Batches, f.Batch)
	}
	return q.FlagReview(ctx, p)
}

func keys(flags []ReviewFlag) []string {
	out := make([]string, len(flags))
	for i, f := range flags {
		out[i] = f.key()
	}
	return out
}

type Settle struct {
	Titles []int64
	Kinds  []string
}

func SettleReview(ctx context.Context, db *pgxpool.Pool, flags []ReviewFlag, settles ...Settle) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := flagReview(ctx, q, flags); err != nil {
			return err
		}
		var released []int64
		for _, s := range settles {
			if len(s.Titles) == 0 {
				continue
			}
			if err := q.SettleReview(ctx, store.SettleReviewParams{TitleIds: s.Titles, Kinds: s.Kinds, Keep: keys(flags)}); err != nil {
				return err
			}
			if slices.Contains(s.Kinds, HeldChange) {
				if err := q.SettleHeldChanges(ctx, s.Titles); err != nil {
					return err
				}
			}
			if slices.Contains(s.Kinds, SharedID) {
				released = append(released, s.Titles...)
			}
		}
		if len(released) == 0 {
			return nil
		}
		claimants, err := q.SharedIDsLetGo(ctx, released)
		if err != nil || len(claimants) == 0 {
			return err
		}
		return q.QueueWikidataNow(ctx, claimants)
	})
}

func replaceReview(ctx context.Context, db *pgxpool.Pool, kind string, flags []ReviewFlag, undecided []ReviewFlag) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := flagReview(ctx, q, flags); err != nil {
			return err
		}
		return q.ReplaceReview(ctx, store.ReplaceReviewParams{Kind: kind, Keep: append(keys(flags), keys(undecided)...)})
	})
}

func ClassesRead(ctx context.Context, db *pgxpool.Pool, qids []string) error {
	read := make([]ReviewFlag, len(qids))
	for i, q := range qids {
		read[i] = ReviewFlag{Kind: UnreadableClass, QID: q}
	}
	return store.New(db).ClearReviewKeys(ctx, keys(read))
}

func RecheckWikidata(ctx context.Context, db *pgxpool.Pool, titleID int64) error {
	return store.New(db).RecheckWikidata(ctx, titleID)
}

func ConflictFlags(ctx context.Context, db *pgxpool.Pool, titleID int64, qid string, conflicts []IDConflict) ([]ReviewFlag, error) {
	var others []int64
	for _, c := range conflicts {
		if c.Other != 0 {
			others = append(others, c.Other)
		}
	}
	items := map[int64]string{}
	if len(others) > 0 {
		rows, err := store.New(db).TitlesWikidata(ctx, others)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			if r.WikidataID != nil {
				items[r.ID] = *r.WikidataID
			}
		}
	}
	var flags []ReviewFlag
	for _, c := range conflicts {
		switch c.Outcome {
		case LeftWithAnotherItem, LeftWithAnotherTitle:
			if items[c.Other] == qid {
				continue
			}
			flags = append(flags, ReviewFlag{Kind: SharedID, QID: qid, Title: titleID, Subject: c.Provider + " " + c.Value, Other: c.Other})
		case KeptTitlesOwn:
			flags = append(flags, ReviewFlag{Kind: TMDBDiffers, QID: qid, Title: titleID, Subject: "tmdb", Wikidata: c.Value})
		}
	}
	return flags, nil
}

func OpenReviewSubjects(ctx context.Context, db *pgxpool.Pool, kind string, titleIDs []int64) ([]string, error) {
	return store.New(db).OpenReviewSubjects(ctx, store.OpenReviewSubjectsParams{Kind: kind, TitleIds: titleIDs})
}

func OpenReview(ctx context.Context, db *pgxpool.Pool) ([]ReviewEntry, error) {
	rows, err := store.New(db).OpenReview(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ReviewEntry, len(rows))
	for i, r := range rows {
		out[i] = ReviewEntry{
			ID: r.ID, Kind: r.Kind, QID: r.Qid, Title: deref(r.TitleID), TitleName: deref(r.TitleName), TitleYear: int(deref(r.TitleYear)),
			TitleKind: deref(r.TitleKind), TitleSlug: deref(r.TitleSlug), TitleTMDB: deref(r.TitleTmdb), Sitelinks: int(r.Sitelinks),
			Subject: r.Subject, Tethr: r.TethrValue, Wikidata: r.WikidataValue, Why: r.Why, Other: deref(r.OtherID),
			OtherName: deref(r.OtherName), OtherQID: deref(r.OtherQid), Batch: deref(r.Batch), CreatedAt: r.CreatedAt,
		}
	}
	return out, nil
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

func reviewTargets(ctx context.Context, q *store.Queries, target string) ([]store.ReviewTargetsRow, error) {
	p := store.ReviewTargetsParams{ID: -1, Batch: &target}
	if id, err := strconv.ParseInt(target, 10, 64); err == nil {
		p = store.ReviewTargetsParams{ID: id}
	}
	rows, err := q.ReviewTargets(ctx, p)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	return rows, nil
}

func AcceptReview(ctx context.Context, db *pgxpool.Pool, target string) (int, error) {
	var accepted []int64
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		rows, err := reviewTargets(ctx, q, target)
		if err != nil {
			return err
		}
		for _, r := range rows {
			if r.Kind != KeptName || r.TitleID == nil {
				continue
			}
			if err := q.SetTitleName(ctx, store.SetTitleNameParams{ID: *r.TitleID, Name: &r.WikidataValue}); err != nil {
				return err
			}
			accepted = append(accepted, r.ID)
		}
		if len(accepted) == 0 {
			return ErrReviewAccept
		}
		return q.CloseReview(ctx, store.CloseReviewParams{Ids: accepted, Outcome: optional("accepted")})
	})
	return len(accepted), err
}

func DismissReview(ctx context.Context, db *pgxpool.Pool, target, reason string) (int, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return 0, ErrReviewReason
	}
	var n int
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		rows, err := reviewTargets(ctx, q, target)
		if err != nil {
			return err
		}
		ids := make([]int64, len(rows))
		for i, r := range rows {
			ids[i] = r.ID
		}
		n = len(ids)
		return q.CloseReview(ctx, store.CloseReviewParams{Ids: ids, Outcome: optional("dismissed"), Reason: &reason})
	})
	if err != nil {
		return 0, err
	}
	return n, nil
}

func PruneReview(ctx context.Context, db *pgxpool.Pool) error {
	return store.New(db).PruneReview(ctx)
}

func CheckCatalogue(ctx context.Context, db *pgxpool.Pool) error {
	q := store.New(db)
	items, err := q.FilmAndShowItems(ctx)
	if err != nil {
		return err
	}
	var twins []ReviewFlag
	for _, it := range items {
		twins = append(twins, ReviewFlag{Kind: FilmAndShow, QID: it.Qid, Title: it.FilmID, Other: it.ShowID})
	}
	if err := replaceReview(ctx, db, FilmAndShow, twins, nil); err != nil {
		return err
	}

	values, err := q.RawValueCounts(ctx, reviewThreshold)
	if err != nil {
		return err
	}
	var qids []string
	for _, v := range values {
		qids = append(qids, v.Qid)
	}
	cs, err := loadClasses(ctx, q, rawFacts{InstanceOf: qids})
	if err != nil {
		return err
	}
	var unmapped, undecided []ReviewFlag
	for _, v := range values {
		f := ReviewFlag{Kind: UnmappedValue, QID: v.Qid, Subject: v.Field, Tethr: v.Label, Why: fmt.Sprintf("%d titles", v.Titles)}
		switch mapped, known := cs.maps(v.Field, v.Qid); {
		case !known:
			undecided = append(undecided, f)
		case !mapped:
			unmapped = append(unmapped, f)
		}
	}
	return replaceReview(ctx, db, UnmappedValue, unmapped, undecided)
}

func (cs classes) maps(field, q string) (mapped, known bool) {
	switch field {
	case "country", "language":
		hand, code := countryMap, func(c store.ClassAncestryRow) string { return c.Country }
		if field == "language" {
			hand, code = languageMap, func(c store.ClassAncestryRow) string { return c.Language }
		}
		if _, ok := hand[q]; ok {
			return true, true
		}
		c, ok := cs[q]
		return ok && code(c) != "", ok
	}
	known = true
	for _, m := range []classMap{genreMap, typeMap, adultMap, sexualMap} {
		hit, complete := cs.reaches(m, q)
		if hit {
			return true, true
		}
		known = known && complete
	}
	return false, known
}

func (cs classes) reaches(m classMap, q string) (hit, complete bool) {
	complete = true
	seen := map[string]bool{}
	var walk func(q string, depth int)
	walk = func(q string, depth int) {
		if hit || seen[q] || depth > 12 {
			return
		}
		seen[q] = true
		if _, ok := m[q]; ok {
			hit = true
			return
		}
		c, ok := cs[q]
		if !ok {
			complete = false
			return
		}
		for _, p := range c.Parents {
			walk(p, depth+1)
		}
	}
	walk(q, 0)
	return hit, complete
}

var ReviewKinds = []string{KeptName, HeldChange, KindChange, SharedID, TMDBDiffers, DeletedItem, FilmAndShow, UnreadableClass, UnmappedValue}

var reviewNames = map[string][2]string{
	KeptName:        {"kept name", "kept names"},
	HeldChange:      {"change after a week's hold", "changes after a week's hold"},
	KindChange:      {"title that changed kind", "titles that changed kind"},
	SharedID:        {"outside ID on two titles", "outside IDs on two titles"},
	TMDBDiffers:     {"TMDB disagreement", "TMDB disagreements"},
	DeletedItem:     {"deleted item", "deleted items"},
	FilmAndShow:     {"film-and-show item", "film-and-show items"},
	UnreadableClass: {"unreadable class", "unreadable classes"},
	UnmappedValue:   {"value that maps to nothing", "values that map to nothing"},
}

func ReviewCount(kind string, n int) string {
	if n == 1 {
		return "1 " + reviewNames[kind][0]
	}
	return fmt.Sprintf("%d %s", n, reviewNames[kind][1])
}

func WikidataURL(qid string) string {
	return "https://www.wikidata.org/wiki/" + qid
}

func oneLine(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}

func (e ReviewEntry) Name() string {
	switch {
	case e.Title != 0 && e.TitleYear != 0:
		return oneLine(fmt.Sprintf("%s (%d)", e.TitleName, e.TitleYear))
	case e.Title != 0 && e.TitleName != "":
		return oneLine(e.TitleName)
	case e.Kind == UnmappedValue && e.Tethr != "":
		return oneLine(e.Tethr)
	}
	return e.QID
}

func kindWord(kind string) string {
	switch kind {
	case "movie":
		return "a film"
	case "show":
		return "a show"
	}
	return "not a film or show"
}

func (e ReviewEntry) Describe() string {
	tethr, wikidata, why := oneLine(e.Tethr), oneLine(e.Wikidata), oneLine(e.Why)
	switch e.Kind {
	case KeptName:
		return fmt.Sprintf("%s: kept “%s”, Wikidata says “%s” (%s)", e.Name(), tethr, wikidata, why)
	case HeldChange:
		if e.Subject == "year" {
			return fmt.Sprintf("%s: year moved from %s after a week's hold", e.Name(), tethr)
		}
		return fmt.Sprintf("%s: renamed from “%s” after a week's hold", e.Name(), tethr)
	case KindChange:
		if e.Wikidata == "none" {
			return fmt.Sprintf("%s: no longer a film or show on Wikidata, kept as it was", e.Name())
		}
		return fmt.Sprintf("%s: now %s on Wikidata, kept as %s", e.Name(), kindWord(e.Wikidata), kindWord(e.Tethr))
	case SharedID:
		other := "another title"
		if e.OtherName != "" {
			other = "“" + oneLine(e.OtherName) + "”"
		}
		if e.OtherQID != "" {
			other += " (" + e.OtherQID + ")"
		}
		return fmt.Sprintf("%s: %s is also on %s", e.Name(), e.Subject, other)
	case TMDBDiffers:
		return fmt.Sprintf("%s: Wikidata gives TMDB %s, Tethr kept %d", e.Name(), wikidata, e.TitleTMDB)
	case DeletedItem:
		return fmt.Sprintf("%s: Wikidata item deleted, kept the last data", e.Name())
	case FilmAndShow:
		return fmt.Sprintf("%s: both a film and a show on Wikidata", e.Name())
	case UnreadableClass:
		return fmt.Sprintf("Class %s can't be read from Wikidata", e.QID)
	case UnmappedValue:
		return fmt.Sprintf("%s “%s” (%s) on %s maps to nothing", e.Subject, tethr, e.QID, why)
	}
	return e.Name()
}

var batchPhrases = map[string]string{
	KeptName:    "kept their names",
	HeldChange:  "changed after a week's hold",
	SharedID:    "share outside IDs with other titles",
	TMDBDiffers: "disagree with Wikidata on TMDB IDs",
	DeletedItem: "lost their Wikidata items",
}

func BatchSummary(kind string, entries []ReviewEntry) string {
	phrase := batchPhrases[kind]
	if kind == KindChange {
		was := map[string]bool{}
		for _, e := range entries {
			was[e.Tethr] = true
		}
		switch {
		case len(was) > 1:
			phrase = "stopped being films or shows"
		case was["show"]:
			phrase = "stopped being shows"
		default:
			phrase = "stopped being films"
		}
	}
	n := "1 title"
	if len(entries) != 1 {
		n = fmt.Sprintf("%d titles", len(entries))
	}
	return fmt.Sprintf("%s from one batch %s", n, phrase)
}
