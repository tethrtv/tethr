package traktexport

import (
	"archive/zip"
	"bytes"
	"errors"
	"testing"

	"github.com/tethrtv/tethr/internal/core"
)

func export(t *testing.T, files map[string]string) *bytes.Reader {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(buf.Bytes())
}

const breakingBad = `{"title":"Breaking Bad","year":2008,"ids":{"trakt":1388,"slug":"breaking-bad","tvdb":81189,"imdb":"tt0903747","tmdb":1396}}`

func TestRead(t *testing.T) {
	r := export(t, map[string]string{
		"watched-history.json": `[
			{"id":9123456789,"watched_at":"2024-02-01T20:00:00.000Z","action":"scrobble","type":"episode",
			 "episode":{"season":1,"number":2,"title":"Cat's in the Bag...","ids":{"trakt":62086,"tvdb":349233,"imdb":"tt1054724","tmdb":62086}},
			 "show":` + breakingBad + `},
			{"id":9123456790,"watched_at":"2024-01-01T20:00:00.000Z","action":"watch","type":"movie",
			 "movie":{"title":"Inception","year":2010,"ids":{"trakt":16662,"slug":"inception-2010","imdb":"tt1375666","tmdb":27205}}}
		]`,
		"lists-watchlist.json": `[
			{"rank":2,"id":1467360778,"listed_at":"2026-05-16T10:32:33.000Z","notes":null,"type":"movie","movie":{"title":"Apex","year":2026,"ids":{"trakt":1077748,"slug":"apex-2026","imdb":"tt16431404","tmdb":1318447,"plex":{"guid":"x","slug":"apex-2026"}}}},
			{"rank":1,"id":1467360616,"listed_at":"2026-05-16T10:30:12.000Z","notes":" soon ","type":"season","season":{"number":2,"ids":{"trakt":5}},"show":` + breakingBad + `}
		]`,
		"watched-playback.json":              `[{"progress":42.5,"paused_at":"2024-02-03T20:00:00.000Z","id":1,"type":"movie","movie":{"title":"Heat","year":1995,"ids":{"trakt":1,"tmdb":949}}}]`,
		"ratings-seasons.json":               `[{"rated_at":"2024-01-02T10:00:00.000Z","rating":9,"type":"season","season":{"number":1,"ids":{"trakt":3}},"show":` + breakingBad + `}]`,
		"hidden-progress-watched-reset.json": `[{"hidden_at":"2025-01-01T00:00:00.000Z","type":"show","show":` + breakingBad + `}]`,
		"user-settings.json":                 `{"user":{"username":"someone"}}`,
		"ratings-movies.json":                `[]`,
	})
	f, err := Read(r, r.Size())
	if err != nil {
		t.Fatal(err)
	}
	if len(f.History) != 2 || len(f.Watchlist) != 2 || len(f.Playback) != 1 || len(f.Ratings) != 1 || len(f.Hidden) != 1 {
		t.Fatalf("file = %+v", f)
	}

	ep := f.History[0]
	if ep.Ref.Kind != "episode" || *ep.Ref.Season != 1 || *ep.Ref.Number != 2 || ep.Ref.IDs != (core.IDs{TMDB: 62086}) || ep.Action != "scrobble" || ep.At == nil {
		t.Errorf("episode = %+v", ep)
	}
	if ep.Ref.Show != (core.IDs{Trakt: 1388, IMDB: "tt0903747", TMDB: 1396, TVDB: 81189}) {
		t.Errorf("show IDs should keep Trakt's own along with TMDB, IMDb and TVDB: %+v", ep.Ref.Show)
	}
	if ep.Label != "Breaking Bad (2008) 1x02" || f.History[1].Label != "Inception (2010)" {
		t.Errorf("labels = %q, %q", ep.Label, f.History[1].Label)
	}
	if f.Watchlist[0].Ref.Kind != "season" || f.Watchlist[0].Notes != "soon" || f.Watchlist[1].Ref.IDs.TMDB != 1318447 {
		t.Errorf("watchlist should be in rank order: %+v", f.Watchlist)
	}
	if f.Playback[0].Progress != 42.5 || f.Ratings[0].Rating != 9 || *f.Ratings[0].Ref.Season != 1 {
		t.Errorf("playback = %+v, ratings = %+v", f.Playback, f.Ratings)
	}
	if f.Hidden[0].Section != "progress_watched_reset" || f.Hidden[0].Ref.Kind != "show" {
		t.Errorf("hidden = %+v", f.Hidden)
	}
}

func TestNotAnExport(t *testing.T) {
	r := bytes.NewReader([]byte("not a zip"))
	if _, err := Read(r, r.Size()); !errors.Is(err, ErrNotAnExport) {
		t.Errorf("plain text: %v", err)
	}
	z := export(t, map[string]string{"photo.jpg": "x"})
	if _, err := Read(z, z.Size()); !errors.Is(err, ErrNotAnExport) {
		t.Errorf("zip without export files: %v", err)
	}
	bad := export(t, map[string]string{"watched-history.json": `{"oops"`})
	if _, err := Read(bad, bad.Size()); err == nil || errors.Is(err, ErrNotAnExport) {
		t.Errorf("broken JSON: %v", err)
	}
}
