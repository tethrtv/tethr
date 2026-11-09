package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/mail"
)

var (
	outsideHousehold = problem("outside_household", "Kids and guest profiles only see lists from their own household.")
	noSuchUser       = problem("not_found", "There’s nobody with that username.")
)

func (s *Server) ListFollowedLists(ctx context.Context, req ListFollowedListsRequestObject) (ListFollowedListsResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return ListFollowedLists404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return ListFollowedLists401JSONResponse{notConnected}, err
	}
	lists, err := core.FollowedLists(ctx, s.DB, c.ProfileID)
	if err != nil {
		return nil, err
	}
	out := make(ListFollowedLists200JSONResponse, 0, len(lists))
	for _, l := range lists {
		out = append(out, s.toList(l))
	}
	return out, nil
}

func (s *Server) FollowList(ctx context.Context, req FollowListRequestObject) (FollowListResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return FollowList404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return FollowList401JSONResponse{notConnected}, err
	}
	l, err := core.FollowList(ctx, s.DB, c.ProfileID, req.Id)
	switch {
	case errors.Is(err, core.ErrNotFound):
		return FollowList404JSONResponse(noSuchList), nil
	case errors.Is(err, core.ErrFollowHousehold):
		return FollowList409JSONResponse(problem("household_list", "This list is already on your lists page, so there’s no need to follow it.")), nil
	case errors.Is(err, core.ErrOutsideHousehold):
		return FollowList403JSONResponse(outsideHousehold), nil
	case err != nil:
		return nil, err
	}
	return FollowList200JSONResponse(s.toList(l)), nil
}

func (s *Server) UnfollowList(ctx context.Context, req UnfollowListRequestObject) (UnfollowListResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return UnfollowList404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return UnfollowList401JSONResponse{notConnected}, err
	}
	if err := core.UnfollowList(ctx, s.DB, c.ProfileID, req.Id); err != nil {
		return nil, err
	}
	return UnfollowList204Response{}, nil
}

func (s *Server) ListFollowers(ctx context.Context, req ListFollowersRequestObject) (ListFollowersResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return ListFollowers404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return ListFollowers401JSONResponse{notConnected}, err
	}
	followers, err := core.ListFollowers(ctx, s.DB, c.ProfileID, req.Id)
	switch {
	case errors.Is(err, core.ErrNotFound):
		return ListFollowers404JSONResponse(noSuchList), nil
	case errors.Is(err, core.ErrFollowersOwner):
		return ListFollowers403JSONResponse(problem("not_yours", "Only the list’s owner can see who follows it.")), nil
	case err != nil:
		return nil, err
	}
	out := make(ListFollowers200JSONResponse, 0, len(followers))
	for _, f := range followers {
		out = append(out, Follower{Username: f.Username, Name: optionalString(f.Name), FollowedAt: f.FollowedAt})
	}
	return out, nil
}

func (s *Server) BrowseLists(ctx context.Context, req BrowseListsRequestObject) (BrowseListsResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return BrowseLists404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return BrowseLists401JSONResponse{notConnected}, err
	}
	p := req.Params
	query := core.BrowseQuery{
		Popular:  p.Order != nil && *p.Order == Popular,
		Rising:   p.Order != nil && *p.Order == Rising,
		Official: p.Official,
		Search:   value(p.Q),
		Page:     core.Page{Page: max(value(p.Page), 1), Limit: min(max(value(p.Limit), 0), 100)},
	}
	if query.Page.Limit == 0 {
		query.Page.Limit = 20
	}
	lists, total, err := core.BrowseLists(ctx, s.DB, c.ProfileID, query)
	if errors.Is(err, core.ErrOutsideHousehold) {
		return BrowseLists403JSONResponse(outsideHousehold), nil
	}
	if err != nil {
		return nil, err
	}
	out := BrowseLists200JSONResponse{Lists: make([]List, 0, len(lists)), Total: total, Page: query.Page.Page, Limit: query.Page.Limit}
	for _, l := range lists {
		out.Lists = append(out.Lists, s.toList(l))
	}
	return out, nil
}

