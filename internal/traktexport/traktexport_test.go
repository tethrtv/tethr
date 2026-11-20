package traktexport

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"strings"
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

func TestReadLists(t *testing.T) {
	page := func(from, to int) string {
		var items []string
		for i := from; i <= to; i++ {
			items = append(items, fmt.Sprintf(`{"rank":%d,"id":%d,"listed_at":"2026-08-10T18:40:58.000Z","notes":null,"my_rating":null,"type":"movie","movie":{"title":"Film %d","year":2001,"ids":{"trakt":%d,"slug":"film-%d","tmdb":%d,"plex":{"guid":"x","slug":"y"}}}}`, i, i, i, i, i, i))
		}
		return "[" + strings.Join(items, ",") + "]"
	}
	r := export(t, map[string]string{
		"lists-lists.json": `[
			{"name":"Heists","description":"The good ones","privacy":"public","type":"personal","display_numbers":true,"allow_comments":true,"sort_by":"rank","sort_how":"asc","created_at":"2026-08-10T11:22:19.000Z","updated_at":"2026-08-10T18:41:38.000Z","item_count":2,"comment_count":0,"likes":0,"ids":{"slug":"heists","trakt":36790015},"images":{"posters":[]},"user":{"username":"someone"}},
			{"name":"Big one","description":null,"privacy":"friends","sort_by":"added","sort_how":"desc","item_count":3,"ids":{"slug":"big-one-2","trakt":42}},
			{"name":"Empty","description":"","privacy":"private","sort_by":"rank","sort_how":"asc","item_count":0,"ids":{"slug":"empty","trakt":7}},
			{"name":"Failed to download","privacy":"link","ids":{"slug":"gone","trakt":8}},
			{"name":"Lost items","privacy":"private","item_count":12,"ids":{"slug":"lost","trakt":4}}
		]`,
		"lists-list-36790015-heists.json": `[
			{"rank":2,"id":2,"listed_at":"2026-08-10T18:40:58.000Z","notes":" the diner ","type":"movie","movie":{"title":"Heat","year":1995,"ids":{"trakt":1,"slug":"heat-1995","imdb":"tt0113277","tmdb":949}}},
			{"rank":1,"id":1,"listed_at":"2026-08-10T18:40:00.000Z","notes":null,"type":"show","show":` + breakingBad + `},
			{"rank":3,"id":3,"listed_at":"2026-08-10T18:41:00.000Z","notes":null,"type":"person","person":{"name":"Michael Mann","ids":{"trakt":5,"slug":"michael-mann","tmdb":1}}}
		]`,
		"lists-list-42-old-name-1.json": page(1, 2),
		"lists-list-42-old-name-3.json": `[]`,
		"lists-list-42-old-name-2.json": page(3, 3),
		"lists-list-7-empty.json":       `[]`,
		"lists-watchlist.json":          `[]`,
		"likes-lists.json":              `[{"liked_at":"2026-01-01T00:00:00.000Z","type":"list","list":{"name":"Someone else's","ids":{"trakt":99,"slug":"theirs"}}}]`,
		"lists-collaborations.json":     `[{"name":"Shared with me","ids":{"trakt":98,"slug":"shared"}}]`,
		"lists-list-98-shared.json":     page(1, 1),
	})
	f, err := Read(r, r.Size())
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Lists) != 5 {
		t.Fatalf("lists = %+v", f.Lists)
	}
	heists := f.Lists[0]
	if heists.Name != "Heists" || heists.Description != "The good ones" || heists.Privacy != "public" || heists.SortBy != "rank" || heists.SortHow != "asc" {
		t.Errorf("heists = %+v", heists)
	}
	if len(heists.Items) != 3 || heists.Items[0].Ref.Kind != "show" || heists.Items[1].Ref.IDs.TMDB != 949 || heists.Items[1].Notes != "the diner" || heists.Items[1].At == nil {
		t.Errorf("heists items = %+v", heists.Items)
	}
	if heists.Items[2].Ref.Kind != "person" || heists.Items[2].Label != "Michael Mann" {
		t.Errorf("a person on a list = %+v", heists.Items[2])
	}
	big := f.Lists[1]
	if big.Name != "Big one" || big.Privacy != "friends" || big.SortBy != "added" || big.SortHow != "desc" || len(big.Items) != 3 {
		t.Fatalf("big one = %+v", big)
	}
	for i, item := range big.Items {
		if item.Ref.IDs.TMDB != int64(i+1) {
			t.Errorf("big one should be in rank order across pages: %+v", big.Items)
		}
	}
	if f.Lists[2].Name != "Empty" || len(f.Lists[2].Items) != 0 || f.Lists[3].Name != "Failed to download" || len(f.Lists[3].Items) != 0 {
		t.Errorf("empty lists = %+v", f.Lists[2:])
	}
	if lost := f.Lists[4].Items; len(lost) != 12 || lost[0].Ref.Kind != "" || lost[0].Label != "Lost items: its items are missing from the export" || lost[11].Label != "" {
		t.Errorf("a list whose items aren't in the export = %+v", lost)
	}
}

func TestListsAloneAreAnExport(t *testing.T) {
	r := export(t, map[string]string{
		"lists-lists.json":         `[{"name":"Heists","privacy":"private","ids":{"slug":"heists","trakt":1}}]`,
		"lists-list-1-heists.json": `[{"rank":1,"type":"movie","movie":{"title":"Heat","year":1995,"ids":{"tmdb":949}}}]`,
	})
	f, err := Read(r, r.Size())
	if err != nil || len(f.Lists) != 1 || len(f.Lists[0].Items) != 1 {
		t.Errorf("lists only = %+v, %v", f, err)
	}
}

func TestRowsAreCapped(t *testing.T) {
	rows := strings.TrimSuffix(strings.Repeat("{},", core.MaxImportRows+1), ",")
	r := export(t, map[string]string{"watched-history.json": "[" + rows + "]"})
	if _, err := Read(r, r.Size()); !errors.Is(err, core.ErrTooManyRows) {
		t.Errorf("history past the row cap: %v", err)
	}
	r = export(t, map[string]string{"lists-lists.json": `[{"name":"Huge","item_count":2000000000,"ids":{"trakt":1}}]`})
	if _, err := Read(r, r.Size()); !errors.Is(err, core.ErrTooManyRows) {
		t.Errorf("a list claiming two billion missing items: %v", err)
	}
}
