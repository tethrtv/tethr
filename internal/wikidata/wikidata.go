package wikidata

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	BaseURL   string
	UserAgent string
	HTTP      *http.Client
}

type Title struct {
	QID      string
	Name     string
	Original string
	Year     int
	Runtime  int
	TMDB     int32
	IMDB     string
	TVDB     string
}

var (
	digits = regexp.MustCompile(`^[0-9]+$`)
	imdbID = regexp.MustCompile(`^tt[0-9]+$`)
)

func property(kind, provider string) (string, *regexp.Regexp) {
	switch {
	case provider == "tmdb" && kind == "movie":
		return "P4947", digits
	case provider == "tmdb" && kind == "show":
		return "P4983", digits
	case provider == "imdb":
		return "P345", imdbID
	case provider == "tvdb" && kind == "show":
		return "P4835", digits
	}
	return "", nil
}

func (c *Client) Find(ctx context.Context, kind, provider string, values []string) (map[string]Title, error) {
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
		return map[string]Title{}, nil
	}

	tmdbProp, started, tvdb := "P4947", "wdt:P577", ""
	if kind == "show" {
		tmdbProp, started, tvdb = "P4983", "wdt:P580|wdt:P577", "OPTIONAL { ?item wdt:P4835 ?tvdb }"
	}
	query := fmt.Sprintf(`SELECT ?key ?item (MIN(STR(?en)) AS ?name) (MIN(STR(?mul)) AS ?fallback) (MIN(STR(?orig)) AS ?original)
  (YEAR(MIN(?date)) AS ?year) (MIN(?minutes) AS ?runtime) (MIN(?tmdb) AS ?tmdbID) (MIN(?imdb) AS ?imdbID) (MIN(?tvdb) AS ?tvdbID)
WHERE {
  VALUES ?key { %s }
  ?item wdt:%s ?key .
  OPTIONAL { ?item rdfs:label ?en . FILTER(LANG(?en) = "en") }
  OPTIONAL { ?item rdfs:label ?mul . FILTER(LANG(?mul) = "mul") }
  OPTIONAL { ?item wdt:P1476 ?orig }
  OPTIONAL { ?item %s ?date }
  OPTIONAL {
    ?item p:P2047 ?length . ?length a wikibase:BestRank ; psv:P2047 [ wikibase:quantityAmount ?amount ; wikibase:quantityUnit ?unit ] .
    BIND(IF(?unit = wd:Q7727, ?amount, IF(?unit = wd:Q11574, ?amount / 60, IF(?unit = wd:Q25235, ?amount * 60, ?unknown))) AS ?minutes)
  }
  OPTIONAL { ?item wdt:%s ?tmdb }
  OPTIONAL { ?item wdt:P345 ?imdb }
  %s
}
GROUP BY ?key ?item`, strings.Join(keys, " "), prop, started, tmdbProp, tvdb)

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

	found := map[string]Title{}
	for _, b := range res.Results.Bindings {
		t := Title{
			QID:      b["item"].Value[strings.LastIndex(b["item"].Value, "/")+1:],
			Name:     b["name"].Value,
			Original: b["original"].Value,
			IMDB:     b["imdbID"].Value,
			TVDB:     b["tvdbID"].Value,
		}
		if t.Name == "" {
			t.Name = b["fallback"].Value
		}
		if y, err := strconv.Atoi(b["year"].Value); err == nil && y > 1800 && y < 2200 {
			t.Year = y
			t.Name = strings.TrimSuffix(t.Name, fmt.Sprintf(" (%d)", y))
		}
		if m, err := strconv.ParseFloat(b["runtime"].Value, 64); err == nil && m >= 1 && m < 1000 {
			t.Runtime = int(math.Round(m))
		}
		if id, err := strconv.ParseInt(b["tmdbID"].Value, 10, 32); err == nil {
			t.TMDB = int32(id)
		}
		if !imdbID.MatchString(t.IMDB) {
			t.IMDB = ""
		}
		if !digits.MatchString(t.TVDB) {
			t.TVDB = ""
		}
		key := b["key"].Value
		// Duplicate items sometimes claim the same ID; the older one is nearly always the real one.
		if old, ok := found[key]; !ok || qnum(t.QID) < qnum(old.QID) {
			found[key] = t
		}
	}
	return found, nil
}

func qnum(qid string) int {
	n, err := strconv.Atoi(strings.TrimPrefix(qid, "Q"))
	if err != nil {
		return int(^uint(0) >> 1)
	}
	return n
}

func (c *Client) post(ctx context.Context, query string, v any) error {
	base := c.BaseURL
	if base == "" {
		base = "https://query.wikidata.org/sparql"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base, strings.NewReader(url.Values{"query": {query}}.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/sparql-results+json")
	req.Header.Set("User-Agent", c.UserAgent)
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: time.Minute}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("wikidata query: %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}
