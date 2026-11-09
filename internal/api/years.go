package api

import (
	"context"
	"errors"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/tethrtv/tethr/internal/core"
)

func (s *Server) GetYear(ctx context.Context, req GetYearRequestObject) (GetYearResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return GetYear404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return GetYear401JSONResponse(notConnected), err
	}
	loc := time.UTC
	if tz := req.Params.Tz; tz != nil && *tz != "" {
		named, known := zone(*tz)
		if !known {
			return GetYear400JSONResponse{problem("invalid_time_zone", "Give a time zone by name, such as Europe/London.")}, nil
		}
		loc = named
	}
	y, err := core.Year(ctx, s.DB, c.ProfileID, req.Year, loc, time.Now())
	if errors.Is(err, core.ErrNotFound) {
		return GetYear404JSONResponse(problem("not_found", "That year hasn’t started yet.")), nil
	}
	if err != nil {
		return nil, err
	}
	out := GetYear200JSONResponse{
		Year:      y.Year,
		Through:   openapi_types.Date{Time: y.Through},
		Plays:     y.Plays,
		Episodes:  y.Episodes,
		Films:     y.Films,
		Minutes:   y.Minutes,
		NewShows:  y.NewShows,
		Rewatched: y.Rewatched,
		Days:      make([]YearDay, 0, len(y.Days)),
		Hours:     y.Hours[:],
		Shows:     make([]YearShow, 0, len(y.Shows)),
		FilmPlays: make([]YearFilm, 0, len(y.FilmsSeen)),
		Run:       YearRun{Days: y.Run.Days},
		Years:     y.Years,
	}
	if out.Years == nil {
		out.Years = []int{}
	}
	for _, d := range y.Days {
		out.Days = append(out.Days, YearDay{Plays: d.Plays, Minutes: d.Minutes})
	}
	for _, sh := range y.Shows {
		out.Shows = append(out.Shows, YearShow{Show: toItem(sh.Show).Title, Episodes: sh.Episodes, Minutes: sh.Minutes, New: sh.New})
	}
	for _, f := range y.FilmsSeen {
		out.FilmPlays = append(out.FilmPlays, YearFilm{Film: toItem(f.Film).Title, WatchedAt: f.At, Precision: YearFilmPrecision(f.Precision), New: f.New, Again: f.Again})
	}
	if y.Run.Days > 0 {
		out.Run.From = &openapi_types.Date{Time: y.Run.From}
	}
	if b := y.Busiest; b != nil {
		out.Busiest = &BusiestDay{Date: openapi_types.Date{Time: b.Date}, Plays: b.Plays, Minutes: b.Minutes, Item: toItem(b.Title), ItemPlays: b.TitlePlays}
	}
	for _, e := range []struct {
		from *core.Entry
		to   **Watch
	}{{y.First, &out.First}, {y.Latest, &out.Latest}} {
		if e.from != nil {
			*e.to = &Watch{Id: e.from.ID, WatchedAt: e.from.At, Precision: WatchPrecision(e.from.Precision), Action: WatchAction(e.from.Action), Item: toItem(e.from.Item)}
		}
	}
	return out, nil
}

func zone(name string) (*time.Location, bool) {
	loc, err := time.LoadLocation(name)
	return loc, err == nil && name != "Local"
}
