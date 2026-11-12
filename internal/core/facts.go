package core

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

//go:embed maps/*.tsv
var mapFiles embed.FS

type classMap map[string][]string

var (
	genreMap, typeMap, adultMap, sexualMap = loadMap("genres"), loadMap("types"), loadMap("adult"), loadMap("sexual")
	countryMap, languageMap                = loadMap("countries"), loadMap("languages")
	factsVersion                           = mapsVersion()
)

var genreOrder = []string{
	"drama", "comedy", "documentary", "crime", "romance", "action", "thriller", "horror", "adventure", "fantasy", "science-fiction", "music",
	"mystery", "kids", "history", "biography", "western", "family", "war", "soap", "reality", "talk-and-game", "sport", "news",
}

var macroLanguages = map[string]string{"cmn": "zh", "yue": "zh", "wuu": "zh", "nan": "zh", "hak": "zh", "sh": "sr"}

func loadMap(name string) classMap {
	m, err := parseMap(name)
	if err != nil {
		panic(err)
	}
	return m
}

func parseMap(name string) (classMap, error) {
	raw, err := mapFiles.ReadFile("maps/" + name + ".tsv")
	if err != nil {
		return nil, err
	}
	m := classMap{}
	sc := bufio.NewScanner(bytes.NewReader(raw))
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) < 2 || !strings.HasPrefix(cols[0], "Q") {
			return nil, fmt.Errorf("maps/%s.tsv:%d: want a Wikidata ID, a tab and tags", name, n)
		}
		if _, dup := m[cols[0]]; dup {
			return nil, fmt.Errorf("maps/%s.tsv:%d: %s mapped twice", name, n, cols[0])
		}
		if cols[1] == "-" {
			m[cols[0]] = nil
			continue
		}
		m[cols[0]] = strings.Split(cols[1], ",")
	}
	return m, sc.Err()
}

func mapsVersion() string {
	h := sha256.New()
	for _, name := range []string{"genres", "types", "adult", "sexual", "countries", "languages"} {
		raw, err := mapFiles.ReadFile("maps/" + name + ".tsv")
		if err != nil {
			panic(err)
		}
		h.Write(raw)
	}
	h.Write([]byte(strings.Join(genreOrder, ",")))
	return hex.EncodeToString(h.Sum(nil))[:12]
}

type rawFacts struct {
	Kind       string
	Runtime    int
	InstanceOf []string
	Genres     []string
	Countries  []string
	Languages  []string
}

type titleFacts struct {
	Genres        []string
	TypeFlags     []string
	Adult         bool
	SexualContent bool
	Countries     []string
	Languages     []string
	Complete      bool
}

type classes map[string]store.ClassAncestryRow

func loadClasses(ctx context.Context, q *store.Queries, r rawFacts) (classes, error) {
	qids := slices.Concat(r.InstanceOf, r.Genres, r.Countries, r.Languages)
	rows, err := q.ClassAncestry(ctx, qids)
	if err != nil {
		return nil, err
	}
	cs := classes{}
	for _, row := range rows {
		cs[row.Qid] = row
	}
	return cs, nil
}

func (cs classes) tags(m classMap, qids ...[]string) ([]string, bool) {
	set := map[string]bool{}
	complete := true
	var walk func(q string, seen map[string]int, depth int)
	walk = func(q string, seen map[string]int, depth int) {
		if d, ok := seen[q]; (ok && d <= depth) || depth > 12 {
			return
		}
		seen[q] = depth
		if t, ok := m[q]; ok {
			for _, x := range t {
				set[x] = true
			}
			return
		}
		c, ok := cs[q]
		if !ok {
			complete = false
			return
		}
		for _, p := range c.Parents {
			walk(p, seen, depth+1)
		}
	}
	for _, list := range qids {
		for _, q := range list {
			walk(q, map[string]int{}, 0)
		}
	}
	out := make([]string, 0, len(set))
	for t := range set {
		out = append(out, t)
	}
	slices.Sort(out)
	return out, complete
}

func (cs classes) code(hand classMap, q string, field func(store.ClassAncestryRow) string) (string, bool) {
	if t, ok := hand[q]; ok && len(t) > 0 {
		return t[0], true
	}
	c, ok := cs[q]
	if !ok {
		return "", false
	}
	return field(c), true
}

