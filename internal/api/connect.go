package api

import (
	"context"
	"errors"
	"net/url"

	"github.com/tethrtv/tethr/internal/core"
)

var (
	noSuchDevice  = problem("unknown_code", "That code doesn’t match a device that’s waiting. Check the TV and try again; codes last 10 minutes.")
	noSuchProfile = problem("not_found", "There’s no profile with that ID in your household.")
)

func (s *Server) GetDevice(ctx context.Context, req GetDeviceRequestObject) (GetDeviceResponseObject, error) {
	if _, _, ok, err := s.signedIn(ctx); !ok {
		return GetDevice401JSONResponse{notSignedIn}, err
	}
	device, err := core.FindDevice(ctx, s.DB, req.Code)
	if errors.Is(err, core.ErrUnknownDevice) {
		return GetDevice404JSONResponse(noSuchDevice), nil
	}
	if err != nil {
		return nil, err
	}
	return GetDevice200JSONResponse{Code: device.UserCode, AppName: device.AppName}, nil
}

func (s *Server) ApproveDevice(ctx context.Context, req ApproveDeviceRequestObject) (ApproveDeviceResponseObject, error) {
	accountID, _, ok, err := s.signedIn(ctx)
	if !ok {
		return ApproveDevice401JSONResponse{notSignedIn}, err
	}
	err = core.ApproveDevice(ctx, s.DB, accountID, req.Code, req.Body.ProfileId)
	switch {
	case errors.Is(err, core.ErrUnknownDevice):
		return ApproveDevice404JSONResponse(noSuchDevice), nil
	case errors.Is(err, core.ErrNotFound):
		return ApproveDevice404JSONResponse(noSuchProfile), nil
	case err != nil:
		return nil, err
	}
	return ApproveDevice204Response{}, nil
}

func (s *Server) DenyDevice(ctx context.Context, req DenyDeviceRequestObject) (DenyDeviceResponseObject, error) {
	if _, _, ok, err := s.signedIn(ctx); !ok {
		return DenyDevice401JSONResponse{notSignedIn}, err
	}
	err := core.DenyDevice(ctx, s.DB, req.Code)
	if errors.Is(err, core.ErrUnknownDevice) {
		return DenyDevice404JSONResponse(noSuchDevice), nil
	}
	if err != nil {
		return nil, err
	}
	return DenyDevice204Response{}, nil
}

func (s *Server) GetAuthorization(ctx context.Context, req GetAuthorizationRequestObject) (GetAuthorizationResponseObject, error) {
	if _, _, ok, err := s.signedIn(ctx); !ok {
		return GetAuthorization401JSONResponse(notSignedIn), err
	}
	name, err := core.AuthorizingApp(ctx, s.DB, req.Params.ClientId, req.Params.RedirectUri)
	if p, ok := authorizeProblem(err); ok {
		return GetAuthorization400JSONResponse{p}, nil
	}
	if err != nil {
		return nil, err
	}
	return GetAuthorization200JSONResponse{AppName: name}, nil
}

func (s *Server) Authorize(ctx context.Context, req AuthorizeRequestObject) (AuthorizeResponseObject, error) {
	accountID, _, ok, err := s.signedIn(ctx)
	if !ok {
		return Authorize401JSONResponse(notSignedIn), err
	}
	in := core.Authorization{
		ClientID:            req.Body.ClientId,
		RedirectURI:         req.Body.RedirectUri,
		ProfileID:           req.Body.ProfileId,
		CodeChallenge:       value(req.Body.CodeChallenge),
		CodeChallengeMethod: value(req.Body.CodeChallengeMethod),
	}
	code, err := core.Authorize(ctx, s.DB, accountID, in)
	if errors.Is(err, core.ErrNotFound) {
		return Authorize404JSONResponse(noSuchProfile), nil
	}
	if p, ok := authorizeProblem(err); ok {
		return Authorize400JSONResponse{p}, nil
	}
	if err != nil {
		return nil, err
	}
	to, err := url.Parse(in.RedirectURI)
	if err != nil {
		return nil, err
	}
	query := to.Query()
	query.Set("code", code)
	if state := value(req.Body.State); state != "" {
		query.Set("state", state)
	}
	to.RawQuery = query.Encode()
	return Authorize200JSONResponse{RedirectTo: to.String()}, nil
}

func authorizeProblem(err error) (ProblemJSONResponse, bool) {
	switch {
	case errors.Is(err, core.ErrUnknownApp):
		return problem("unknown_app", "Tethr doesn’t know the app that sent you here."), true
	case errors.Is(err, core.ErrBadRedirect):
		return problem("bad_redirect", "The app asked to send you somewhere it hasn’t registered, so Tethr stopped."), true
	case errors.Is(err, core.ErrBadCodeChallenge):
		return problem("bad_code_challenge", "The app sent a code challenge Tethr doesn’t support."), true
	}
	return ProblemJSONResponse{}, false
}

func (s *Server) ListConnections(ctx context.Context, _ ListConnectionsRequestObject) (ListConnectionsResponseObject, error) {
	accountID, _, ok, err := s.signedIn(ctx)
	if !ok {
		return ListConnections401JSONResponse{notSignedIn}, err
	}
	grants, err := core.ListGrants(ctx, s.DB, accountID)
	if err != nil {
		return nil, err
	}
	out := make(ListConnections200JSONResponse, 0, len(grants))
	for _, g := range grants {
		out = append(out, Connection{
			Id:          g.ID,
			AppName:     g.AppName,
			ProfileId:   g.ProfileID,
			ProfileName: g.ProfileName,
			Label:       g.Label,
			UserAgent:   g.UserAgent,
			CreatedAt:   g.CreatedAt,
			LastUsedAt:  g.LastUsedAt,
		})
	}
	return out, nil
}

func (s *Server) RenameConnection(ctx context.Context, req RenameConnectionRequestObject) (RenameConnectionResponseObject, error) {
	accountID, _, ok, err := s.signedIn(ctx)
	if !ok {
		return RenameConnection401JSONResponse{notSignedIn}, err
	}
	err = core.RenameGrant(ctx, s.DB, accountID, req.Id, req.Body.Label)
	if errors.Is(err, core.ErrNotFound) {
		return RenameConnection404JSONResponse(problem("not_found", "That app isn’t connected any more.")), nil
	}
	if err != nil {
		return nil, err
	}
	return RenameConnection204Response{}, nil
}

func (s *Server) Disconnect(ctx context.Context, req DisconnectRequestObject) (DisconnectResponseObject, error) {
	accountID, _, ok, err := s.signedIn(ctx)
	if !ok {
		return Disconnect401JSONResponse{notSignedIn}, err
	}
	err = core.RevokeGrant(ctx, s.DB, accountID, req.Id)
	if errors.Is(err, core.ErrNotFound) {
		return Disconnect404JSONResponse(problem("not_found", "That app isn’t connected any more.")), nil
	}
	if err != nil {
		return nil, err
	}
	return Disconnect204Response{}, nil
}
