package mdblist

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fake(t *testing.T, pages map[string]string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("apikey") != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		key := r.URL.Path
		if c := r.URL.Query().Get("cursor"); c != "" {
			key += "@" + c
		}
		body, ok := pages[key]
		if !ok {
			body = "{}"
			if r.URL.Path == "/sync/playback" || r.URL.Path == "/lists/user" {
				body = "[]"
			}
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return &Client{BaseURL: srv.URL}
}

func TestFetch(t *testing.T) {
	c := fake(t, map[string]string{
		"/sync/watched": `{"movies":[{"watched_at":"2024-01-01T20:00:00Z","movie":{"title":"Inception","year":2010,"ids":{"imdb":"tt1375666","tmdb":27205,"trakt":16662}}}],
			"episodes":[],"pagination":{"next_cursor":"two"}}`,
		"/sync/watched@two": `{"movies":[],"episodes":[{"watched_at":"2024-02-01T20:00:00Z","episode":{"season":1,"number":2,"ids":{"tmdb":62086},
			"show":{"title":"Breaking Bad","year":2008,"ids":{"tmdb":1396,"tvdb":81189}}}},
			{"watched_at":"2024-02-02T20:00:00Z","episode":{"season":3,"number":1}}],"pagination":{"next_cursor":null}}`,
		"/sync/ratings": `{"movies":[{"rated_at":"2024-01-02T10:00:00Z","rating":9,"movie":{"title":"Inception","ids":{"tmdb":27205}}}],
			"shows":[{"rating":0,"show":{"title":"Unrated","ids":{"tmdb":1}}}],
			"seasons":[{"rating":"7","show":{"title":"Breaking Bad","ids":{"tmdb":1396}},"season":{"number":1}}]}`,
		"/watchlist/items": `{"movies":[{"title":"Heat","release_year":1995,"mediatype":"movie","imdb_id":"tt0113277","watchlist_at":"2026-11-26T07:19:20.208Z","ids":{"tmdb":949}}],
			"shows":[{"title":"The Wire","release_year":2002,"ids":{"tmdb":1438,"tvdb":79126}}]}`,
		"/sync/playback": `[{"progress":42.5,"paused_at":"2024-03-01T20:00:00Z","type":"movie","movie":{"title":"Heat","ids":{"tmdb":949}}},
			{"progress":"12.5","type":"movie","movie":{"title":"Inception","ids":{"tmdb":27205}}},
			{"progress":"","type":"movie","movie":{"title":"Lola","ids":{"tmdb":1}}}]`,
		"/sync/dropped": `{"shows":[{"dropped_at":"2024-04-01T00:00:00Z","on_hold":false,"show":{"title":"Lost","ids":{"tmdb":4607}}},
			{"on_hold":true,"show":{"title":"Dark","ids":{"tmdb":70523}}}]}`,
		"/lists/user": `[{"id":11,"name":"Heists","private":false,"dynamic":false},{"id":12,"name":"Top films","dynamic":true}]`,
		"/lists/11/items": `{"movies":[{"title":"Heat","release_year":1995,"mediatype":"movie","rank":2,"ids":{"tmdb":949}}],
			"shows":[{"title":"The Wire","release_year":2002,"mediatype":"show","rank":1,"ids":{"tmdb":1438}}]}`,
		"/lists/12/items": `{"movies":[{"title":"Inception","release_year":2010,"mediatype":"movie","ids":{"tmdb":27205}}]}`,
	})
	f, err := c.Fetch(context.Background(), "secret")
	if err != nil {
		t.Fatal(err)
	}

	if len(f.History) != 3 || f.History[2].Label != "Unknown show 3x01" || f.History[0].Ref.IDs.TMDB != 27205 || f.History[0].Ref.IDs.Trakt != 16662 || f.History[0].Label != "Inception (2010)" {
		t.Errorf("history = %+v", f.History)
	}
	ep := f.History[1]
	if ep.Ref.Kind != "episode" || ep.Ref.Show.TVDB != 81189 || *ep.Ref.Season != 1 || *ep.Ref.Number != 2 || ep.Label != "Breaking Bad (2008) 1x02" {
		t.Errorf("episode = %+v", ep)
	}
	if len(f.Ratings) != 2 || f.Ratings[0].Rating != 9 || f.Ratings[1].Ref.Kind != "season" || f.Ratings[1].Rating != 7 {
		t.Errorf("ratings = %+v", f.Ratings)
	}
	if len(f.Watchlist) != 2 || f.Watchlist[0].Ref.IDs.IMDB != "tt0113277" || f.Watchlist[0].At == nil || f.Watchlist[1].Ref.Kind != "show" || f.Watchlist[1].Ref.IDs.TVDB != 79126 {
		t.Errorf("watchlist = %+v", f.Watchlist)
	}
	if len(f.Playback) != 2 || f.Playback[0].Progress != 42.5 || f.Playback[1].Progress != 12.5 {
		t.Errorf("playback = %+v", f.Playback)
	}
	if len(f.Hidden) != 2 || f.Hidden[0].Section != "dropped" || f.Hidden[1].Section != "progress_watched" {
		t.Errorf("hidden = %+v", f.Hidden)
	}
	if len(f.Lists) != 2 || f.Lists[0].Privacy != "public" || f.Lists[0].Items[0].Label != "The Wire (2002)" || f.Lists[0].Snapshot || !f.Lists[1].Snapshot || f.Lists[1].Privacy != "" || len(f.Lists[1].Items) != 1 {
		t.Errorf("lists = %+v", f.Lists)
	}
}

func TestFetchErrors(t *testing.T) {
	c := fake(t, nil)
	if _, err := c.Fetch(context.Background(), "wrong"); !errors.Is(err, ErrBadKey) {
		t.Errorf("wrong key = %v", err)
	}

	limited := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer limited.Close()
	if _, err := (&Client{BaseURL: limited.URL}).Fetch(context.Background(), "secret"); !errors.Is(err, ErrLimited) {
		t.Errorf("limited = %v", err)
	}

	gone := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	gone.Close()
	_, err := (&Client{BaseURL: gone.URL}).Fetch(context.Background(), "secret")
	if !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "secret") {
		t.Errorf("unreachable = %v", err)
	}
}

func TestFetchNeedsACursorWhenThereIsMore(t *testing.T) {
	c := fake(t, map[string]string{"/sync/watched": `{"movies":[],"pagination":{"has_more":true}}`})
	if _, err := c.Fetch(context.Background(), "secret"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("more without a cursor = %v", err)
	}
}

func TestFetchStopsOnEndlessPages(t *testing.T) {
	c := fake(t, map[string]string{
		"/sync/watched":       `{"movies":[],"pagination":{"next_cursor":"again"}}`,
		"/sync/watched@again": `{"movies":[],"pagination":{"next_cursor":"again"}}`,
	})
	if _, err := c.Fetch(context.Background(), "secret"); err == nil {
		t.Error("a cursor that never ends should stop the fetch")
	}
}

func TestFetchLaterForbiddenIsNotABadKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sync/ratings" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()
	_, err := (&Client{BaseURL: srv.URL}).Fetch(context.Background(), "secret")
	if errors.Is(err, ErrBadKey) || !errors.Is(err, ErrUnavailable) {
		t.Errorf("forbidden later = %v", err)
	}
}
