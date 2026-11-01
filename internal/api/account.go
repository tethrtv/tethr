package api

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/store"
)

var notSignedIn = problem("not_signed_in", "You’re not signed in.")

func (s *Server) signedIn(ctx context.Context) (int64, string, bool, error) {
	token, _ := ctx.Value(sessionKey).(string)
	accountID, err := core.SessionAccount(ctx, s.DB, token)
	if errors.Is(err, core.ErrNoSession) {
		return 0, "", false, nil
	}
	return accountID, token, err == nil, err
}

func (s *Server) AddProfile(ctx context.Context, req AddProfileRequestObject) (AddProfileResponseObject, error) {
	accountID, _, ok, err := s.signedIn(ctx)
	if !ok {
		return AddProfile401JSONResponse(notSignedIn), err
	}
	in := core.NewProfile{Name: req.Body.Name, Kind: string(req.Body.Kind), Username: value(req.Body.Username)}
	profile, err := core.AddProfile(ctx, s.DB, accountID, in)
	switch {
	case errors.Is(err, core.ErrUsernameTaken):
		return AddProfile409JSONResponse(usernameTaken), nil
	case err != nil:
		if p, ok := inputProblem(err); ok {
			return AddProfile400JSONResponse{p}, nil
		}
		return nil, err
	}
	return AddProfile201JSONResponse(toProfile(profile)), nil
}

func (s *Server) UpdateProfile(ctx context.Context, req UpdateProfileRequestObject) (UpdateProfileResponseObject, error) {
	accountID, _, ok, err := s.signedIn(ctx)
	if !ok {
		return UpdateProfile401JSONResponse(notSignedIn), err
	}
	changes := core.ProfileChanges{Name: req.Body.Name, Username: req.Body.Username, Private: req.Body.Private}
	profile, err := core.UpdateProfile(ctx, s.DB, accountID, req.Id, changes)
	switch {
	case errors.Is(err, core.ErrNotFound):
		return UpdateProfile404JSONResponse(problem("not_found", "There’s no profile with that ID in your household.")), nil
	case errors.Is(err, core.ErrUsernameTaken):
		return UpdateProfile409JSONResponse(usernameTaken), nil
	case err != nil:
		if p, ok := inputProblem(err); ok {
			return UpdateProfile400JSONResponse{p}, nil
		}
		return nil, err
	}
	return UpdateProfile200JSONResponse(toProfile(profile)), nil
}

func (s *Server) RemoveProfile(ctx context.Context, req RemoveProfileRequestObject) (RemoveProfileResponseObject, error) {
	accountID, _, ok, err := s.signedIn(ctx)
	if !ok {
		return RemoveProfile401JSONResponse(notSignedIn), err
	}
	err = core.RemoveProfile(ctx, s.DB, accountID, req.Id)
	switch {
	case errors.Is(err, core.ErrNotFound):
		return RemoveProfile404JSONResponse(problem("not_found", "There’s no profile with that ID in your household.")), nil
	case errors.Is(err, core.ErrOwnerProfile):
		return RemoveProfile400JSONResponse{problem("owner_profile", "Your own profile can’t be removed. Delete the account instead.")}, nil
	case err != nil:
		return nil, err
	}
	return RemoveProfile204Response{}, nil
}

func (s *Server) ListSessions(ctx context.Context, _ ListSessionsRequestObject) (ListSessionsResponseObject, error) {
	accountID, token, ok, err := s.signedIn(ctx)
	if !ok {
		return ListSessions401JSONResponse{notSignedIn}, err
	}
	sessions, current, err := core.ListSessions(ctx, s.DB, accountID, token)
	if err != nil {
		return nil, err
	}
	out := make(ListSessions200JSONResponse, 0, len(sessions))
	for _, row := range sessions {
		out = append(out, Session{
			Id:         row.ID,
			UserAgent:  row.UserAgent,
			CreatedAt:  row.CreatedAt,
			LastSeenAt: row.LastSeenAt,
			Current:    row.ID == current,
		})
	}
	return out, nil
}

func (s *Server) EndSession(ctx context.Context, req EndSessionRequestObject) (EndSessionResponseObject, error) {
	accountID, _, ok, err := s.signedIn(ctx)
	if !ok {
		return EndSession401JSONResponse{notSignedIn}, err
	}
	err = core.EndSession(ctx, s.DB, accountID, req.Id)
	if errors.Is(err, core.ErrNotFound) {
		return EndSession404JSONResponse(problem("not_found", "That device is already signed out.")), nil
	}
	if err != nil {
		return nil, err
	}
	return EndSession204Response{}, nil
}

func (s *Server) SignOutOthers(ctx context.Context, _ SignOutOthersRequestObject) (SignOutOthersResponseObject, error) {
	accountID, token, ok, err := s.signedIn(ctx)
	if !ok {
		return SignOutOthers401JSONResponse{notSignedIn}, err
	}
	if err := core.SignOutOthers(ctx, s.DB, accountID, token); err != nil {
		return nil, err
	}
	return SignOutOthers204Response{}, nil
}