func (s *Server) GetUser(ctx context.Context, req GetUserRequestObject) (GetUserResponseObject, error) {
	viewer, ok, err := s.reader(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return GetUser404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return GetUser401JSONResponse{notConnected}, err
	}
	page, err := core.ProfilePage(ctx, s.DB, viewer, req.Username)
	switch {
	case errors.Is(err, core.ErrNotFound):
		return GetUser404JSONResponse(noSuchUser), nil
	case errors.Is(err, core.ErrOutsideHousehold):
		return GetUser403JSONResponse(outsideHousehold), nil
	case err != nil:
		return nil, err
	}
	out := GetUser200JSONResponse{Username: page.Username, Name: optionalString(page.Name), Private: page.Private, Lists: make([]List, 0, len(page.Lists))}
	for _, l := range page.Lists {
		out.Lists = append(out.Lists, s.toList(l))
	}
	return out, nil
}

func (s *Server) ReportList(ctx context.Context, req ReportListRequestObject) (ReportListResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return ReportList404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return ReportList401JSONResponse(notConnected), err
	}
	r, isNew, err := core.ReportList(ctx, s.DB, c.ProfileID, req.Id, string(req.Body.Reason), value(req.Body.Note))
	switch {
	case errors.Is(err, core.ErrNotFound):
		return ReportList404JSONResponse(noSuchList), nil
	case errors.Is(err, core.ErrReportReason):
		return ReportList400JSONResponse{problem("invalid_reason", "Pick a reason for the report.")}, nil
	case errors.Is(err, core.ErrReportNote):
		return ReportList400JSONResponse{problem("invalid_note", "Notes are up to 500 characters.")}, nil
	case errors.Is(err, core.ErrReportOfficial):
		return ReportList403JSONResponse(problem("official_list", "Lists made by Tethr can’t be reported. If something on one is wrong, email reports@tethr.tv.")), nil
	case errors.Is(err, core.ErrTooManyReports):
		return ReportList429JSONResponse(problem("too_many_reports", "That’s a lot of reports for one day. If something urgent needs a look, email reports@tethr.tv.")), nil
	case err != nil:
		return nil, err
	}
	if isNew {
		if err := s.Mail.Send(ctx, s.reportEmail(r)); err != nil {
			s.log().Error("emailing a report", "list", r.List.ID, "err", err)
		}
	}
	return ReportList204Response{}, nil
}

func (s *Server) reportEmail(r core.Report) mail.Message {
	note := "No note."
	if r.Note != nil {
		note = *r.Note
	}
	id := strconv.FormatInt(r.List.ID, 10)
	text := fmt.Sprintf("%s reported %q by %s for %s.\n\n%s\n\n%s%s\n\nIt's out of browsing until you decide. On the server:\n\n  tethr report keep %s\n  tethr report remove %s <reason the owner will read>\n",
		r.Reporter, r.List.Name, r.List.Owner.Username, r.Reason, note, s.AppURL, r.List.Path(), id, id)
	return mail.Message{To: s.ReportsTo, Subject: "Report: " + r.List.Name, Text: text}
}

func RemovedListEmail(appURL string, removed core.RemovedList, reason string) (mail.Message, error) {
	data := struct{ Title, Preview, Lead, Button, Link, Note, Footer, AppURL, Image, ImageAlt string }{
		Title:   "We made one of your lists private",
		Preview: fmt.Sprintf("Your list “%s” broke Tethr’s rules.", removed.Name),
		Lead:    fmt.Sprintf("Your list “%s” was reported, and we found it breaks Tethr’s rules: %s. We’ve made it private, so only you can see it now. Nothing on it has been deleted.", removed.Name, reason),
		Button:  "Read the terms",
		Link:    appURL + "/terms",
		Note:    "If you think we got this wrong, reply to this email and we’ll look again.",
		Footer:  "We email you whenever we act on a report about something of yours.",
		AppURL:  appURL,
	}
	var html bytes.Buffer
	if err := emails.ExecuteTemplate(&html, "notice.html", data); err != nil {
		return mail.Message{}, err
	}
	text := fmt.Sprintf("%s\n\n%s\n\n%s\n\n%s\n", data.Title, data.Lead, data.Note, data.Link)
	return mail.Message{To: removed.Email, Subject: data.Title, Text: text, HTML: html.String()}, nil
}

func (s *Server) GetUserList(ctx context.Context, req GetUserListRequestObject) (GetUserListResponseObject, error) {
	viewer, ok, err := s.reader(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return GetUserList404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return GetUserList401JSONResponse{notConnected}, err
	}
	l, err := core.FindList(ctx, s.DB, viewer, req.Username, req.Slug)
	switch {
	case errors.Is(err, core.ErrNotFound):
		return GetUserList404JSONResponse(noSuchList), nil
	case err != nil:
		return nil, err
	}
	return GetUserList200JSONResponse(s.toList(l)), nil
}
