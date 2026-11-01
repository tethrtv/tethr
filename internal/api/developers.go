package api

import (
	"context"
	"errors"

	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/store"
)

var noSuchApp = problem("not_found", "You don’t have an app with that ID.")

func (s *Server) ListApps(ctx context.Context, _ ListAppsRequestObject) (ListAppsResponseObject, error) {
	accountID, _, ok, err := s.signedIn(ctx)
	if !ok {
		return ListApps401JSONResponse{notSignedIn}, err
	}
	apps, err := core.ListApps(ctx, s.DB, accountID)
	if err != nil {
		return nil, err
	}
	out := make(ListApps200JSONResponse, 0, len(apps))
	for _, a := range apps {
		out = append(out, DeveloperApp{
			Id:           a.ID,
			Name:         a.Name,
			ClientId:     a.ClientID,
			RedirectUris: a.RedirectUris,
			Connections:  a.Connections,
			CreatedAt:    a.CreatedAt,
		})
	}
	return out, nil
}

func (s *Server) RegisterApp(ctx context.Context, req RegisterAppRequestObject) (RegisterAppResponseObject, error) {
	accountID, _, ok, err := s.signedIn(ctx)
	if !ok {
		return RegisterApp401JSONResponse(notSignedIn), err
	}
	app, row, err := core.RegisterApp(ctx, s.DB, accountID, req.Body.Name, value(req.Body.RedirectUris))
	if errors.Is(err, core.ErrTooManyApps) {
		return RegisterApp409JSONResponse(problem("too_many_apps", "You can register up to 10 apps. Revoke one you don’t use first.")), nil
	}
	if p, ok := appProblem(err); ok {
		return RegisterApp400JSONResponse{p}, nil
	}
	if err != nil {
		return nil, err
	}
	return RegisterApp201JSONResponse{App: toDeveloperApp(row, 0), ClientSecret: app.ClientSecret}, nil
}

func (s *Server) UpdateApp(ctx context.Context, req UpdateAppRequestObject) (UpdateAppResponseObject, error) {
	accountID, _, ok, err := s.signedIn(ctx)
	if !ok {
		return UpdateApp401JSONResponse(notSignedIn), err
	}
	row, err := core.UpdateApp(ctx, s.DB, accountID, req.Id, core.AppChanges{Name: req.Body.Name, RedirectURIs: req.Body.RedirectUris})
	if errors.Is(err, core.ErrNotFound) {
		return UpdateApp404JSONResponse(noSuchApp), nil
	}
	if p, ok := appProblem(err); ok {
		return UpdateApp400JSONResponse{p}, nil
	}
	if err != nil {
		return nil, err
	}
	apps, err := core.ListApps(ctx, s.DB, accountID)
	if err != nil {
		return nil, err
	}
	var connections int64
	for _, a := range apps {
		if a.ID == row.ID {
			connections = a.Connections
		}
	}
	return UpdateApp200JSONResponse(toDeveloperApp(row, connections)), nil
}

func (s *Server) RevokeApp(ctx context.Context, req RevokeAppRequestObject) (RevokeAppResponseObject, error) {
	accountID, _, ok, err := s.signedIn(ctx)
	if !ok {
		return RevokeApp401JSONResponse{notSignedIn}, err
	}
	err = core.RevokeApp(ctx, s.DB, accountID, req.Id)
	if errors.Is(err, core.ErrNotFound) {
		return RevokeApp404JSONResponse(noSuchApp), nil
	}
	if err != nil {
		return nil, err
	}
	return RevokeApp204Response{}, nil
}

func (s *Server) NewAppSecret(ctx context.Context, req NewAppSecretRequestObject) (NewAppSecretResponseObject, error) {
	accountID, _, ok, err := s.signedIn(ctx)
	if !ok {
		return NewAppSecret401JSONResponse{notSignedIn}, err
	}
	secret, err := core.NewAppSecret(ctx, s.DB, accountID, req.Id)
	if errors.Is(err, core.ErrNotFound) {
		return NewAppSecret404JSONResponse(noSuchApp), nil
	}
	if err != nil {
		return nil, err
	}
	return NewAppSecret200JSONResponse{ClientSecret: secret}, nil
}

func appProblem(err error) (ProblemJSONResponse, bool) {
	switch {
	case errors.Is(err, core.ErrAppName):
		return problem("invalid_name", "Give the app a name of up to 60 characters."), true
	case errors.Is(err, core.ErrRedirectURI):
		return problem("invalid_redirect", "Sign-in can return to an https address, a custom scheme like myplayer://callback, or http on localhost. Up to 10 of them."), true
	}
	return ProblemJSONResponse{}, false
}

func toDeveloperApp(a store.App, connections int64) DeveloperApp {
	return DeveloperApp{
		Id:           a.ID,
		Name:         a.Name,
		ClientId:     a.ClientID,
		RedirectUris: a.RedirectUris,
		Connections:  connections,
		CreatedAt:    a.CreatedAt,
	}
}
