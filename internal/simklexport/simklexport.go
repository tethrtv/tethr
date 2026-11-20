package simklexport

import (
	"archive/zip"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/tethrtv/tethr/internal/core"
)

var ErrNotAnExport = errors.New("that isn't a Simkl backup")

type BadFileError struct {
	Name string
	Err  error
}

func (e BadFileError) Error() string { return e.Name + ": " + e.Err.Error() }
func (e BadFileError) Unwrap() error { return e.Err }

const maxUnpacked = 32 << 20

type id string

func (i *id) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*i = id(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*i = id(n.String())
	return nil
}

func (i id) number() int64 {
	n, _ := strconv.ParseInt(string(i), 10, 64)
	return n
}

type media struct {
	Title string `json:"title"`
	Year  *int   `json:"year"`
	IDs   struct {
		IMDB id `json:"imdb"`
		TMDB id `json:"tmdb"`
		TVDB id `json:"tvdb"`
	} `json:"ids"`
}

func (m media) ids() core.IDs {
	return core.IDs{IMDB: string(m.IDs.IMDB), TMDB: m.IDs.TMDB.number(), TVDB: m.IDs.TVDB.number()}
}

func (m media) label() string {
	name := cmp.Or(m.Title, "Untitled")
	if m.Year != nil {
		return fmt.Sprintf("%s (%d)", name, *m.Year)
	}
	return name
}

type entry struct {
	Status        string     `json:"status"`
	AddedAt       *time.Time `json:"added_to_watchlist_at"`
	LastWatchedAt *time.Time `json:"last_watched_at"`
	Rating        int16      `json:"user_rating"`
	RatedAt       *time.Time `json:"user_rated_at"`
	WatchedCount  int        `json:"watched_episodes_count"`
	Rewatch       bool       `json:"is_rewatch"`
	Show          *media     `json:"show"`
	Movie         *media     `json:"movie"`
	Seasons       []struct {
		Number   int32 `json:"number"`
		Episodes []struct {
			Number    int32      `json:"number"`
			WatchedAt *time.Time `json:"watched_at"`
		} `json:"episodes"`
	} `json:"seasons"`
}

func Read(r io.ReaderAt, size int64) (core.ImportFile, error) {
	var out core.ImportFile
	z, err := zip.NewReader(r, size)
	if err != nil {
		return out, ErrNotAnExport
	}
	budget := int64(maxUnpacked)
	found := false
	for _, f := range z.File {
		name := path.Base(f.Name)
		if f.FileInfo().IsDir() || !strings.EqualFold(path.Ext(name), ".json") {
			continue
		}
		ok, err := readBackup(f, &budget, &out)
		if errors.Is(err, core.ErrTooManyRows) || errors.Is(err, core.ErrTooBig) {
			return out, err
		}
		if err != nil && strings.HasPrefix(strings.ToLower(name), "simkl") {
			return out, BadFileError{Name: name, Err: err}
		}
		if err != nil || !ok {
			continue
		}
		found = true
	}
	if !found {
		return out, ErrNotAnExport
	}
	return out, nil
}

func readBackup(f *zip.File, budget *int64, out *core.ImportFile) (bool, error) {
	rc, err := f.Open()
	if err != nil {
		return false, err
	}
	defer func() { _ = rc.Close() }()
	limited := &io.LimitedReader{R: rc, N: *budget + 1}
	defer func() { *budget = limited.N - 1 }()
	var file core.ImportFile
	ok, err := decodeBackup(json.NewDecoder(limited), &file, rows(out))
	if limited.N <= 0 {
		return false, core.ErrTooBig
	}
	if err != nil || !ok {
		return false, err
	}
	out.History = append(out.History, file.History...)
	out.Watchlist = append(out.Watchlist, file.Watchlist...)
	out.Ratings = append(out.Ratings, file.Ratings...)
	out.Hidden = append(out.Hidden, file.Hidden...)
	return true, nil
}

func rows(f *core.ImportFile) int {
	return len(f.History) + len(f.Watchlist) + len(f.Playback) + len(f.Ratings) + len(f.Hidden)
}

func decodeBackup(d *json.Decoder, out *core.ImportFile, before int) (bool, error) {
	if tok, err := d.Token(); err != nil || tok != json.Delim('{') {
		return false, cmp.Or(err, errors.New("expected an object"))
	}
	found := false
	for d.More() {
		tok, err := d.Token()
		if err != nil {
			return false, err
		}
		key, _ := tok.(string)
		if key != "shows" && key != "movies" && key != "anime" {
			var skip json.RawMessage
			if err := d.Decode(&skip); err != nil {
				return false, err
			}
			continue
		}
		found = true
		tok, err = d.Token()
		if err != nil {
			return false, err
		}
		if tok == nil {
			continue
		}
		if tok != json.Delim('[') {
			return false, fmt.Errorf("expected a list of %s", key)
		}
		for d.More() {
			var e entry
			if err := d.Decode(&e); err != nil {
				return false, err
			}
			e.add(out, key == "anime")
			if before+rows(out) > core.MaxImportRows {
				return false, core.ErrTooManyRows
			}
		}
		if _, err := d.Token(); err != nil {
			return false, err
		}
	}
	return found, nil
}

func (e entry) add(out *core.ImportFile, anime bool) {
	m, kind := e.Show, "show"
	if e.Movie != nil {
		m, kind = e.Movie, "movie"
	}
	if m == nil {
		return
	}
	title := core.Ref{Kind: kind, IDs: m.ids()}
	// Simkl splits anime into its own entries per season, numbered from 1, so episodes can't be placed on a TMDB show.
	show := title.IDs
	if anime {
		show = core.IDs{}
	}

	if kind == "movie" && e.Status == "completed" {
		out.History = append(out.History, core.ImportItem{Ref: title, Label: m.label(), At: e.LastWatchedAt, Action: "watch"})
	}
	for _, s := range e.Seasons {
		for _, ep := range s.Episodes {
			label := fmt.Sprintf("%s %dx%02d", m.label(), s.Number, ep.Number)
			if anime {
				label = fmt.Sprintf("%s E%d", m.label(), ep.Number)
			}
			ref := core.Ref{Kind: "episode", Show: show, Season: &s.Number, Number: &ep.Number}
			out.History = append(out.History, core.ImportItem{Ref: ref, Label: label, At: ep.WatchedAt, Action: "watch"})
		}
	}
	if e.Rewatch {
		return
	}

	switch {
	case e.Status == "plantowatch", e.Status == "watching" && e.WatchedCount == 0 && len(e.Seasons) == 0:
		out.Watchlist = append(out.Watchlist, core.ImportItem{Ref: title, Label: m.label(), At: e.AddedAt})
	case e.Status == "dropped":
		out.Hidden = append(out.Hidden, core.ImportItem{Ref: title, Label: m.label(), Section: "dropped"})
	case e.Status == "hold":
		out.Hidden = append(out.Hidden, core.ImportItem{Ref: title, Label: m.label(), Section: "progress_watched"})
	}
	if e.Rating > 0 {
		out.Ratings = append(out.Ratings, core.ImportItem{Ref: title, Label: m.label(), At: e.RatedAt, Rating: e.Rating})
	}
}