func (s *Server) DeleteAccount(ctx context.Context, req DeleteAccountRequestObject) (DeleteAccountResponseObject, error) {
	accountID, _, ok, err := s.signedIn(ctx)
	if !ok {
		return DeleteAccount401JSONResponse(notSignedIn), err
	}
	err = core.DeleteAccount(ctx, s.DB, accountID, req.Body.Username)
	if errors.Is(err, core.ErrConfirmMismatch) {
		return DeleteAccount400JSONResponse{problem("confirm_mismatch", "Type your username exactly to confirm.")}, nil
	}
	if err != nil {
		return nil, err
	}
	cookie := s.cookie("", time.Unix(0, 0))
	return DeleteAccount204Response{Headers: DeleteAccount204ResponseHeaders{SetCookie: &cookie}}, nil
}

func (s *Server) ListPasskeys(ctx context.Context, _ ListPasskeysRequestObject) (ListPasskeysResponseObject, error) {
	accountID, _, ok, err := s.signedIn(ctx)
	if !ok {
		return ListPasskeys401JSONResponse{notSignedIn}, err
	}
	passkeys, err := core.ListPasskeys(ctx, s.DB, accountID)
	if err != nil {
		return nil, err
	}
	out := make(ListPasskeys200JSONResponse, 0, len(passkeys))
	for _, p := range passkeys {
		out = append(out, toPasskey(p))
	}
	return out, nil
}

func (s *Server) AddPasskeyOptions(ctx context.Context, _ AddPasskeyOptionsRequestObject) (AddPasskeyOptionsResponseObject, error) {
	accountID, _, ok, err := s.signedIn(ctx)
	if !ok {
		return AddPasskeyOptions401JSONResponse{notSignedIn}, err
	}
	options, err := core.BeginAddPasskey(ctx, s.DB, s.WebAuthn, accountID)
	if err != nil {
		return nil, err
	}
	out, err := toOptions(options)
	return AddPasskeyOptions200JSONResponse(out), err
}

func (s *Server) AddPasskey(ctx context.Context, req AddPasskeyRequestObject) (AddPasskeyResponseObject, error) {
	accountID, _, ok, err := s.signedIn(ctx)
	if !ok {
		return AddPasskey401JSONResponse(notSignedIn), err
	}
	credential, err := json.Marshal(req.Body.Credential)
	if err != nil {
		return nil, err
	}
	passkey, err := core.FinishAddPasskey(ctx, s.DB, s.WebAuthn, accountID, req.Body.Challenge, credential, value(req.Body.Name))
	if p, ok := passkeyProblem(err); ok {
		return AddPasskey400JSONResponse{p}, nil
	}
	if err != nil {
		return nil, err
	}
	return AddPasskey201JSONResponse(toPasskey(passkey)), nil
}

func (s *Server) RemovePasskey(ctx context.Context, req RemovePasskeyRequestObject) (RemovePasskeyResponseObject, error) {
	accountID, _, ok, err := s.signedIn(ctx)
	if !ok {
		return RemovePasskey401JSONResponse{notSignedIn}, err
	}
	err = core.RemovePasskey(ctx, s.DB, accountID, req.Id)
	if errors.Is(err, core.ErrNotFound) {
		return RemovePasskey404JSONResponse(problem("not_found", "That passkey has already been removed.")), nil
	}
	if err != nil {
		return nil, err
	}
	return RemovePasskey204Response{}, nil
}

func (s *Server) PasskeySignInOptions(ctx context.Context, _ PasskeySignInOptionsRequestObject) (PasskeySignInOptionsResponseObject, error) {
	options, err := core.BeginPasskeySignIn(ctx, s.DB, s.WebAuthn)
	if err != nil {
		return nil, err
	}
	out, err := toOptions(options)
	return PasskeySignInOptions200JSONResponse(out), err
}

func (s *Server) PasskeySignIn(ctx context.Context, req PasskeySignInRequestObject) (PasskeySignInResponseObject, error) {
	credential, err := json.Marshal(req.Body.Credential)
	if err != nil {
		return nil, err
	}
	userAgent, _ := ctx.Value(userAgentKey).(string)
	session, err := core.FinishPasskeySignIn(ctx, s.DB, s.WebAuthn, req.Body.Challenge, credential, userAgent)
	if p, ok := passkeyProblem(err); ok {
		return PasskeySignIn400JSONResponse{p}, nil
	}
	if err != nil {
		return nil, err
	}
	accountID, err := core.SessionAccount(ctx, s.DB, session.Token)
	if err != nil {
		return nil, err
	}
	me, err := s.me(ctx, accountID)
	if err != nil {
		return nil, err
	}
	cookie := s.cookie(session.Token, session.ExpiresAt)
	return PasskeySignIn200JSONResponse{Body: me, Headers: PasskeySignIn200ResponseHeaders{SetCookie: &cookie}}, nil
}

var usernameTaken = problem("username_taken", "That username is taken. Try another.")

func passkeyProblem(err error) (ProblemJSONResponse, bool) {
	switch {
	case errors.Is(err, core.ErrChallengeExpired):
		return problem("passkey_expired", "That took too long. Try again."), true
	case errors.Is(err, core.ErrPasskeyRejected):
		return problem("passkey_rejected", "That passkey couldn’t be checked. Try again, or sign in with email."), true
	}
	return ProblemJSONResponse{}, false
}

func toOptions(o core.PasskeyOptions) (PasskeyOptions, error) {
	var options map[string]any
	err := json.Unmarshal(o.Options, &options)
	return PasskeyOptions{Challenge: o.Challenge, Options: options}, err
}

func toPasskey(p store.Passkey) Passkey {
	return Passkey{Id: p.ID, Name: p.Name, CreatedAt: p.CreatedAt, LastUsedAt: p.LastUsedAt}
}
