package simklexport

import (
	"archive/zip"
	"bytes"
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

const maxUnpacked = 512 << 20

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

type library struct {
	Shows  []entry `json:"shows"`
	Movies []entry `json:"movies"`
	Anime  []entry `json:"anime"`
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
		b, ok, err := readBackup(f, &budget)
		if err != nil && strings.HasPrefix(strings.ToLower(name), "simkl") {
			return out, BadFileError{Name: name, Err: err}
		}
		if err != nil || !ok {
			continue
		}
		found = true
		for _, e := range b.Shows {
			e.add(&out, false)
		}
		for _, e := range b.Movies {
			e.add(&out, false)
		}
		for _, e := range b.Anime {
			e.add(&out, true)
		}
	}
	if !found {
		return out, ErrNotAnExport
	}
	return out, nil
}

func readBackup(f *zip.File, budget *int64) (library, bool, error) {
	var b library
	rc, err := f.Open()
	if err != nil {
		return b, false, err
	}
	defer func() { _ = rc.Close() }()
	limited := &io.LimitedReader{R: rc, N: *budget + 1}
	body, err := io.ReadAll(limited)
	*budget = limited.N - 1
	if *budget < 0 {
		return b, false, errors.New("the backup is too big")
	}
	if err != nil {
		return b, false, err
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(body, &keys); err != nil {
		return b, false, err
	}
	_, shows := keys["shows"]
	_, movies := keys["movies"]
	_, anime := keys["anime"]
	if !shows && !movies && !anime {
		return b, false, nil
	}
	return b, true, json.NewDecoder(bytes.NewReader(body)).Decode(&b)
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
