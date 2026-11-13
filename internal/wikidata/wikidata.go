package wikidata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	BaseURL   string
	QLeverURL string
	APIURL    string
	UserAgent string
	HTTP      *http.Client
}

type Title struct {
	QID      string
	Name     string
	Original string
	Year     int
	Released time.Time
	Runtime  int
	TMDB     int32
	IDs      map[string][]string

	InstanceOf []string
	Genres     []string
	Countries  []string
	Languages  []string
	Started    string
	Ended      string
	Sitelinks  int
	OtherNames map[string]string
}

type Class struct {
	QID      string
	Label    string
	Country  string
	Language string
	Parents  []string
}

var (
	digits = regexp.MustCompile(`^[0-9]+$`)
	number = regexp.MustCompile(`^[1-9][0-9]{0,8}$`)
	qid    = regexp.MustCompile(`^Q[1-9][0-9]*$`)
)

type outsideID struct {
	movie, show string
	valid       *regexp.Regexp
}

var outsideIDs = map[string]outsideID{
	"imdb":           {"P345", "P345", regexp.MustCompile(`^tt[0-9]{7,9}$`)},
	"tvdb":           {"", "P4835", number},
	"tvmaze":         {"", "P8600", number},
	"letterboxd":     {"P6127", "", regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)},
	"plex":           {"P11460", "P11460", regexp.MustCompile(`^[0-9a-f]{24}$`)},
	"trakt_film":     {"P12492", "", regexp.MustCompile(`^[1-9][0-9]{0,9}$`)},
	"trakt_slug":     {"P8013", "P8013", regexp.MustCompile(`^(movies|shows)/[a-z0-9]+(-[a-z0-9]+)*$`)},
	"eidr":           {"P2704", "P2704", regexp.MustCompile(`^10\.5240/([0-9A-F]{4}-){5}[0-9A-Z]$`)},
	"isan":           {"P3212", "P3212", regexp.MustCompile(`^([0-9A-F]{4}-){2}[0-9A-F]{4}(-[0-9A-F]{4}-[0-9A-Z](-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-Z])?)?$`)},
	"mal":            {"P4086", "P4086", number},
	"anidb":          {"P5646", "P5646", number},
	"anilist":        {"P8729", "P8729", number},
	"rottentomatoes": {"P1258", "P1258", regexp.MustCompile(`^(m|tv)/[a-z0-9_-]+$`)},
	"metacritic":     {"P1712", "P1712", regexp.MustCompile(`^(movie|tv)/[^/\s]+$`)},
	"appletv":        {"P9586", "P9751", regexp.MustCompile(`^umc\.cmc\.[a-z0-9]{20,30}$`)},
	"netflix":        {"P1874", "P1874", regexp.MustCompile(`^[1-9][0-9]{3,9}$`)},
	"primevideo":     {"P8055", "P8055", regexp.MustCompile(`^([A-Z0-9]{10}|[A-Z0-9]{26}|amzn1\.dv\.gti\.[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12})$`)},
	"disneyplus":     {"P7595", "P7596", regexp.MustCompile(`^([A-Za-z0-9]{12,13}|(entity-)?[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12})$`)},
}

func (id outsideID) property(kind string) string {
	if kind == "show" {
		return id.show
	}
	return id.movie
}

func property(kind, provider string) (string, *regexp.Regexp) {
	switch {
	case provider == "tmdb" && kind == "movie":
		return "P4947", digits
	case provider == "tmdb" && kind == "show":
		return "P4983", digits
	}
	if id, ok := outsideIDs[provider]; ok && id.property(kind) != "" {
		return id.property(kind), id.valid
	}
	return "", nil
}

func (c *Client) Find(ctx context.Context, kind, provider string, values []string) (map[string]string, error) {
	prop, valid := property(kind, provider)
	if prop == "" {
		return nil, fmt.Errorf("can't look up a %s by %s id", kind, provider)
	}
	var keys []string
	for _, v := range values {
		if valid.MatchString(v) {
			keys = append(keys, strconv.Quote(v))
		}
	}
	if len(keys) == 0 {
		return map[string]string{}, nil
	}
	query := fmt.Sprintf("SELECT ?key ?item WHERE {\n  VALUES ?key { %s }\n  ?item wdt:%s ?key .\n}", strings.Join(keys, " "), prop)

	var res struct {
		Results struct {
			Bindings []map[string]struct {
				Value string `json:"value"`
			} `json:"bindings"`
		} `json:"results"`
	}
	if err := c.post(ctx, query, &res); err != nil {
		return nil, err
	}

	found := map[string]string{}
	for _, b := range res.Results.Bindings {
		item := b["item"].Value[strings.LastIndex(b["item"].Value, "/")+1:]
		key := b["key"].Value
		if !IsQID(item) {
			continue
		}
		// Duplicate items sometimes claim the same ID; the older one is nearly always the real one.
		if old, ok := found[key]; !ok || qnum(item) < qnum(old) {
			found[key] = item
		}
	}
	return found, nil
}

