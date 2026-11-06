package api

import (
	"context"
	"errors"

	"github.com/tethrtv/tethr/internal/core"
)

var noSuchServer = problem("not_found", "There’s no media server with that ID.")

func (s *Server) ListMediaServers(ctx context.Context, _ ListMediaServersRequestObject) (ListMediaServersResponseObject, error) {
	accountID, _, ok, err := s.signedIn(ctx)
	if !ok {
		return ListMediaServers401JSONResponse{notSignedIn}, err
	}
	servers, err := core.MediaServers(ctx, s.DB, accountID)
	if err != nil {
		return nil, err
	}
	out := make(ListMediaServers200JSONResponse, 0, len(servers))
	for _, m := range servers {
		out = append(out, toMediaServer(m))
	}
	return out, nil
}

func (s *Server) AddMediaServer(ctx context.Context, req AddMediaServerRequestObject) (AddMediaServerResponseObject, error) {
	accountID, _, ok, err := s.signedIn(ctx)
	if !ok {
		return AddMediaServer401JSONResponse(notSignedIn), err
	}
	if !req.Body.Kind.Valid() {
		return AddMediaServer400JSONResponse{problem("unknown_server", "Tethr can take plays from Jellyfin, Plex and Emby.")}, nil
	}
	m, secret, err := core.AddMediaServer(ctx, s.DB, accountID, string(req.Body.Kind))
	if err != nil {
		return nil, err
	}
	return AddMediaServer201JSONResponse{Server: toMediaServer(m), Path: hookPath(m.Kind, secret)}, nil
}

func (s *Server) RemoveMediaServer(ctx context.Context, req RemoveMediaServerRequestObject) (RemoveMediaServerResponseObject, error) {
	accountID, _, ok, err := s.signedIn(ctx)
	if !ok {
		return RemoveMediaServer401JSONResponse{notSignedIn}, err
	}
	err = core.RemoveMediaServer(ctx, s.DB, accountID, req.Id)
	if errors.Is(err, core.ErrNotFound) {
		return RemoveMediaServer404JSONResponse(noSuchServer), nil
	}
	if err != nil {
		return nil, err
	}
	return RemoveMediaServer204Response{}, nil
}

func (s *Server) NewMediaServerAddress(ctx context.Context, req NewMediaServerAddressRequestObject) (NewMediaServerAddressResponseObject, error) {
	accountID, _, ok, err := s.signedIn(ctx)
	if !ok {
		return NewMediaServerAddress401JSONResponse{notSignedIn}, err
	}
	servers, err := core.MediaServers(ctx, s.DB, accountID)
	if err != nil {
		return nil, err
	}
	for _, m := range servers {
		if m.ID != req.Id {
			continue
		}
		secret, err := core.NewMediaServerSecret(ctx, s.DB, accountID, m.ID)
		if err != nil {
			return nil, err
		}
		return NewMediaServerAddress200JSONResponse{Path: hookPath(m.Kind, secret)}, nil
	}
	return NewMediaServerAddress404JSONResponse(noSuchServer), nil
}

func (s *Server) MatchServerUser(ctx context.Context, req MatchServerUserRequestObject) (MatchServerUserResponseObject, error) {
	accountID, _, ok, err := s.signedIn(ctx)
	if !ok {
		return MatchServerUser401JSONResponse{notSignedIn}, err
	}
	err = core.MatchServerUser(ctx, s.DB, accountID, req.Id, req.User, req.Body.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return MatchServerUser404JSONResponse(problem("not_found", "There’s no such user on that server, or no such profile in your household.")), nil
	}
	if err != nil {
		return nil, err
	}
	return MatchServerUser204Response{}, nil
}

func hookPath(kind, secret string) string {
	return "/hooks/" + kind + "/" + secret
}

func toMediaServer(m core.MediaServer) MediaServer {
	out := MediaServer{Id: m.ID, Kind: MediaServerKind(m.Kind), Name: m.Name, CreatedAt: m.CreatedAt, LastEventAt: m.LastEventAt, Users: make([]MediaServerUser, 0, len(m.Users))}
	for _, u := range m.Users {
		out.Users = append(out.Users, MediaServerUser{Id: u.ID, Name: u.Name, ProfileId: u.ProfileID, Ignored: u.Ignored, HeldPlays: int(u.Held), LastEventAt: u.LastEventAt})
	}
	return out
}
