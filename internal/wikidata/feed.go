package wikidata

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
)

type Change struct {
	QID       string
	At        time.Time
	Relevant  bool
	LoggedOut bool
}

func (c *Client) RecentChanges(ctx context.Context, from, to time.Time, cont string) ([]Change, string, error) {
	params := url.Values{
		"action": {"query"}, "list": {"recentchanges"}, "rcnamespace": {"0"}, "rctype": {"edit|new|log"},
		"rcprop": {"title|timestamp|comment|loginfo|user"}, "rclimit": {"500"}, "rcdir": {"newer"},
		"rcstart": {from.UTC().Format(time.RFC3339)}, "rcend": {to.UTC().Format(time.RFC3339)},
	}
	if cont != "" {
		params.Set("rccontinue", cont)
	}
	var res struct {
		Continue struct {
			RC string `json:"rccontinue"`
		} `json:"continue"`
		Query struct {
			RC []struct {
				Type      string  `json:"type"`
				Title     string  `json:"title"`
				Timestamp string  `json:"timestamp"`
				Comment   string  `json:"comment"`
				User      string  `json:"user"`
				Anon      *string `json:"anon"`
				Temp      *string `json:"temp"`
			} `json:"recentchanges"`
		} `json:"query"`
	}
	if err := c.get(ctx, params, &res); err != nil {
		return nil, "", err
	}
	var changes []Change
	for _, rc := range res.Query.RC {
		if !IsQID(rc.Title) {
			continue
		}
		at, err := time.Parse(time.RFC3339, rc.Timestamp)
		if err != nil {
			return nil, "", fmt.Errorf("wikidata api: change to %s: %w", rc.Title, err)
		}
		loggedOut := rc.Anon != nil || rc.Temp != nil || strings.HasPrefix(rc.User, "~") || net.ParseIP(rc.User) != nil
		changes = append(changes, Change{QID: rc.Title, At: at, Relevant: rc.Type == "log" || relevantEdit(rc.Comment), LoggedOut: loggedOut})
	}
	return changes, res.Continue.RC, nil
}

var (
	editAction   = regexp.MustCompile(`^/\* *([a-z-]+)`)
	editProperty = regexp.MustCompile(`\[\[Property:(P[0-9]+)\]\]`)
)

func relevantEdit(comment string) bool {
	m := editAction.FindStringSubmatch(comment)
	if m == nil {
		return true
	}
	action := m[1]
	for _, skip := range []string{"wbsetdescription", "wbsetaliases", "wbsetsitelink", "clientsitelink", "wblinktitles", "wbsetreference", "wbremovereferences", "wbsetqualifier", "wbremovequalifiers"} {
		if strings.HasPrefix(action, skip) {
			return false
		}
	}
	if strings.HasPrefix(action, "wbsetlabel") {
		return true
	}
	props := editProperty.FindAllStringSubmatch(comment, -1)
	if len(props) == 0 {
		return true
	}
	read := readProperties()
	for _, p := range props {
		if slices.Contains(read, p[1]) {
			return true
		}
	}
	return false
}
