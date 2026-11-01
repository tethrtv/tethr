package api

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/tethrtv/tethr/internal/core"
)

func (s *Server) caller(ctx context.Context) (core.Caller, bool, error) {
	if token, ok := ctx.Value(bearerKey).(string); ok {
		c, err := core.TokenCaller(ctx, s.DB, token)
		if errors.Is(err, core.ErrInvalidGrant) {
			return core.Caller{}, false, nil
		}
		return c, err == nil, err
	}
	accountID, _, ok, err := s.signedIn(ctx)
	if !ok {
		return core.Caller{}, false, err
	}
	c, err := core.AccountCaller(ctx, s.DB, accountID)
	return c, err == nil, err
}

var notConnected = problem("not_signed_in", "Sign in, or send an access token that hasn’t expired or been revoked.")

func (s *Server) GetProfile(ctx context.Context, _ GetProfileRequestObject) (GetProfileResponseObject, error) {
	c, ok, err := s.caller(ctx)
	if !ok {
		return GetProfile401JSONResponse{notConnected}, err
	}
	p, err := core.Profile(ctx, s.DB, c.ProfileID)
	if err != nil {
		return nil, err
	}
	return GetProfile200JSONResponse(toProfile(p)), nil
}

func (s *Server) Scrobble(ctx context.Context, req ScrobbleRequestObject) (ScrobbleResponseObject, error) {
	c, ok, err := s.caller(ctx)
	if !ok {
		return Scrobble401JSONResponse(notConnected), err
	}
	in := core.Scrobble{Ref: fromRef(req.Body.Item), Progress: req.Body.Progress, Duration: req.Body.DurationSeconds}
	var result core.ScrobbleResult
	switch req.Body.Event {
	case Start:
		result, err = core.ScrobbleStart(ctx, s.DB, c, in)
	case Pause:
		result, err = core.ScrobblePause(ctx, s.DB, c, in)
	case Stop:
		result, err = core.ScrobbleStop(ctx, s.DB, c, in)
	default:
		return Scrobble400JSONResponse{problem("invalid_event", "event is start, pause or stop.")}, nil
	}
	var already core.AlreadyScrobbled
	switch {
	case errors.As(err, &already):
		return Scrobble409JSONResponse(problem("already_watched", "That was logged as watched at "+already.WatchedAt.Format(time.RFC3339)+". Stops for the same item within 10 minutes are ignored.")), nil
	case errors.Is(err, core.ErrNotFound):
		return Scrobble404JSONResponse(noSuchItem), nil
	case errors.Is(err, core.ErrProgressTooLow):
		return Scrobble422JSONResponse(problem("progress_too_low", "Progress under 1% isn’t saved.")), nil
	case err != nil:
		return nil, err
	}
	out := ScrobbleResult{Result: Watching, Progress: result.Progress, Item: toItem(result.Item)}
	switch result.Action {
	case "pause":
		out.Result = Paused
	case "scrobble":
		out.Result = Watched
	}
	if result.ID != 0 {
		out.Id = &result.ID
	}
	return Scrobble200JSONResponse(out), nil
}

var noSuchItem = problem("not_found", "Send a film or show with at least one ID, or an episode by its show’s IDs, season and number.")

func (s *Server) ListHistory(ctx context.Context, req ListHistoryRequestObject) (ListHistoryResponseObject, error) {
	c, ok, err := s.caller(ctx)
	if !ok {
		return ListHistory401JSONResponse{notConnected}, err
	}
	page := core.Page{Page: max(value(req.Params.Page), 1), Limit: min(max(value(req.Params.Limit), 0), 1000)}
	if page.Limit == 0 {
		page.Limit = 50
	}
	filter := core.HistoryFilter{}
	if req.Params.Type != nil {
		filter.Kind = string(*req.Params.Type)
	}
	if req.Params.TitleId != nil {
		filter.ID = *req.Params.TitleId
	}
	entries, total, err := core.History(ctx, s.DB, c.ProfileID, filter, page)
	if err != nil {
		return nil, err
	}
	out := HistoryPage{Items: make([]Watch, 0, len(entries)), Total: total, Page: page.Page, Limit: page.Limit}
	for _, e := range entries {
		out.Items = append(out.Items, Watch{Id: e.ID, WatchedAt: e.At, Action: WatchAction(e.Action), Item: toItem(e.Item)})
	}
	return ListHistory200JSONResponse(out), nil
}