func (c *Client) entities(ctx context.Context, ids []string) (map[string]entity, error) {
	var res struct {
		Entities map[string]entity `json:"entities"`
	}
	if err := c.get(ctx, url.Values{"action": {"wbgetentities"}, "ids": {strings.Join(ids, "|")}, "props": {"labels|claims|sitelinks"}}, &res); err != nil {
		return nil, err
	}
	if len(res.Entities) == 0 {
		return nil, errors.New("wikidata api: no items in answer")
	}
	return res.Entities, nil
}

func (c *Client) FetchItems(ctx context.Context, qids []string) (map[string]Item, []string, []string, error) {
	var ids []string
	for _, q := range qids {
		if IsQID(q) && !slices.Contains(ids, q) {
			ids = append(ids, q)
		}
	}
	if len(ids) > 50 {
		return nil, nil, nil, fmt.Errorf("wikidata takes 50 items at a time, not %d", len(ids))
	}
	found := map[string]Item{}
	if len(ids) == 0 {
		return found, nil, nil, nil
	}
	entities, err := c.entities(ctx, ids)
	if err != nil {
		return nil, nil, nil, err
	}
	var deleted, unreadable []string
	for _, q := range ids {
		e, ok := entities[q]
		switch {
		case ok && e.Missing != nil:
			deleted = append(deleted, q)
		case !ok, !e.readable():
			unreadable = append(unreadable, q)
		default:
			found[q] = Item{e: e}
		}
	}
	return found, deleted, unreadable, nil
}

func (c *Client) Fetch(ctx context.Context, kind string, qids []string) (map[string]Title, []string, error) {
	var ids []string
	seen := map[string]bool{}
	for _, q := range qids {
		if IsQID(q) && !seen[q] {
			seen[q] = true
			ids = append(ids, q)
		}
	}
	if len(ids) == 0 {
		return map[string]Title{}, nil, nil
	}
	if len(ids) > 50 {
		return nil, nil, fmt.Errorf("wikidata takes 50 items at a time, not %d", len(ids))
	}

	entities, err := c.entities(ctx, ids)
	if err != nil {
		return nil, nil, err
	}
	found := map[string]Title{}
	var deleted []string
	for _, q := range ids {
		e, ok := entities[q]
		switch {
		case !ok || (e.Missing == nil && !e.readable()):
		case e.Missing != nil:
			deleted = append(deleted, q)
		default:
			found[q] = e.title(kind)
		}
	}
	return found, deleted, nil
}

type entity struct {
	ID      string  `json:"id"`
	Missing *string `json:"missing"`
	Labels  map[string]struct {
		Value string `json:"value"`
	} `json:"labels"`
	Claims    map[string]json.RawMessage `json:"claims"`
	Sitelinks map[string]json.RawMessage `json:"sitelinks"`

	sitelinkCount int
}

type snak struct {
	SnakType  string `json:"snaktype"`
	DataValue struct {
		Value json.RawMessage `json:"value"`
	} `json:"datavalue"`
}

func readProperties() []string {
	read := []string{"P31", "P136", "P279", "P297", "P218", "P305", "P364", "P495", "P577", "P580", "P582", "P1476", "P2047", "P4947", "P4983"}
	for _, id := range outsideIDs {
		read = append(read, id.movie, id.show)
	}
	return read
}

func (e entity) readable() bool {
	for _, p := range readProperties() {
		var all []struct {
			MainSnak snak   `json:"mainsnak"`
			Rank     string `json:"rank"`
		}
		if raw, ok := e.Claims[p]; ok && json.Unmarshal(raw, &all) != nil {
			return false
		}
	}
	return true
}

func IsQID(s string) bool {
	return qid.MatchString(s)
}

