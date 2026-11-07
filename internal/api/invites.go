package api

import (
	"context"
	"errors"

	"github.com/tethrtv/tethr/internal/core"
)

func (s *Server) toInvite(i core.Invite) Invite {
	return Invite{Code: i.Code, Url: s.AppURL + "/join?invite=" + i.Code, CreatedAt: i.CreatedAt, UsedAt: i.UsedAt, UsedBy: optionalString(i.UsedBy)}
}

func (s *Server) ListInvites(ctx context.Context, _ ListInvitesRequestObject) (ListInvitesResponseObject, error) {
	accountID, _, ok, err := s.signedIn(ctx)
	if !ok {
		return ListInvites401JSONResponse{notSignedIn}, err
	}
	if !s.RequireInvite {
		return ListInvites200JSONResponse{Open: true, Invites: []Invite{}}, nil
	}
	invites, err := core.AccountInvites(ctx, s.DB, accountID)
	if err != nil {
		return nil, err
	}
	out := ListInvites200JSONResponse{Paused: invites.Paused, Left: invites.Left, Invites: make([]Invite, 0, len(invites.Made))}
	for _, i := range invites.Made {
		out.Invites = append(out.Invites, s.toInvite(i))
	}
	return out, nil
}

func (s *Server) MakeInvite(ctx context.Context, _ MakeInviteRequestObject) (MakeInviteResponseObject, error) {
	accountID, _, ok, err := s.signedIn(ctx)
	if !ok {
		return MakeInvite401JSONResponse{notSignedIn}, err
	}
	if !s.RequireInvite {
		return MakeInvite409JSONResponse(problem("signups_open", "Anyone can sign up, so there’s no need for an invite.")), nil
	}
	i, err := core.MakeInvite(ctx, s.DB, accountID)
	switch {
	case errors.Is(err, core.ErrInvitesPaused):
		return MakeInvite409JSONResponse(problem("invites_paused", "New invites are paused for a little while. The ones you’ve made still work.")), nil
	case errors.Is(err, core.ErrNoInvitesLeft):
		return MakeInvite409JSONResponse(problem("no_invites_left", "You’ve used all your invites.")), nil
	case err != nil:
		return nil, err
	}
	return MakeInvite201JSONResponse(s.toInvite(i)), nil
}

func (s *Server) CheckInvite(ctx context.Context, req CheckInviteRequestObject) (CheckInviteResponseObject, error) {
	c, err := core.CheckInvite(ctx, s.DB, req.Code)
	if err != nil {
		return nil, err
	}
	return CheckInvite200JSONResponse{Valid: c.Valid, InvitedBy: optionalString(c.InvitedBy)}, nil
}
