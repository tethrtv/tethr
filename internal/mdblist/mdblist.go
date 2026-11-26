package mdblist

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/tethrtv/tethr/internal/core"
)

var (
	ErrBadKey      = errors.New("mdblist didn't accept that api key")
	ErrLimited     = errors.New("mdblist's request limit was reached")
	ErrUnavailable = errors.New("mdblist couldn't be reached")
)

const (
	pageSize = 1000
	maxCalls = 300
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

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

type ids struct {
	IMDB  id `json:"imdb"`
	TMDB  id `json:"tmdb"`
	TVDB  id `json:"tvdb"`
	Trakt id `json:"trakt"`
}

func (i ids) core() core.IDs {
	return core.IDs{IMDB: string(i.IMDB), TMDB: i.TMDB.number(), TVDB: i.TVDB.number(), Trakt: i.Trakt.number()}
}

type media struct {
	Title string `json:"title"`
	Year  int    `json:"year"`
	IDs   ids    `json:"ids"`
}

func (m media) label() string {
	name := cmp.Or(m.Title, "Untitled")
	if m.Year > 0 {
		return fmt.Sprintf("%s (%d)", name, m.Year)
	}
	return name
}

type episode struct {
	Season int32  `json:"season"`
	Number int32  `json:"number"`
	IDs    ids    `json:"ids"`
	Show   *media `json:"show"`
}

type entry struct {
	WatchedAt *time.Time `json:"watched_at"`
	RatedAt   *time.Time `json:"rated_at"`
	PausedAt  *time.Time `json:"paused_at"`
	DroppedAt *time.Time `json:"dropped_at"`
	Rating    float64    `json:"rating"`
	Progress  float64    `json:"progress"`
	OnHold    bool       `json:"on_hold"`
	Movie     *media     `json:"movie"`
	Show      *media     `json:"show"`
	Season    *struct {
		Number int32 `json:"number"`
		IDs    ids   `json:"ids"`
	} `json:"season"`
	Episode *episode `json:"episode"`
}

func (e entry) ref() (core.Ref, string, bool) {
	switch {
	case e.Episode != nil:
		show := e.Show
		if show == nil {
			show = e.Episode.Show
		}
		if show == nil {
			return core.Ref{}, fmt.Sprintf("Unknown show %dx%02d", e.Episode.Season, e.Episode.Number), false
		}
		season, number := e.Episode.Season, e.Episode.Number
		return core.Ref{Kind: "episode", Show: show.IDs.core(), Season: &season, Number: &number},
			fmt.Sprintf("%s %dx%02d", show.label(), season, number), true
	case e.Season != nil && e.Show != nil:
		season := e.Season.Number
		return core.Ref{Kind: "season", Show: e.Show.IDs.core(), Season: &season},
			fmt.Sprintf("%s season %d", e.Show.label(), season), true
	case e.Movie != nil:
		return core.Ref{Kind: "movie", IDs: e.Movie.IDs.core()}, e.Movie.label(), true
	case e.Show != nil:
		return core.Ref{Kind: "show", IDs: e.Show.IDs.core()}, e.Show.label(), true
	}
	return core.Ref{}, "Untitled", false
}

type page struct {
	Movies     []entry    `json:"movies"`
	Shows      []entry    `json:"shows"`
	Seasons    []entry    `json:"seasons"`
	Episodes   []entry    `json:"episodes"`
	Pagination pagination `json:"pagination"`
}

type pagination struct {
	NextCursor string `json:"next_cursor"`
	HasMore    bool   `json:"has_more"`
}

type listItem struct {
	Title       string     `json:"title"`
	ReleaseYear int        `json:"release_year"`
	IDs         ids        `json:"ids"`
	IMDBID      id         `json:"imdb_id"`
	TVDBID      id         `json:"tvdb_id"`
	Rank        int        `json:"rank"`
	WatchlistAt *time.Time `json:"watchlist_at"`
}

func (l listItem) item(kind string) core.ImportItem {
	i := l.IDs.core()
	i.IMDB = cmp.Or(i.IMDB, string(l.IMDBID))
	i.TVDB = cmp.Or(i.TVDB, l.TVDBID.number())
	return core.ImportItem{Ref: core.Ref{Kind: kind, IDs: i}, Label: media{Title: l.Title, Year: l.ReleaseYear}.label(), At: l.WatchlistAt}
}

type listPage struct {
	Movies     []listItem `json:"movies"`
	Shows      []listItem `json:"shows"`
	Pagination pagination `json:"pagination"`
}

type list struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Dynamic     bool   `json:"dynamic"`
	Type        string `json:"type"`
	Private     *bool  `json:"private"`
}

func (l list) snapshot() bool {
	return l.Dynamic || l.Type != "" && l.Type != "static"
}

type fetch struct {
	*Client
	ctx   context.Context
	key   string
	calls int
	rows  int
}