func (e entity) statements(p string, bestOnly bool) []json.RawMessage {
	var all []struct {
		MainSnak snak   `json:"mainsnak"`
		Rank     string `json:"rank"`
	}
	if err := json.Unmarshal(e.Claims[p], &all); err != nil {
		return nil
	}
	var preferred, normal []json.RawMessage
	for _, s := range all {
		if s.MainSnak.SnakType != "value" {
			continue
		}
		switch s.Rank {
		case "preferred":
			preferred = append(preferred, s.MainSnak.DataValue.Value)
		case "normal":
			normal = append(normal, s.MainSnak.DataValue.Value)
		}
	}
	if bestOnly && len(preferred) > 0 {
		return preferred
	}
	return append(preferred, normal...)
}

func (e entity) strings(p string, valid *regexp.Regexp) []string {
	var out []string
	for _, raw := range e.statements(p, true) {
		var v string
		if json.Unmarshal(raw, &v) == nil && valid.MatchString(v) {
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return slices.Compact(out)
}

func (e entity) title(kind string) Title {
	t := Title{QID: e.ID}

	var originals []string
	for _, raw := range e.statements("P1476", true) {
		var v struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(raw, &v) == nil && v.Text != "" {
			originals = append(originals, v.Text)
		}
	}
	sort.Strings(originals)
	if len(originals) > 0 {
		t.Original = originals[0]
	}

	dates := e.statements("P577", false)
	if kind == "show" {
		dates = append(dates, e.statements("P580", false)...)
	}
	for _, raw := range dates {
		year, released, ok := releaseDate(raw)
		if !ok {
			continue
		}
		if t.Year == 0 || year < t.Year {
			t.Year = year
		}
		if t.Released.IsZero() || released.Before(t.Released) {
			t.Released = released
		}
	}

	t.Name = trimDisambiguator(e.name(t.Original), t.Year)
	if e.label("en") == "" && e.label("mul") == "" && t.Original == "" {
		t.OtherNames = map[string]string{}
		for lang := range e.Labels {
			if v := e.label(lang); v != "" {
				t.OtherNames[lang] = trimDisambiguator(v, t.Year)
			}
		}
	}
	t.Runtime = e.runtime()
	t.InstanceOf = e.items("P31")
	t.Genres = e.items("P136")
	t.Countries = e.items("P495")
	t.Languages = e.items("P364")
	t.Sitelinks = max(len(e.Sitelinks), e.sitelinkCount)
	if kind == "show" {
		for _, raw := range e.statements("P580", false) {
			if d, ok := partialDate(raw); ok && (t.Started == "" || d < t.Started) {
				t.Started = d
			}
		}
		for _, raw := range e.statements("P582", false) {
			if d, ok := partialDate(raw); ok && d > t.Ended {
				t.Ended = d
			}
		}
	}

	tmdbProp, _ := property(kind, "tmdb")
	for _, v := range e.strings(tmdbProp, digits) {
		if id, err := strconv.ParseInt(v, 10, 32); err == nil && id > 0 && (t.TMDB == 0 || int32(id) < t.TMDB) {
			t.TMDB = int32(id)
		}
	}
	t.IDs = map[string][]string{}
	for provider, id := range outsideIDs {
		if id.property(kind) == "" {
			continue
		}
		if values := e.strings(id.property(kind), id.valid); len(values) > 0 {
			t.IDs[provider] = values
		}
	}
	return t
}

var fallbackLanguages = []string{"en-gb", "en-ca", "en-us", "fr", "de", "es", "it", "nl", "pt", "sv", "da", "nb", "pl", "cs", "ca", "ga", "cy"}

// Some batch edits set an item's label to its own ID.
func (e entity) label(lang string) string {
	if v := e.Labels[lang].Value; v != e.ID {
		return v
	}
	return ""
}

func (e entity) name(original string) string {
	if v := e.label("en"); v != "" {
		return v
	}
	if v := e.label("mul"); v != "" {
		return v
	}
	if original != "" {
		return original
	}
	for _, l := range fallbackLanguages {
		if v := e.label(l); v != "" {
			return v
		}
	}
	langs := make([]string, 0, len(e.Labels))
	for l := range e.Labels {
		if e.label(l) != "" {
			langs = append(langs, l)
		}
	}
	if len(langs) == 0 {
		return ""
	}
	sort.Strings(langs)
	return e.label(langs[0])
}

func trimDisambiguator(name string, year int) string {
	i := strings.LastIndex(name, " (")
	if i <= 0 || !strings.HasSuffix(name, ")") {
		return name
	}
	inside := strings.ToLower(name[i+2 : len(name)-1])
	if year > 0 && strings.Contains(inside, strconv.Itoa(year)) {
		return name[:i]
	}
	for _, w := range []string{"film", "series", "tv", "serial", "movie"} {
		if strings.Contains(inside, w) {
			return name[:i]
		}
	}
	return name
}

func parseDate(raw json.RawMessage) (y, m, d int, ok bool) {
	var v struct {
		Time      string `json:"time"`
		Precision int    `json:"precision"`
	}
	if json.Unmarshal(raw, &v) != nil || v.Precision < 9 || !strings.HasPrefix(v.Time, "+") {
		return 0, 0, 0, false
	}
	parts := strings.SplitN(strings.TrimPrefix(v.Time, "+"), "-", 3)
	if len(parts) < 3 || len(parts[2]) < 2 {
		return 0, 0, 0, false
	}
	y, errY := strconv.Atoi(parts[0])
	m, errM := strconv.Atoi(parts[1])
	d, errD := strconv.Atoi(parts[2][:2])
	if errY != nil || errM != nil || errD != nil || y <= 1800 || y >= 2200 || m > 12 || d > 31 {
		return 0, 0, 0, false
	}
	if v.Precision == 9 {
		m = 0
	}
	if v.Precision <= 10 {
		d = 0
	}
	return y, m, d, true
}

func releaseDate(raw json.RawMessage) (int, time.Time, bool) {
	y, m, d, ok := parseDate(raw)
	if !ok {
		return 0, time.Time{}, false
	}
	// A date known only to the year or month counts as released once that year or month is over.
	switch {
	case m == 0:
		return y, time.Date(y+1, 1, 1, 0, 0, 0, 0, time.UTC), true
	case d == 0:
		return y, time.Date(y, time.Month(m)+1, 1, 0, 0, 0, 0, time.UTC), true
	}
	return y, time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC), true
}

func partialDate(raw json.RawMessage) (string, bool) {
	y, m, d, ok := parseDate(raw)
	switch {
	case !ok:
		return "", false
	case m == 0:
		return fmt.Sprintf("%04d", y), true
	case d == 0:
		return fmt.Sprintf("%04d-%02d", y, m), true
	}
	return fmt.Sprintf("%04d-%02d-%02d", y, m, d), true
}

func (e entity) items(p string) []string {
	var out []string
	for _, raw := range e.statements(p, true) {
		var v struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(raw, &v) == nil && IsQID(v.ID) && !slices.Contains(out, v.ID) {
			out = append(out, v.ID)
		}
	}
	return out
}

func (e entity) first(p string) string {
	for _, raw := range e.statements(p, true) {
		var v string
		if json.Unmarshal(raw, &v) == nil && v != "" {
			return v
		}
	}
	return ""
}

func (c *Client) FetchClasses(ctx context.Context, qids []string) (map[string]Class, []string, error) {
	var ids []string
	for _, q := range qids {
		if IsQID(q) && !slices.Contains(ids, q) {
			ids = append(ids, q)
		}
	}
	if len(ids) > 50 {
		return nil, nil, fmt.Errorf("wikidata takes 50 items at a time, not %d", len(ids))
	}
	found := map[string]Class{}
	var unreadable []string
	if len(ids) == 0 {
		return found, nil, nil
	}
	var res struct {
		Entities map[string]entity `json:"entities"`
	}
	if err := c.get(ctx, url.Values{"action": {"wbgetentities"}, "ids": {strings.Join(ids, "|")}, "props": {"labels|claims"}, "languages": {"en"}}, &res); err != nil {
		return nil, nil, err
	}
	if len(res.Entities) == 0 {
		return nil, nil, errors.New("wikidata api: no items in answer")
	}
	for _, q := range ids {
		e, ok := res.Entities[q]
		if !ok || (e.Missing == nil && !e.readable()) {
			found[q] = Class{QID: q}
			unreadable = append(unreadable, q)
			continue
		}
		class := Class{QID: q, Label: e.Labels["en"].Value, Country: e.first("P297"), Language: e.first("P305"), Parents: e.items("P279")}
		if class.Language == "" {
			class.Language = e.first("P218")
		}
		if e.Missing == nil && e.ID != q {
			class.Parents = append([]string{e.ID}, class.Parents...)
		}
		found[q] = class
	}
	return found, unreadable, nil
}

func (e entity) runtime() int {
	best := 0.0
	for _, raw := range e.statements("P2047", true) {
		var q struct {
			Amount string `json:"amount"`
			Unit   string `json:"unit"`
		}
		if json.Unmarshal(raw, &q) != nil {
			continue
		}
		a, err := strconv.ParseFloat(strings.TrimPrefix(q.Amount, "+"), 64)
		if err != nil {
			continue
		}
		switch q.Unit[strings.LastIndex(q.Unit, "/")+1:] {
		case "Q7727":
		case "Q11574":
			a /= 60
		case "Q25235":
			a *= 60
		default:
			continue
		}
		if a >= 1 && a < 1000 && (best == 0 || a < best) {
			best = a
		}
	}
	return int(math.Round(best))
}

func qnum(qid string) int {
	n, err := strconv.Atoi(strings.TrimPrefix(qid, "Q"))
	if err != nil {
		return int(^uint(0) >> 1)
	}
	return n
}

func (c *Client) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: time.Minute}
}