func (s *Server) AddHistory(ctx context.Context, req AddHistoryRequestObject) (AddHistoryResponseObject, error) {
	c, ok, err := s.caller(ctx)
	if !ok {
		return AddHistory401JSONResponse(notConnected), err
	}
	watches := make([]core.Watch, 0, len(req.Body.Items))
	for _, w := range req.Body.Items {
		at := time.Now()
		if w.WatchedAt != nil {
			at = *w.WatchedAt
		}
		watches = append(watches, core.Watch{Ref: fromRef(w.Item), At: at})
	}
	result, err := core.AddHistory(ctx, s.DB, c, watches)
	if err != nil {
		return nil, err
	}
	added := total(result.Added)
	return AddHistory200JSONResponse(SyncResult{Added: added, Existing: len(watches) - added - len(result.NotFound), NotFound: toRefs(result.NotFound)}), nil
}

func (s *Server) RemoveWatch(ctx context.Context, req RemoveWatchRequestObject) (RemoveWatchResponseObject, error) {
	c, ok, err := s.caller(ctx)
	if !ok {
		return RemoveWatch401JSONResponse{notConnected}, err
	}
	result, err := core.RemoveHistory(ctx, s.DB, c.ProfileID, []int64{req.Id}, nil)
	if err != nil {
		return nil, err
	}
	if len(result.NotIDs) > 0 {
		return RemoveWatch404JSONResponse(problem("not_found", "There’s no watch with that ID.")), nil
	}
	return RemoveWatch204Response{}, nil
}

func (s *Server) ListPlayback(ctx context.Context, req ListPlaybackRequestObject) (ListPlaybackResponseObject, error) {
	c, ok, err := s.caller(ctx)
	if !ok {
		return ListPlayback401JSONResponse{notConnected}, err
	}
	kind := ""
	if req.Params.Type != nil {
		kind = string(*req.Params.Type)
	}
	entries, err := core.Playback(ctx, s.DB, c.ProfileID, kind, core.Range{}, core.Page{Page: 1, Limit: 1000})
	if err != nil {
		return nil, err
	}
	out := make(ListPlayback200JSONResponse, 0, len(entries))
	for _, e := range entries {
		out = append(out, ResumePoint{Id: e.ID, Progress: e.Progress, PausedAt: e.At, Item: toItem(e.Item)})
	}
	return out, nil
}

func (s *Server) RemovePlayback(ctx context.Context, req RemovePlaybackRequestObject) (RemovePlaybackResponseObject, error) {
	c, ok, err := s.caller(ctx)
	if !ok {
		return RemovePlayback401JSONResponse{notConnected}, err
	}
	err = core.RemovePlayback(ctx, s.DB, c.ProfileID, req.Id)
	if errors.Is(err, core.ErrNotFound) {
		return RemovePlayback404JSONResponse(problem("not_found", "There’s no resume point with that ID.")), nil
	}
	if err != nil {
		return nil, err
	}
	return RemovePlayback204Response{}, nil
}

func (s *Server) ListWatchlist(ctx context.Context, req ListWatchlistRequestObject) (ListWatchlistResponseObject, error) {
	c, ok, err := s.caller(ctx)
	if !ok {
		return ListWatchlist401JSONResponse{notConnected}, err
	}
	kind := ""
	if req.Params.Type != nil {
		kind = string(*req.Params.Type)
	}
	entries, err := core.Watchlist(ctx, s.DB, c.ProfileID, kind)
	if err != nil {
		return nil, err
	}
	out := make(ListWatchlist200JSONResponse, 0, len(entries))
	for _, e := range entries {
		out = append(out, Listing{Id: e.ID, Rank: int(e.Rank), ListedAt: e.At, Notes: e.Notes, Item: toItem(e.Item)})
	}
	return out, nil
}

func (s *Server) AddToWatchlist(ctx context.Context, req AddToWatchlistRequestObject) (AddToWatchlistResponseObject, error) {
	c, ok, err := s.caller(ctx)
	if !ok {
		return AddToWatchlist401JSONResponse(notConnected), err
	}
	listings := make([]core.Listing, 0, len(req.Body.Items))
	for _, l := range req.Body.Items {
		listings = append(listings, core.Listing{Ref: fromRef(l.Item), Notes: strings.TrimSpace(value(l.Notes))})
	}
	result, err := core.AddToWatchlist(ctx, s.DB, c.ProfileID, listings)
	if err != nil {
		return nil, err
	}
	return AddToWatchlist200JSONResponse(SyncResult{Added: total(result.Added), Existing: total(result.Existing), NotFound: toRefs(result.NotFound)}), nil
}

