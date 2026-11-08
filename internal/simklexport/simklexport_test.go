package simklexport

import (
	"archive/zip"
	"bytes"
	"errors"
	"testing"
	"time"

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

const spongebob = `{"title":"SpongeBob SquarePants","poster":"92/928500219b0b490e1","year":1999,"runtime":10,
	"ids":{"simkl":5420,"slug":"spongebob-squarepants","imdb":"tt0206512","tmdb":"387","tvdb":"75886","tvdbslug":"spongebob-squarepants","traktslug":"spongebob-squarepants"}}`

const backup = `{
	"shows": [
		{"added_to_watchlist_at":"2026-11-08T16:38:00Z","last_watched_at":"2026-11-08T16:38:00Z","user_rated_at":null,"user_rating":null,
		 "status":"watching","last_watched":"S01E02","next_to_watch":"S01E03","watched_episodes_count":2,"total_episodes_count":643,"not_aired_episodes_count":7,
		 "show":` + spongebob + `,"is_rewatch":false,
		 "seasons":[{"number":1,"episodes":[{"number":1,"watched_at":"2026-11-08T16:38:00Z"},{"number":2,"watched_at":"2026-11-08T16:50:00Z"}]}]},
		{"last_watched_at":"2026-11-09T10:00:00Z","status":"watching","show":` + spongebob + `,"is_rewatch":true,
		 "seasons":[{"number":1,"episodes":[{"number":1,"watched_at":"2026-11-09T10:00:00Z"}]}]},
		{"added_to_watchlist_at":"2026-11-08T16:41:34Z","last_watched_at":null,"user_rated_at":"2026-11-08T16:41:34Z","user_rating":7,
		 "status":"watching","last_watched":null,"next_to_watch":"S01E01","watched_episodes_count":0,
		 "show":{"title":"Sram","year":2024,"ids":{"simkl":2430829,"imdb":"tt32062031","tmdb":"263208","tvdb":"448820"}}},
		{"added_to_watchlist_at":"2026-10-01T09:00:00Z","status":"dropped","watched_episodes_count":1,
		 "show":{"title":"Dropped Show","year":2020,"ids":{"simkl":1,"tmdb":"100"}},
		 "seasons":[{"number":1,"episodes":[{"number":1,"watched_at":"2026-10-01T09:00:00Z"}]}]},
		{"added_to_watchlist_at":"2026-10-02T09:00:00Z","status":"hold","watched_episodes_count":0,
		 "show":{"title":"Paused Show","year":2021,"ids":{"simkl":2,"tvdb":"200"}}}
	],
	"movies": [
		{"added_to_watchlist_at":"2026-11-08T16:40:46Z","last_watched_at":"2026-11-08T16:40:46Z","user_rated_at":null,"user_rating":null,"status":"completed",
		 "movie":{"title":"Spider-Man: Brand New Day","year":2026,"ids":{"simkl":1902343,"imdb":"tt22084616","tmdb":"969681","tvdb":"135896","letterboxd":"spider-man-brand-new-day"}}},
		{"added_to_watchlist_at":"2026-11-08T16:41:19Z","last_watched_at":null,"status":"plantowatch",
		 "movie":{"title":"Obsession","year":2026,"ids":{"simkl":2558015,"imdb":"tt37287335","tmdb":"1339713"}}},
		{"added_to_watchlist_at":"2026-11-08T16:41:29Z","last_watched_at":"2026-11-08T16:41:29Z","user_rated_at":"2026-11-08T16:41:29Z","user_rating":5,"status":"completed",
		 "movie":{"title":"Resident Evil","year":2026,"ids":{"simkl":2695349,"imdb":"tt35538033","tmdb":"1423191"}}}
	],
	"anime": [
		{"added_to_watchlist_at":"2026-11-08T16:42:06Z","last_watched_at":"2026-11-08T08:00:00Z","user_rating":8,"user_rated_at":"2026-11-08T16:42:06Z",
		 "status":"watching","last_watched":"E1","next_to_watch":"E2","watched_episodes_count":1,
		 "show":{"title":"Ao Ashi Season 2","year":2026,"ids":{"simkl":2767602,"slug":"ao-ashi-season-2","mal":"61603","anilist":"191788","kitsu":"49883","anidb":"19318"}},
		 "anime_type":"tv","seasons":[{"number":1,"episodes":[{"number":1,"watched_at":"2026-11-08T08:00:00Z"}]}]}
	]
}`

func at(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

func TestRead(t *testing.T) {
	r := export(t, map[string]string{"SimklBackup.json": backup})
	f, err := Read(r, r.Size())
	if err != nil {
		t.Fatal(err)
	}

	type play struct {
		label string
		at    time.Time
	}
	var plays []play
	for _, h := range f.History {
		var when time.Time
		if h.At != nil {
			when = *h.At
		}
		plays = append(plays, play{h.Label, when})
	}
	want := []play{
		{"SpongeBob SquarePants (1999) 1x01", at("2026-11-08T16:38:00Z")},
		{"SpongeBob SquarePants (1999) 1x02", at("2026-11-08T16:50:00Z")},
		{"SpongeBob SquarePants (1999) 1x01", at("2026-11-09T10:00:00Z")},
		{"Dropped Show (2020) 1x01", at("2026-10-01T09:00:00Z")},
		{"Spider-Man: Brand New Day (2026)", at("2026-11-08T16:40:46Z")},
		{"Resident Evil (2026)", at("2026-11-08T16:41:29Z")},
		{"Ao Ashi Season 2 (2026) E1", at("2026-11-08T08:00:00Z")},
	}
	if len(plays) != len(want) {
		t.Fatalf("plays = %+v", plays)
	}
	for i := range want {
		if plays[i] != want[i] {
			t.Errorf("play %d = %+v, want %+v", i, plays[i], want[i])
		}
	}

	ep := f.History[1]
	if ep.Ref.Kind != "episode" || ep.Ref.Show != (core.IDs{IMDB: "tt0206512", TMDB: 387, TVDB: 75886}) || *ep.Ref.Season != 1 || *ep.Ref.Number != 2 || ep.Action != "watch" {
		t.Errorf("episode = %+v", ep.Ref)
	}
	if film := f.History[4]; film.Ref.Kind != "movie" || film.Ref.IDs != (core.IDs{IMDB: "tt22084616", TMDB: 969681, TVDB: 135896}) {
		t.Errorf("film = %+v", film.Ref)
	}
	if anime := f.History[6]; anime.Ref.Kind != "episode" || anime.Ref.Show != (core.IDs{}) {
		t.Errorf("anime should have nothing to match on: %+v", anime.Ref)
	}

	if len(f.Watchlist) != 2 || f.Watchlist[0].Label != "Sram (2024)" || f.Watchlist[0].Ref.Kind != "show" || f.Watchlist[1].Label != "Obsession (2026)" || *f.Watchlist[1].At != at("2026-11-08T16:41:19Z") {
		t.Errorf("watchlist = %+v", f.Watchlist)
	}

	if len(f.Ratings) != 3 {
		t.Fatalf("ratings = %+v", f.Ratings)
	}
	if r := f.Ratings[0]; r.Label != "Sram (2024)" || r.Rating != 7 || r.Ref.IDs.TMDB != 263208 || *r.At != at("2026-11-08T16:41:34Z") {
		t.Errorf("show rating = %+v", r)
	}
	if r := f.Ratings[1]; r.Label != "Resident Evil (2026)" || r.Rating != 5 || r.Ref.Kind != "movie" {
		t.Errorf("film rating = %+v", r)
	}
	if r := f.Ratings[2]; r.Label != "Ao Ashi Season 2 (2026)" || r.Ref.Kind != "show" || r.Ref.IDs != (core.IDs{}) {
		t.Errorf("anime rating = %+v", r)
	}

	if len(f.Hidden) != 2 || f.Hidden[0].Section != "dropped" || f.Hidden[0].Label != "Dropped Show (2020)" || f.Hidden[1].Section != "progress_watched" || f.Hidden[1].Ref.IDs.TVDB != 200 {
		t.Errorf("hidden = %+v", f.Hidden)
	}
	if len(f.Playback) != 0 {
		t.Errorf("playback = %+v", f.Playback)
	}
}

func TestReadMissing(t *testing.T) {
	cases := map[string]*bytes.Reader{
		"not a zip":         bytes.NewReader([]byte("hello")),
		"a trakt export":    export(t, map[string]string{"watched-history.json": `[]`}),
		"json of something": export(t, map[string]string{"notes.json": `{"hello":"there"}`}),
	}
	for name, r := range cases {
		if _, err := Read(r, r.Size()); !errors.Is(err, ErrNotAnExport) {
			t.Errorf("%s = %v", name, err)
		}
	}

	r := export(t, map[string]string{"SimklBackup.json": `{"shows": [`})
	var bad BadFileError
	if _, err := Read(r, r.Size()); !errors.As(err, &bad) || bad.Name != "SimklBackup.json" {
		t.Errorf("broken json = %v", err)
	}
}
