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
	out := TitleDetail{Type: TitleDetailType(t.Item.Kind()), Title: toItem(t.Item).Title, Watches: toPlays(t.Watches), Resume: toResume(t.Resume)}
	if r := t.Rating; r != nil {
		out.Rating = &TitleRating{Id: r.ID, Rating: int(r.Rating), RatedAt: r.At}
	}
	if l := t.Listing; l != nil {
		out.Listing = &TitleListing{Id: l.ID, ListedAt: l.At}
	}
	if t.Item.Kind() == "show" {
		seasons := make([]SeasonDetail, 0, len(t.Seasons))
		for _, season := range t.Seasons {
			episodes := make([]EpisodeDetail, 0, len(season.Episodes))
			for _, e := range season.Episodes {
				episode := toItem(core.Item{Title: t.Item.Title, Episode: &e.Episode}).Episode
				episodes = append(episodes, EpisodeDetail{Episode: *episode, Watches: toPlays(e.Watches), Resume: toResume(e.Resume)})
			}
			seasons = append(seasons, SeasonDetail{Number: int(season.Number), Episodes: episodes})
		}
		out.Seasons = &seasons
	}
	return GetTitle200JSONResponse(out), nil
}

func (s *Server) Search(ctx context.Context, req SearchRequestObject) (SearchResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return Search404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return Search401JSONResponse{notConnected}, err
	}
	kind := ""
	if req.Params.Type != nil {
		kind = string(*req.Params.Type)
	}
	items, err := core.Search(ctx, s.DB, c.ProfileID, req.Params.Q, kind)
	if err != nil {
		return nil, err
	}
	out := make(Search200JSONResponse, 0, len(items))
	for _, i := range items {
		out = append(out, toItem(i))
	}
	return out, nil
}

func toPlays(entries []core.Entry) []Play {
	out := make([]Play, 0, len(entries))
	for _, e := range entries {
		out = append(out, Play{Id: e.ID, WatchedAt: e.At, Action: PlayAction(e.Action)})
	}
	return out
}

func toResume(e *core.Entry) *Resume {
	if e == nil {
		return nil
	}
	return &Resume{Id: e.ID, Progress: e.Progress}
}