func (s *Server) RemoveFromWatchlist(ctx context.Context, req RemoveFromWatchlistRequestObject) (RemoveFromWatchlistResponseObject, error) {
	c, ok, err := s.caller(ctx)
	if !ok {
		return RemoveFromWatchlist401JSONResponse{notConnected}, err
	}
	err = core.RemoveListing(ctx, s.DB, c.ProfileID, req.Id)
	if errors.Is(err, core.ErrNotFound) {
		return RemoveFromWatchlist404JSONResponse(problem("not_found", "That isn’t on the watchlist.")), nil
	}
	if err != nil {
		return nil, err
	}
	return RemoveFromWatchlist204Response{}, nil
}

func (s *Server) GetActivity(ctx context.Context, _ GetActivityRequestObject) (GetActivityResponseObject, error) {
	c, ok, err := s.caller(ctx)
	if !ok {
		return GetActivity401JSONResponse{notConnected}, err
	}
	a, err := core.LastActivities(ctx, s.DB, c.ProfileID)
	if err != nil {
		return nil, err
	}
	latest := func(areas ...string) time.Time {
		at := a.Since
		for _, area := range areas {
			if t := a.Marks[area]; t.After(at) {
				at = t
			}
		}
		return at
	}
	out := Activity{
		History:   latest("movies.watched_at", "episodes.watched_at"),
		Playback:  latest("movies.paused_at", "episodes.paused_at"),
		Watchlist: latest("watchlist.updated_at"),
	}
	out.Latest = latest("movies.watched_at", "episodes.watched_at", "movies.paused_at", "episodes.paused_at", "watchlist.updated_at")
	return GetActivity200JSONResponse(out), nil
}

func total(c core.Counts) int { return c.Movies + c.Shows + c.Seasons + c.Episodes }

func fromIDs(ids *Ids) core.IDs {
	if ids == nil {
		return core.IDs{}
	}
	out := core.IDs{IMDB: value(ids.Imdb)}
	if ids.Tethr != nil {
		out.Trakt = *ids.Tethr
	}
	if ids.Tmdb != nil {
		out.TMDB = int64(*ids.Tmdb)
	}
	if ids.Tvdb != nil {
		out.TVDB = int64(*ids.Tvdb)
	}
	return out
}

func fromRef(r ItemRef) core.Ref {
	ref := core.Ref{Kind: string(r.Type), IDs: fromIDs(r.Ids), Show: fromIDs(r.Show)}
	if r.Season != nil {
		season := int32(*r.Season)
		ref.Season = &season
	}
	if r.Number != nil {
		number := int32(*r.Number)
		ref.Number = &number
	}
	return ref
}

func toIDs(ids core.IDs) *Ids {
	out := &Ids{}
	if ids.Trakt != 0 {
		out.Tethr = &ids.Trakt
	}
	if ids.TMDB != 0 {
		tmdb := int(ids.TMDB)
		out.Tmdb = &tmdb
	}
	if ids.IMDB != "" {
		out.Imdb = &ids.IMDB
	}
	if ids.TVDB != 0 {
		tvdb := int(ids.TVDB)
		out.Tvdb = &tvdb
	}
	return out
}

func toRefs(refs []core.Ref) []ItemRef {
	out := make([]ItemRef, 0, len(refs))
	for _, r := range refs {
		ref := ItemRef{Type: ItemRefType(r.Kind), Ids: toIDs(r.IDs)}
		if r.Show != (core.IDs{}) {
			ref.Show = toIDs(r.Show)
		}
		if r.Season != nil {
			season := int(*r.Season)
			ref.Season = &season
		}
		if r.Number != nil {
			number := int(*r.Number)
			ref.Number = &number
		}
		out = append(out, ref)
	}
	return out
}

func toItem(i core.Item) Item {
	t := i.Title
	title := Title{Ids: Ids{Tethr: &t.ID}, Name: t.Name}
	if t.Year != nil {
		year := int(*t.Year)
		title.Year = &year
	}
	if t.TmdbID != nil {
		tmdb := int(*t.TmdbID)
		title.Ids.Tmdb = &tmdb
	}
	if t.Imdb != "" {
		title.Ids.Imdb = &t.Imdb
	}
	if tvdb, err := strconv.Atoi(t.Tvdb); err == nil {
		title.Ids.Tvdb = &tvdb
	}
	out := Item{Type: ItemType(i.Kind()), Title: title}
	if i.Season != nil {
		season := int(*i.Season)
		out.Season = &season
	}
	if e := i.Episode; e != nil {
		episode := &Episode{Season: int(e.SeasonNumber), Number: int(e.EpisodeNumber), Ids: Ids{Tethr: &e.ID}}
		if e.TmdbEpisodeID != nil {
			tmdb := int(*e.TmdbEpisodeID)
			episode.Ids.Tmdb = &tmdb
		}
		out.Episode = episode
	}
	return out
}