func deriveFacts(cs classes, r rawFacts) titleFacts {
	f := titleFacts{Complete: true}
	note := func(ok bool) { f.Complete = f.Complete && ok }

	flags, ok := cs.tags(typeMap, r.InstanceOf, r.Genres)
	note(ok)
	switch {
	case r.Runtime > 40:
		flags = slices.DeleteFunc(flags, func(s string) bool { return s == "short" })
	case r.Kind == "movie" && r.Runtime > 0 && !slices.Contains(flags, "short"):
		flags = append(flags, "short")
	}

	for _, q := range r.Countries {
		code, ok := cs.code(countryMap, q, func(c store.ClassAncestryRow) string { return c.Country })
		note(ok)
		if code != "" && !slices.Contains(f.Countries, code) {
			f.Countries = append(f.Countries, code)
		}
	}
	if len(f.Countries) > 0 && !slices.Contains(f.Countries, "JP") {
		flags = slices.DeleteFunc(flags, func(s string) bool { return s == "anime" })
	}
	slices.Sort(flags)
	f.TypeFlags = flags

	for _, q := range r.Languages {
		tag, ok := cs.code(languageMap, q, func(c store.ClassAncestryRow) string { return c.Language })
		note(ok)
		if tag != "" && !slices.Contains(f.Languages, tag) {
			f.Languages = append(f.Languages, tag)
		}
	}

	votes := map[string]int{}
	for _, q := range r.Genres {
		tags, ok := cs.tags(genreMap, []string{q})
		note(ok)
		for _, t := range tags {
			votes[t]++
		}
	}
	for g := range votes {
		f.Genres = append(f.Genres, g)
	}
	rank := func(g string) int {
		if i := slices.Index(genreOrder, g); i >= 0 {
			return i
		}
		return len(genreOrder)
	}
	slices.SortFunc(f.Genres, func(a, b string) int {
		if votes[a] != votes[b] {
			return votes[b] - votes[a]
		}
		return rank(a) - rank(b)
	})
	f.Genres = f.Genres[:min(len(f.Genres), 4)]

	adult, ok := cs.tags(adultMap, r.InstanceOf, r.Genres)
	note(ok)
	f.Adult = len(adult) > 0
	sexual, ok := cs.tags(sexualMap, r.InstanceOf, r.Genres)
	note(ok)
	f.SexualContent = f.Adult || len(sexual) > 0
	return f
}

func ownLanguageName(names map[string]string, languages []string) string {
	for _, tag := range languages {
		base, _, _ := strings.Cut(tag, "-")
		if m, ok := macroLanguages[base]; ok {
			base = m
		}
		for _, l := range []string{strings.ToLower(tag), base} {
			if n := names[l]; n != "" {
				return n
			}
		}
	}
	return ""
}

func ClassesNeeded(ctx context.Context, db *pgxpool.Pool, qids []string) ([]string, error) {
	return store.New(db).MissingClasses(ctx, qids)
}

type WikidataClass struct {
	QID      string
	Label    string
	Country  string
	Language string
	Parents  []string
}

func SaveClasses(ctx context.Context, db *pgxpool.Pool, cs []WikidataClass) error {
	q := store.New(db)
	for _, c := range cs {
		err := q.SaveClass(ctx, store.SaveClassParams{Qid: c.QID, Label: c.Label, Country: c.Country, Language: c.Language, Parents: append([]string{}, c.Parents...)})
		if err != nil {
			return err
		}
	}
	return nil
}

func OutdatedFacts(ctx context.Context, db *pgxpool.Pool, limit int32) ([]store.OutdatedFactsRow, error) {
	return store.New(db).OutdatedFacts(ctx, store.OutdatedFactsParams{Version: &factsVersion, MaxTitles: limit})
}

func RederiveFacts(ctx context.Context, db *pgxpool.Pool, t store.OutdatedFactsRow) error {
	q := store.New(db)
	r := rawFacts{Kind: t.Kind, InstanceOf: t.WikidataInstanceOf, Genres: t.WikidataGenres, Countries: t.WikidataCountries, Languages: t.WikidataLanguages}
	if t.RuntimeMinutes != nil {
		r.Runtime = int(*t.RuntimeMinutes)
	}
	cs, err := loadClasses(ctx, q, r)
	if err != nil {
		return err
	}
	f := deriveFacts(cs, r)
	return q.DeriveTitleFacts(ctx, store.DeriveTitleFactsParams{
		ID: t.ID, Genres: orEmpty(f.Genres), TypeFlags: orEmpty(f.TypeFlags), Adult: f.Adult, SexualContent: f.SexualContent,
		Countries: orEmpty(f.Countries), Languages: orEmpty(f.Languages), FactsVersion: version(f),
	})
}

func version(f titleFacts) *string {
	if !f.Complete {
		return nil
	}
	return &factsVersion
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
