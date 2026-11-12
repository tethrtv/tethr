package wikidata

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const prefixes = "PREFIX wd: <http://www.wikidata.org/entity/>\nPREFIX wdt: <http://www.wikidata.org/prop/direct/>\n"

func (c *Client) ListTitles(ctx context.Context, classes []string) ([]string, error, error) {
	var values []string
	for _, q := range classes {
		if IsQID(q) {
			values = append(values, "wd:"+q)
		}
	}
	queries := []string{
		prefixes + "SELECT ?item WHERE { { ?item wdt:P4947 [] } UNION { ?item wdt:P4983 [] } }",
		prefixes + fmt.Sprintf("SELECT ?item WHERE { VALUES ?class { %s } ?item wdt:P31 ?class . { ?item wdt:P345 [] } UNION { ?item wdt:P4835 [] } UNION { ?item wdt:P8600 [] } MINUS { ?item wdt:P4947 [] } MINUS { ?item wdt:P4983 [] } }", strings.Join(values, " ")),
	}
	seen := map[string]bool{}
	var out []string
	var fellBack error
	for i, query := range queries {
		qids, primary, err := c.list(ctx, query, i == 0)
		if err != nil {
			return nil, nil, err
		}
		fellBack = errors.Join(fellBack, primary)
		for _, q := range qids {
			if !seen[q] {
				seen[q] = true
				out = append(out, q)
			}
		}
	}
	return out, fellBack, nil
}

func (c *Client) list(ctx context.Context, query string, never0 bool) ([]string, error, error) {
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 5 * time.Minute}
	}
	ask := func(base string) ([]string, error) {
		var res struct {
			Results struct {
				Bindings []map[string]struct {
					Value string `json:"value"`
				} `json:"bindings"`
			} `json:"results"`
		}
		if err := c.postWith(ctx, hc, base, query, &res); err != nil {
			return nil, err
		}
		var qids []string
		for _, b := range res.Results.Bindings {
			v := b["item"].Value
			if q := v[strings.LastIndex(v, "/")+1:]; IsQID(q) {
				qids = append(qids, q)
			}
		}
		if never0 && len(qids) == 0 {
			return nil, errors.New("wikidata query: an empty answer to a list that is never empty")
		}
		return qids, nil
	}
	primary := c.BaseURL
	if primary == "" {
		primary = "https://query.wikidata.org/sparql"
	}
	qids, err := ask(primary)
	if err == nil || ctx.Err() != nil {
		return qids, nil, err
	}
	fallback := c.QLeverURL
	if fallback == "" {
		fallback = "https://qlever.dev/api/wikidata"
	}
	qids, ferr := ask(fallback)
	if ferr != nil {
		return nil, nil, fmt.Errorf("qlever: %w (after the query service failed: %s)", ferr, err.Error())
	}
	return qids, err, nil
}