func (c *Client) Fetch(ctx context.Context, key string) (core.ImportFile, error) {
	f := &fetch{Client: c, ctx: ctx, key: key}
	var out core.ImportFile

	err := f.pages("/sync/watched", url.Values{"plays": {"all"}}, func(p page) error {
		for _, e := range append(p.Movies, p.Episodes...) {
			ref, label, _ := e.ref()
			out.History = append(out.History, core.ImportItem{Ref: ref, Label: label, At: e.WatchedAt, Action: "watch"})
		}
		return f.count(len(p.Movies) + len(p.Episodes))
	})
	if err != nil {
		return out, err
	}

	err = f.pages("/sync/ratings", nil, func(p page) error {
		for _, e := range append(append(append(p.Movies, p.Shows...), p.Seasons...), p.Episodes...) {
			if rating := int16(e.Rating + 0.5); rating != 0 {
				ref, label, _ := e.ref()
				out.Ratings = append(out.Ratings, core.ImportItem{Ref: ref, Label: label, At: e.RatedAt, Rating: rating})
			}
		}
		return f.count(len(p.Movies) + len(p.Shows) + len(p.Seasons) + len(p.Episodes))
	})
	if err != nil {
		return out, err
	}

	err = f.listPages("/watchlist/items", func(p listPage) error {
		for _, l := range p.Movies {
			out.Watchlist = append(out.Watchlist, l.item("movie"))
		}
		for _, l := range p.Shows {
			out.Watchlist = append(out.Watchlist, l.item("show"))
		}
		return f.count(len(p.Movies) + len(p.Shows))
	})
	if err != nil {
		return out, err
	}

	var paused []entry
	if err := f.get("/sync/playback", nil, &paused); err != nil {
		return out, err
	}
	for _, e := range paused {
		if e.Progress > 0 {
			ref, label, _ := e.ref()
			out.Playback = append(out.Playback, core.ImportItem{Ref: ref, Label: label, At: e.PausedAt, Progress: e.Progress})
		}
	}
	if err := f.count(len(paused)); err != nil {
		return out, err
	}

	err = f.pages("/sync/dropped", nil, func(p page) error {
		for _, e := range p.Shows {
			ref, label, _ := e.ref()
			section := "dropped"
			if e.OnHold {
				section = "progress_watched"
			}
			out.Hidden = append(out.Hidden, core.ImportItem{Ref: ref, Label: label, At: e.DroppedAt, Section: section})
		}
		return f.count(len(p.Shows))
	})
	if err != nil {
		return out, err
	}

	var lists []list
	if err := f.get("/lists/user", nil, &lists); err != nil {
		return out, err
	}
	for _, l := range lists {
		il := core.ImportList{Name: l.Name, Description: l.Description, Snapshot: l.snapshot()}
		if l.Private != nil && !*l.Private {
			il.Privacy = "public"
		}
		type ranked struct {
			rank int
			item core.ImportItem
		}
		var items []ranked
		err := f.listPages(fmt.Sprintf("/lists/%d/items", l.ID), func(p listPage) error {
			for _, i := range p.Movies {
				items = append(items, ranked{i.Rank, i.item("movie")})
			}
			for _, i := range p.Shows {
				items = append(items, ranked{i.Rank, i.item("show")})
			}
			return f.count(len(p.Movies) + len(p.Shows))
		})
		if err != nil {
			return out, err
		}
		slices.SortStableFunc(items, func(a, b ranked) int { return cmp.Compare(a.rank, b.rank) })
		for _, i := range items {
			il.Items = append(il.Items, i.item)
		}
		out.Lists = append(out.Lists, il)
	}
	return out, nil
}

func (f *fetch) count(n int) error {
	f.rows += n
	if f.rows > core.MaxImportRows {
		return core.ErrTooManyRows
	}
	return nil
}

func (f *fetch) pages(path string, q url.Values, each func(page) error) error {
	return paginate(f, path, q, each, func(p page) pagination { return p.Pagination })
}

func (f *fetch) listPages(path string, each func(listPage) error) error {
	return paginate(f, path, url.Values{"unified": {"false"}}, each, func(p listPage) pagination { return p.Pagination })
}

func paginate[P any](f *fetch, path string, q url.Values, each func(P) error, next func(P) pagination) error {
	cursor := ""
	for {
		v := url.Values{"limit": {strconv.Itoa(pageSize)}}
		for k, vs := range q {
			v[k] = vs
		}
		if cursor != "" {
			v.Set("cursor", cursor)
		}
		var p P
		if err := f.get(path, v, &p); err != nil {
			return err
		}
		if err := each(p); err != nil {
			return err
		}
		n := next(p)
		if n.NextCursor == "" && n.HasMore {
			return fmt.Errorf("%w: %s: more to come but no cursor", ErrUnavailable, path)
		}
		if cursor = n.NextCursor; cursor == "" {
			return nil
		}
	}
}

func (f *fetch) get(path string, q url.Values, into any) error {
	if f.calls++; f.calls > maxCalls {
		return fmt.Errorf("%w: stopped after %d calls to mdblist", core.ErrTooManyRows, maxCalls)
	}
	if q == nil {
		q = url.Values{}
	}
	q.Set("apikey", f.key)
	base := cmp.Or(f.BaseURL, "https://api.mdblist.com")
	req, err := http.NewRequestWithContext(f.ctx, http.MethodGet, base+path+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	client := f.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		if errors.Is(f.ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("%w: %s after %d calls: %w", ErrUnavailable, path, f.calls, f.ctx.Err())
		}
		if f.ctx.Err() != nil {
			return f.ctx.Err()
		}
		return fmt.Errorf("%w: %s: %w", ErrUnavailable, path, redact(err))
	}
	defer func() { _ = res.Body.Close() }()
	switch {
	// Only the first call tells a wrong key apart: a later 403 is about that endpoint, not the key.
	case res.StatusCode == http.StatusUnauthorized, res.StatusCode == http.StatusForbidden && f.calls == 1:
		return ErrBadKey
	case res.StatusCode == http.StatusTooManyRequests:
		return ErrLimited
	case res.StatusCode != http.StatusOK:
		return fmt.Errorf("%w: %s: status %d", ErrUnavailable, path, res.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 64<<20)).Decode(into); err != nil {
		return fmt.Errorf("%w: %s: %w", ErrUnavailable, path, err)
	}
	return nil
}

// Errors from net/http quote the URL, and the key is in its query string.
func redact(err error) error {
	var u *url.Error
	if errors.As(err, &u) {
		return errors.New(u.Op + ": " + u.Err.Error())
	}
	return err
}
