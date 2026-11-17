package core

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

var (
	ErrRule     = errors.New("that rule can't be used")
	ErrRuleList = errors.New("a rule list's titles come from its rule, so they can't be added, ordered or given notes by hand")
	ErrRuleSort = errors.New("rule lists sort by rank, title or year, asc or desc")
)

const maxRuleValues = 50

var errTooManyTakenOut = fmt.Errorf("%w: up to 1,000 titles can be taken out", ErrRule)

var (
	TypeFlags    = []string{"animated", "anime", "event", "miniseries", "short", "silent", "special", "tv-film", "web"}
	qidPattern   = regexp.MustCompile(`^Q[1-9][0-9]*$`)
	countryCode  = regexp.MustCompile(`^[A-Z]{2}$`)
	languageCode = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)
)

type Rule struct {
	Kind      string  `json:"kind,omitempty"`
	Genres    Values  `json:"genres,omitzero"`
	Types     Values  `json:"types,omitzero"`
	Studios   Values  `json:"studios,omitzero"`
	Networks  Values  `json:"networks,omitzero"`
	Countries Values  `json:"countries,omitzero"`
	Languages Values  `json:"languages,omitzero"`
	Year      Between `json:"year,omitzero"`
	Runtime   Between `json:"runtime,omitzero"`
	Watched   *bool   `json:"watched,omitempty"`
	Rated     *bool   `json:"rated,omitempty"`
	Watchlist *bool   `json:"watchlist,omitempty"`
	LeftOut   []int64 `json:"left_out,omitempty"`
}

type Values struct {
	Any  []string `json:"any,omitempty"`
	None []string `json:"none,omitempty"`
}

type Between struct {
	From *int `json:"from,omitempty"`
	To   *int `json:"to,omitempty"`
}

type Preview struct {
	Entries []ListEntry
	Matched int
	Names   map[string]string
}

type CompanyMatch struct {
	QID       string
	Name      string
	StudioOf  int64
	NetworkOf int64
}

func (r Rule) check() error {
	if r.Kind != "" && r.Kind != "movie" && r.Kind != "show" {
		return fmt.Errorf("%w: kind is movie or show", ErrRule)
	}
	fields := []struct {
		name  string
		v     Values
		valid func(string) bool
	}{
		{"genres", r.Genres, func(s string) bool { return slices.Contains(genreOrder, s) }},
		{"types", r.Types, func(s string) bool { return slices.Contains(TypeFlags, s) }},
		{"studios", r.Studios, qidPattern.MatchString},
		{"networks", r.Networks, qidPattern.MatchString},
		{"countries", r.Countries, countryCode.MatchString},
		{"languages", r.Languages, languageCode.MatchString},
	}
	for _, f := range fields {
		if len(f.v.Any) > maxRuleValues || len(f.v.None) > maxRuleValues {
			return fmt.Errorf("%w: %s can hold up to %d values each way", ErrRule, f.name, maxRuleValues)
		}
		for _, s := range slices.Concat(f.v.Any, f.v.None) {
			if !f.valid(s) {
				return fmt.Errorf("%w: %q isn’t one of the %s", ErrRule, s, f.name)
			}
		}
	}
	for _, g := range []struct {
		name  string
		r     Between
		max   int
		words string
	}{{"year", r.Year, 3000, "a year is between 0 and 3000"}, {"runtime", r.Runtime, 10000, "a runtime is between 0 and 10,000 minutes"}} {
		from, to := g.r.From, g.r.To
		if from != nil && (*from < 0 || *from > g.max) || to != nil && (*to < 0 || *to > g.max) {
			return fmt.Errorf("%w: %s", ErrRule, g.words)
		}
		if from != nil && to != nil && *from > *to {
			return fmt.Errorf("%w: %s runs from a lower number to a higher one", ErrRule, g.name)
		}
	}
	if len(r.LeftOut) > maxListItems {
		return errTooManyTakenOut
	}
	return nil
}