var ErrBusy = errors.New("wikidata is busy")

func (c *Client) get(ctx context.Context, params url.Values, v any) error {
	base := c.APIURL
	if base == "" {
		base = "https://www.wikidata.org/w/api.php"
	}
	params.Set("format", "json")
	// Wikidata folds query service lag into maxlag, so a low value refuses plain reads whenever the query service falls behind.
	params.Set("maxlag", "60")
	raw, err := c.send(ctx, func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, base+"?"+params.Encode(), nil)
	})
	if err != nil {
		return fmt.Errorf("wikidata api: %w", err)
	}
	var body struct {
		Error *struct {
			Code string `json:"code"`
			Info string `json:"info"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return fmt.Errorf("wikidata api: %w", err)
	}
	if body.Error != nil {
		return fmt.Errorf("wikidata api: %s: %s", body.Error.Code, body.Error.Info)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("wikidata api: %w", err)
	}
	return nil
}

func (c *Client) post(ctx context.Context, query string, v any) error {
	base := c.BaseURL
	if base == "" {
		base = "https://query.wikidata.org/sparql"
	}
	return c.postTo(ctx, base, query, v)
}

func (c *Client) postTo(ctx context.Context, base, query string, v any) error {
	return c.postWith(ctx, c.client(), base, query, v)
}

func (c *Client) postWith(ctx context.Context, hc *http.Client, base, query string, v any) error {
	raw, err := c.sendWith(ctx, hc, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, base, strings.NewReader(url.Values{"query": {query}}.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/sparql-results+json")
		return req, nil
	})
	if err != nil {
		return fmt.Errorf("wikidata query: %w", err)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("wikidata query: %w", err)
	}
	return nil
}

func (c *Client) send(ctx context.Context, build func() (*http.Request, error)) ([]byte, error) {
	return c.sendWith(ctx, c.client(), build)
}

func (c *Client) sendWith(ctx context.Context, hc *http.Client, build func() (*http.Request, error)) ([]byte, error) {
	for attempt := 1; ; attempt++ {
		req, err := build()
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", c.UserAgent)
		resp, err := hc.Do(req)
		if err != nil {
			return nil, err
		}
		raw, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading answer: %w", err)
		}
		lagged := busy(resp, raw)
		switch {
		case !lagged && resp.StatusCode != http.StatusOK:
			return nil, fmt.Errorf("%s: %.300s", resp.Status, bytes.TrimSpace(raw))
		case !lagged:
			return raw, nil
		}
		pause, ok := retryAfter(resp.Header.Get("Retry-After"))
		if attempt == 3 || !ok {
			return nil, fmt.Errorf("%w: %s, retry-after %q, try %d", ErrBusy, resp.Status, resp.Header.Get("Retry-After"), attempt)
		}
		t := time.NewTimer(pause)
		select {
		case <-ctx.Done():
			t.Stop()
			return nil, ctx.Err()
		case <-t.C:
		}
	}
}

func busy(resp *http.Response, raw []byte) bool {
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
		return true
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	return json.Unmarshal(raw, &body) == nil && body.Error.Code == "maxlag"
}

func retryAfter(header string) (time.Duration, bool) {
	if header == "" {
		return time.Minute, true
	}
	secs, err := strconv.Atoi(header)
	if err != nil || secs < 0 || secs > 120 {
		return 0, false
	}
	return time.Duration(max(secs, 1)) * time.Second, true
}
