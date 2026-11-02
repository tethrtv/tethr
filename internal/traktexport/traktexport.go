package traktexport

import (
	"archive/zip"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/tethrtv/tethr/internal/core"
)

var ErrNotAnExport = errors.New("that isn't a Trakt export zip")

type BadFileError struct {
	Name string
	Err  error
}

func (e BadFileError) Error() string { return e.Name + ": " + e.Err.Error() }
func (e BadFileError) Unwrap() error { return e.Err }

const maxUnpacked = 512 << 20

type ids struct {
	IMDB string `json:"imdb"`
	TMDB int64  `json:"tmdb"`
	TVDB int64  `json:"tvdb"`
}

func (i ids) core() core.IDs { return core.IDs{IMDB: i.IMDB, TMDB: i.TMDB, TVDB: i.TVDB} }

type media struct {
	Title string `json:"title"`
	Year  *int   `json:"year"`
	IDs   ids    `json:"ids"`
}

func (m media) label() string {
	name := cmp.Or(m.Title, "Untitled")
	if m.Year != nil {
		return fmt.Sprintf("%s (%d)", name, *m.Year)
	}
	return name
}

type entry struct {
	Type      string     `json:"type"`
	Action    string     `json:"action"`
	Rank      int        `json:"rank"`
	Notes     *string    `json:"notes"`
	Progress  float64    `json:"progress"`
	Rating    int16      `json:"rating"`
	WatchedAt *time.Time `json:"watched_at"`
	ListedAt  *time.Time `json:"listed_at"`
	PausedAt  *time.Time `json:"paused_at"`
	RatedAt   *time.Time `json:"rated_at"`
	HiddenAt  *time.Time `json:"hidden_at"`
	Movie     *media     `json:"movie"`
	Show      *media     `json:"show"`
	Season    *struct {
		Number int32 `json:"number"`
	} `json:"season"`
	Episode *struct {
		Season int32 `json:"season"`
		Number int32 `json:"number"`
		IDs    ids   `json:"ids"`
	} `json:"episode"`
}

func (e entry) item() core.ImportItem {
	var item core.ImportItem
	switch {
	case e.Movie != nil:
		item.Ref = core.Ref{Kind: "movie", IDs: e.Movie.IDs.core()}
		item.Label = e.Movie.label()
	case e.Show == nil:
		item.Ref = core.Ref{Kind: e.Type}
		item.Label = "Something with no title"
	case e.Episode != nil:
		item.Ref = core.Ref{Kind: "episode", IDs: core.IDs{TMDB: e.Episode.IDs.TMDB}, Show: e.Show.IDs.core(), Season: &e.Episode.Season, Number: &e.Episode.Number}
		item.Label = fmt.Sprintf("%s %dx%02d", e.Show.label(), e.Episode.Season, e.Episode.Number)
	case e.Season != nil:
		item.Ref = core.Ref{Kind: "season", Show: e.Show.IDs.core(), Season: &e.Season.Number}
		item.Label = fmt.Sprintf("%s season %d", e.Show.label(), e.Season.Number)
	default:
		item.Ref = core.Ref{Kind: "show", IDs: e.Show.IDs.core()}
		item.Label = e.Show.label()
	}
	return item
}

var files = regexp.MustCompile(`^(watched-history|lists-watchlist|watched-playback|ratings-(?:movies|shows|seasons|episodes)|hidden-[a-z-]+)(?:-\d+)?\.json$`)

func Read(r io.ReaderAt, size int64) (core.ImportFile, error) {
	var out core.ImportFile
	z, err := zip.NewReader(r, size)
	if err != nil {
		return out, ErrNotAnExport
	}
	budget := int64(maxUnpacked)
	found := false
	for _, f := range z.File {
		m := files.FindStringSubmatch(path.Base(f.Name))
		if m == nil || f.FileInfo().IsDir() {
			continue
		}
		found = true
		entries, err := readEntries(f, &budget)
		if err != nil {
			return out, BadFileError{Name: path.Base(f.Name), Err: err}
		}
		kind := m[1]
		if kind == "lists-watchlist" {
			slices.SortStableFunc(entries, func(a, b entry) int { return cmp.Compare(a.Rank, b.Rank) })
		}
		for _, e := range entries {
			item := e.item()
			switch {
			case kind == "watched-history":
				item.At, item.Action = e.WatchedAt, e.Action
				out.History = append(out.History, item)
			case kind == "lists-watchlist":
				item.At, item.Notes = e.ListedAt, strings.TrimSpace(deref(e.Notes))
				out.Watchlist = append(out.Watchlist, item)
			case kind == "watched-playback":
				item.At, item.Progress = e.PausedAt, e.Progress
				out.Playback = append(out.Playback, item)
			case strings.HasPrefix(kind, "ratings-"):
				item.At, item.Rating = e.RatedAt, e.Rating
				out.Ratings = append(out.Ratings, item)
			default:
				item.At = e.HiddenAt
				item.Section = strings.ReplaceAll(strings.TrimPrefix(kind, "hidden-"), "-", "_")
				out.Hidden = append(out.Hidden, item)
			}
		}
	}
	if !found {
		return out, ErrNotAnExport
	}
	return out, nil
}

func readEntries(f *zip.File, budget *int64) ([]entry, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	limited := &io.LimitedReader{R: rc, N: *budget + 1}
	var entries []entry
	err = json.NewDecoder(limited).Decode(&entries)
	*budget = limited.N - 1
	if *budget < 0 {
		return nil, errors.New("the export is too big")
	}
	return entries, err
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