func (r Rule) params(viewer int64, kids bool) store.RuleTitlesParams {
	p := store.RuleTitlesParams{
		Genres: orEmpty(r.Genres.Any), NotGenres: orEmpty(r.Genres.None),
		Types: orEmpty(r.Types.Any), NotTypes: orEmpty(r.Types.None),
		Studios: orEmpty(r.Studios.Any), NotStudios: orEmpty(r.Studios.None),
		Networks: orEmpty(r.Networks.Any), NotNetworks: orEmpty(r.Networks.None),
		Countries: orEmpty(r.Countries.Any), NotCountries: orEmpty(r.Countries.None),
		Languages: orEmpty(r.Languages.Any), NotLanguages: orEmpty(r.Languages.None),
		YearFrom: int32Of(r.Year.From), YearTo: int32Of(r.Year.To),
		RuntimeFrom: int32Of(r.Runtime.From), RuntimeTo: int32Of(r.Runtime.To),
		LeftOut: r.LeftOut, ForKids: kids, Viewer: viewer,
	}
	if p.LeftOut == nil {
		p.LeftOut = []int64{}
	}
	if r.Kind != "" {
		p.Kind = &r.Kind
	}
	if viewer != 0 {
		p.Watched, p.Rated, p.Watchlist = r.Watched, r.Rated, r.Watchlist
	}
	return p
}

func int32Of(n *int) *int32 {
	if n == nil {
		return nil
	}
	v := int32(*n)
	return &v
}

func readRule(raw []byte) (*Rule, error) {
	if raw == nil {
		return nil, nil
	}
	var r Rule
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("reading a list's rule: %w", err)
	}
	return &r, nil
}

func ruleJSON(r *Rule) ([]byte, error) {
	if r == nil {
		return nil, nil
	}
	return json.Marshal(r)
}

type ruleMatches struct {
	rows    []store.ListItemsInRow
	matched int64
}

func matchRule(ctx context.Context, q *store.Queries, listID int64, r Rule, viewer int64, kids bool) (ruleMatches, error) {
	found, err := q.RuleTitles(ctx, r.params(viewer, kids))
	if err != nil {
		return ruleMatches{}, err
	}
	m := ruleMatches{rows: make([]store.ListItemsInRow, 0, len(found))}
	for n, t := range found {
		m.matched = t.Matched
		m.rows = append(m.rows, store.ListItemsInRow{ID: t.ID, ListID: listID, TitleID: t.ID, Rank: int32(n + 1), ListedAt: t.CreatedAt, TitleKind: t.Kind})
	}
	return m, nil
}

func ruleListsWith(ctx context.Context, q *store.Queries, profileID, titleID int64) ([]InList, error) {
	lists, err := q.TitleRuleLists(ctx, profileID)
	if err != nil || len(lists) == 0 {
		return nil, err
	}
	kids, err := isKids(ctx, q, profileID)
	if err != nil {
		return nil, err
	}
	var on []InList
	for _, l := range lists {
		r, err := readRule(l.Rule)
		if err != nil {
			return nil, fmt.Errorf("rule list %d: %w", l.ID, err)
		}
		found, err := q.RuleTitles(ctx, r.params(profileID, kids))
		if err != nil {
			return nil, fmt.Errorf("rule list %d: %w", l.ID, err)
		}
		if slices.ContainsFunc(found, func(t store.RuleTitlesRow) bool { return t.ID == titleID }) {
			on = append(on, InList{ListID: l.ID, ItemID: titleID})
		}
	}
	return on, nil
}

func listItemRows(ctx context.Context, q *store.Queries, viewer int64, lists []List, kids bool) ([]store.ListItemsInRow, map[int64]int64, error) {
	var byHand []int64
	var rows []store.ListItemsInRow
	matched := map[int64]int64{}
	for _, l := range lists {
		if l.Rule == nil {
			byHand = append(byHand, l.ID)
			continue
		}
		m, err := matchRule(ctx, q, l.ID, *l.Rule, viewer, kids)
		if err != nil {
			return nil, nil, err
		}
		rows = append(rows, m.rows...)
		matched[l.ID] = m.matched
	}
	if len(byHand) > 0 {
		items, err := q.ListItemsIn(ctx, store.ListItemsInParams{ListIds: byHand, ForKids: kids})
		if err != nil {
			return nil, nil, err
		}
		rows = append(rows, items...)
	}
	return rows, matched, nil
}

