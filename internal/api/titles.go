package api

import (
	"context"
	"errors"

	"github.com/tethrtv/tethr/internal/core"
)

func (s *Server) GetTitle(ctx context.Context, req GetTitleRequestObject) (GetTitleResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return GetTitle404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return GetTitle401JSONResponse{notConnected}, err
	}
	kind := ""
	if req.Params.Type != nil {
		kind = string(*req.Params.Type)
	}
	t, err := core.Title(ctx, s.DB, c.ProfileID, req.Id, kind)
	if errors.Is(err, core.ErrNotFound) {
		return GetTitle404JSONResponse(problem("not_found", "There’s no film or show with that ID or slug.")), nil
	}
	if err != nil {
		return nil, err
	}
	out := TitleDetail{Type: TitleDetailType(t.Item.Kind()), Title: toItem(ctx, t.Item).Title, Watches: toPlays(t.Watches), Resume: toResume(t.Resume), Facts: toFacts(t.Facts)}
	if r := t.Rating; r != nil {
		out.Rating = &TitleRating{Id: r.ID, Rating: int(r.Rating), RatedAt: r.At}
	}
	if l := t.Listing; l != nil {
		out.Listing = &TitleListing{Id: l.ID, ListedAt: l.At}
	}
	if len(t.Lists) > 0 {
		inLists := make([]TitleInList, 0, len(t.Lists))
		for _, l := range t.Lists {
			inLists = append(inLists, TitleInList{ListId: l.ListID, ItemId: l.ItemID})
		}
		out.Lists = &inLists
	}
	if t.Item.Kind() == "show" {
		if t.Order != "" {
			out.EpisodeOrder = &t.Order
		}
		seasons := make([]SeasonDetail, 0, len(t.Seasons))
		for _, season := range t.Seasons {
			episodes := make([]EpisodeDetail, 0, len(season.Episodes))
			for _, e := range season.Episodes {
				episodes = append(episodes, EpisodeDetail{Episode: toEpisode(ctx, e.Episode), Watches: toPlays(e.Watches), Resume: toResume(e.Resume)})
			}
			detail := SeasonDetail{Number: int(season.Number), Episodes: episodes}
			if season.ID != 0 {
				detail.Ids = &Ids{Tethr: &season.ID}
			}
			seasons = append(seasons, detail)
		}
		out.Seasons = &seasons
		out.RewatchFrom = t.RewatchFrom
	}
	return GetTitle200JSONResponse(out), nil
}

func (s *Server) PickEpisodeOrder(ctx context.Context, req PickEpisodeOrderRequestObject) (PickEpisodeOrderResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return PickEpisodeOrder404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return PickEpisodeOrder401JSONResponse{notConnected}, err
	}
	if s.FetchOrder == nil {
		return PickEpisodeOrder503JSONResponse(problem("no_tmdb", "This server can’t reach TMDB, so shows stay in aired order.")), nil
	}
	order, err := s.FetchOrder(ctx, req.Body.TmdbGroup)
	if err == nil {
		err = core.PickEpisodeOrder(ctx, s.DB, c.ProfileID, req.Id, &order)
	}
	switch {
	case errors.Is(err, core.ErrNotFound):
		return PickEpisodeOrder404JSONResponse(noSuchShow), nil
	case errors.Is(err, core.ErrWrongShow):
		return PickEpisodeOrder422JSONResponse(problem("wrong_episode_group", "That isn’t one of this show’s episode groups on TMDB.")), nil
	case err != nil:
		return nil, err
	}
	return PickEpisodeOrder204Response{}, nil
}

func (s *Server) UseAiredOrder(ctx context.Context, req UseAiredOrderRequestObject) (UseAiredOrderResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return UseAiredOrder404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return UseAiredOrder401JSONResponse{notConnected}, err
	}
	err = core.PickEpisodeOrder(ctx, s.DB, c.ProfileID, req.Id, nil)
	if errors.Is(err, core.ErrNotFound) {
		return UseAiredOrder404JSONResponse(noSuchShow), nil
	}
	if err != nil {
		return nil, err
	}
	return UseAiredOrder204Response{}, nil
}

var noSuchShow = problem("not_found", "There’s no show with that ID or slug.")

func toEpisode(ctx context.Context, e core.OrderedEpisode) Episode {
	return *toItem(ctx, core.Item{Episode: &e}).Episode
}

func (s *Server) Search(ctx context.Context, req SearchRequestObject) (SearchResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return Search404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return Search401JSONResponse(notConnected), err
	}
	kind := ""
	if req.Params.Type != nil {
		kind = string(*req.Params.Type)
	}
	scope := Library
	if req.Params.Scope != nil {
		scope = *req.Params.Scope
	}
	if !scope.Valid() {
		return Search400JSONResponse{problem("invalid_scope", "Scope is library or catalogue.")}, nil
	}
	items, err := core.Search(ctx, s.DB, c.ProfileID, req.Params.Q, kind, string(scope))
	if err != nil {
		return nil, err
	}
	out := make(Search200JSONResponse, 0, len(items))
	for _, i := range items {
		out = append(out, toItem(ctx, i))
	}
	return out, nil
}

func toPlays(entries []core.Entry) []Play {
	out := make([]Play, 0, len(entries))
	for _, e := range entries {
		out = append(out, Play{Id: e.ID, WatchedAt: e.At, Precision: PlayPrecision(e.Precision), Action: PlayAction(e.Action)})
	}
	return out
}

func toResume(e *core.Entry) *Resume {
	if e == nil {
		return nil
	}
	return &Resume{Id: e.ID, Progress: e.Progress}
}

func (s *Server) ReportTitle(ctx context.Context, req ReportTitleRequestObject) (ReportTitleResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return ReportTitle404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return ReportTitle401JSONResponse(notConnected), err
	}
	r := core.TitleReport{Reason: string(req.Body.Reason), Suggestion: value(req.Body.Suggestion), Note: value(req.Body.Note)}
	if req.Body.Provider != nil {
		r.Provider = string(*req.Body.Provider)
	}
	_, err = core.ReportTitle(ctx, s.DB, c.ProfileID, req.Id, r)
	bad := func(code, detail string) (ReportTitleResponseObject, error) {
		return ReportTitle400JSONResponse{problem(code, detail)}, nil
	}
	switch {
	case errors.Is(err, core.ErrNotFound):
		return ReportTitle404JSONResponse(problem("not_found", "That film or show isn’t on Tethr any more.")), nil
	case errors.Is(err, core.ErrReportProfile):
		return ReportTitle403JSONResponse(problem("adults_only", "Kids and guest profiles can’t send reports.")), nil
	case errors.Is(err, core.ErrTitleReportReason):
		return bad("invalid_reason", "Pick what’s wrong.")
	case errors.Is(err, core.ErrReportSuggestion):
		return bad("invalid_suggestion", "That doesn’t look right for what you picked. Leave it empty if you’re not sure.")
	case errors.Is(err, core.ErrReportSame):
		return bad("same_as_now", "That’s what Tethr already has.")
	case errors.Is(err, core.ErrReportProvider):
		return bad("invalid_provider", "Pick a site that lists this film or show.")
	case errors.Is(err, core.ErrReportDuplicate):
		return bad("unknown_duplicate", "Tethr can’t find another title there. Paste the other one’s Wikidata link.")
	case errors.Is(err, core.ErrReportSelf):
		return bad("same_title", "That’s this title. Paste a link to the other one.")
	case errors.Is(err, core.ErrReportNote):
		return bad("invalid_note", "Notes are up to 500 characters.")
	case errors.Is(err, core.ErrTooManyTitleReports):
		return ReportTitle429JSONResponse(problem("too_many_reports", "That’s a lot of reports for one day. Try again tomorrow, or email reports@tethr.tv.")), nil
	case err != nil:
		return nil, err
	}
	return ReportTitle204Response{}, nil
}

func (s *Server) MatchFilms(ctx context.Context, req MatchFilmsRequestObject) (MatchFilmsResponseObject, error) {
	if _, ok, err := s.caller(ctx); !ok {
		return MatchFilms401JSONResponse(notConnected), err
	}
	if n := len(req.Body.Films); n == 0 || n > 100 {
		return MatchFilms400JSONResponse{problem("invalid_films", "Send between 1 and 100 films.")}, nil
	}
	films := make([]core.FilmName, len(req.Body.Films))
	for i, f := range req.Body.Films {
		films[i] = core.FilmName{Name: f.Name, Year: value(f.Year)}
	}
	found, err := core.MatchFilms(ctx, s.DB, films)
	if err != nil {
		return nil, err
	}
	out := MatchFilms200JSONResponse{Results: make([]FilmMatch, len(found))}
	for i, f := range found {
		out.Results[i].Candidates = make([]Item, len(f.Candidates))
		for j, c := range f.Candidates {
			out.Results[i].Candidates[j] = toItem(ctx, c)
		}
		if f.Match != nil {
			m := toItem(ctx, *f.Match)
			out.Results[i].Match = &m
		}
	}
	return out, nil
}

func toFacts(f core.TitleFacts) *TitleFacts {
	out := TitleFacts{Countries: some(f.Countries), Languages: some(f.Languages), Started: f.Started, Ended: f.Ended}
	if len(f.Genres) > 0 {
		genres := make([]TitleFactsGenres, 0, len(f.Genres))
		for _, g := range f.Genres {
			genres = append(genres, TitleFactsGenres(g))
		}
		out.Genres = &genres
	}
	if len(f.TypeFlags) > 0 {
		flags := make([]TitleFactsTypeFlags, 0, len(f.TypeFlags))
		for _, t := range f.TypeFlags {
			flags = append(flags, TitleFactsTypeFlags(t))
		}
		out.TypeFlags = &flags
	}
	out.Studios = toCompanies(f.Studios)
	out.Networks = toCompanies(f.Networks)
	if out.Genres == nil && out.Countries == nil && out.Languages == nil && out.TypeFlags == nil && out.Started == nil && out.Ended == nil &&
		out.Studios == nil && out.Networks == nil {
		return nil
	}
	return &out
}

func some[T any](s []T) *[]T {
	if len(s) == 0 {
		return nil
	}
	return &s
}

func toCompanies(cs []core.Company) *[]Company {
	if len(cs) == 0 {
		return nil
	}
	out := make([]Company, 0, len(cs))
	for _, c := range cs {
		out = append(out, Company{Wikidata: c.QID, Name: c.Name})
	}
	return &out
}