func PreviewRule(ctx context.Context, db *pgxpool.Pool, viewer int64, r Rule, listID int64, query ListQuery) (Preview, error) {
	if err := r.check(); err != nil {
		return Preview{}, err
	}
	q := store.New(db)
	if listID != 0 {
		l, err := getList(ctx, q, viewer, listID)
		if err != nil {
			return Preview{}, err
		}
		if l.Rule != nil {
			r.LeftOut = l.Rule.LeftOut
		}
	}
	kids, err := isKids(ctx, q, viewer)
	if err != nil {
		return Preview{}, err
	}
	m, err := matchRule(ctx, q, 0, r, viewer, kids)
	if err != nil {
		return Preview{}, err
	}
	entries, _, err := entriesFor(ctx, q, viewer, m.rows, query, "rank", "asc")
	if err != nil {
		return Preview{}, err
	}
	names, err := companyNames(ctx, q, []Rule{r})
	return Preview{Entries: entries, Matched: int(m.matched), Names: names}, err
}

func companyNames(ctx context.Context, q *store.Queries, rules []Rule) (map[string]string, error) {
	var qids []string
	for _, r := range rules {
		qids = slices.Concat(qids, r.Studios.Any, r.Studios.None, r.Networks.Any, r.Networks.None)
	}
	names := map[string]string{}
	if len(qids) == 0 {
		return names, nil
	}
	labels, err := q.ClassLabels(ctx, qids)
	if err != nil {
		return nil, err
	}
	for _, l := range labels {
		names[l.Qid] = l.Label
	}
	return names, nil
}

func CopyRuleList(ctx context.Context, db *pgxpool.Pool, profileID, listID int64, name string) (List, error) {
	from, entries, _, err := ListEntries(ctx, db, profileID, listID, ListQuery{})
	if err != nil {
		return List{}, err
	}
	if from.Rule == nil {
		return List{}, ErrNotFound
	}
	l := store.List{Name: cmp.Or(strings.TrimSpace(name), from.Name), Privacy: "private", SortBy: "rank", SortHow: "asc"}
	if err := (ListChanges{}).apply(&l); err != nil {
		return List{}, err
	}
	var out List
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		id, err := createList(ctx, q, profileID, l)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if _, err := q.AddListItem(ctx, store.AddListItemParams{ListID: id, TitleID: e.Item.Title.ID}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		out, err = getList(ctx, q, profileID, id)
		return err
	})
	return out, err
}

func leaveOut(ctx context.Context, q *store.Queries, viewer int64, l store.List, titleIDs []int64) (int64, error) {
	r, err := readRule(l.Rule)
	if err != nil {
		return 0, err
	}
	kids, err := isKids(ctx, q, viewer)
	if err != nil {
		return 0, err
	}
	m, err := matchRule(ctx, q, l.ID, *r, viewer, kids)
	if err != nil {
		return 0, err
	}
	var removed int64
	for _, row := range m.rows {
		if slices.Contains(titleIDs, row.TitleID) {
			r.LeftOut = append(r.LeftOut, row.TitleID)
			removed++
		}
	}
	if removed == 0 {
		return 0, nil
	}
	if len(r.LeftOut) > maxListItems {
		return 0, errTooManyTakenOut
	}
	raw, err := ruleJSON(r)
	if err != nil {
		return 0, err
	}
	return removed, q.SetListRule(ctx, store.SetListRuleParams{ID: l.ID, Rule: raw})
}

func Companies(ctx context.Context, db *pgxpool.Pool, search string) ([]CompanyMatch, error) {
	search = strings.TrimSpace(search)
	if search == "" {
		return []CompanyMatch{}, nil
	}
	search = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(search)
	rows, err := store.New(db).SearchCompanies(ctx, search)
	if err != nil {
		return nil, err
	}
	out := make([]CompanyMatch, 0, len(rows))
	for _, r := range rows {
		out = append(out, CompanyMatch{QID: r.Qid, Name: r.Label, StudioOf: r.StudioOf, NetworkOf: r.NetworkOf})
	}
	return out, nil
}

func ruleVersion(rows []store.ListItemsInRow) string {
	var b strings.Builder
	for _, r := range rows {
		fmt.Fprintf(&b, "%d,", r.TitleID)
	}
	return b.String()
}
