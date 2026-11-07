package api

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/tethrtv/tethr/internal/core"
)

var noSuchList = problem("not_found", "There’s no list with that ID, or it isn’t shared with you.")

func listProblem(err error) (ProblemJSONResponse, int, bool) {
	switch {
	case errors.Is(err, core.ErrNotFound):
		return noSuchList, 404, true
	case errors.Is(err, core.ErrListName):
		return problem("invalid_name", "Give the list a name of up to 100 characters."), 400, true
	case errors.Is(err, core.ErrListDescription):
		return problem("invalid_description", "Descriptions are up to 1,000 characters."), 400, true
	case errors.Is(err, core.ErrListNotes):
		return problem("invalid_notes", "Notes are up to 500 characters."), 400, true
	case errors.Is(err, core.ErrListPrivacy):
		return problem("invalid_privacy", "Lists are private, household or public."), 400, true
	case errors.Is(err, core.ErrListSort):
		return problem("invalid_sort", "Lists sort by rank, added, title or year."), 400, true
	case errors.Is(err, core.ErrListCover):
		return problem("invalid_cover", "The cover has to be an item on the list."), 400, true
	case errors.Is(err, core.ErrListOrder):
		return problem("invalid_order", "Send every item on the list once, in the new order."), 400, true
	case errors.Is(err, core.ErrListReadOnly):
		return problem("read_only", "You can see this list but not change it."), 403, true
	case errors.Is(err, core.ErrPublicList):
		return problem("public_not_allowed", "Kids and guest profiles can’t make public lists."), 403, true
	case errors.Is(err, core.ErrTooManyLists):
		return problem("too_many_lists", "A profile can have up to 100 lists."), 422, true
	case errors.Is(err, core.ErrListFull):
		return problem("list_full", "A list holds up to 1,000 items."), 422, true
	}
	return ProblemJSONResponse{}, 0, false
}

func (s *Server) reader(ctx context.Context, profileID *int64) (int64, bool, error) {
	_, bearer := ctx.Value(bearerKey).(string)
	_, session := ctx.Value(sessionKey).(string)
	if !bearer && !session {
		return 0, true, nil
	}
	c, ok, err := s.profileCaller(ctx, profileID)
	if errors.Is(err, core.ErrNotFound) {
		return 0, true, err
	}
	if !ok && !bearer && err == nil {
		return 0, true, nil
	}
	return c.ProfileID, ok, err
}

func (s *Server) toList(l core.List) List {
	cover := make([]Item, 0, len(l.Cover))
	for _, i := range l.Cover {
		cover = append(cover, toItem(i))
	}
	return List{
		Id:           l.ID,
		Name:         l.Name,
		Slug:         l.Slug,
		Description:  l.Description,
		Privacy:      ListPrivacy(l.Privacy),
		SortBy:       ListSortBy(l.SortBy),
		SortHow:      ListSortHow(l.SortHow),
		Owner:        ListOwner{Username: l.Owner.Username, Name: l.Owner.Name},
		Access:       ListAccess(l.Access),
		CanEditItems: l.CanEdit,
		InUpNext:     l.InUpNext,
		ItemCount:    l.ItemCount,
		WatchedCount: l.Watched,
		CoverItemId:  l.CoverItemID,
		Cover:        cover,
		Url:          s.AppURL + "/lists/" + strconv.FormatInt(l.ID, 10) + "-" + l.Slug,
		CreatedAt:    l.CreatedAt,
		UpdatedAt:    l.UpdatedAt,
	}
}

func (s *Server) etag(l core.List) string {
	return `"` + s.Version + "-" + l.Version + `"`
}

func (s *Server) unchanged(l core.List, ifNoneMatch *string) bool {
	if ifNoneMatch == nil {
		return false
	}
	for _, tag := range strings.Split(*ifNoneMatch, ",") {
		tag = strings.TrimPrefix(strings.TrimSpace(tag), "W/")
		if tag == s.etag(l) || tag == "*" {
			return true
		}
	}
	return false
}

func (s *Server) ListLists(ctx context.Context, req ListListsRequestObject) (ListListsResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return ListLists404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return ListLists401JSONResponse{notConnected}, err
	}
	lists, err := core.Lists(ctx, s.DB, c.ProfileID)
	if err != nil {
		return nil, err
	}
	out := make(ListLists200JSONResponse, 0, len(lists))
	for _, l := range lists {
		out = append(out, s.toList(l))
	}
	return out, nil
}

func (s *Server) CreateList(ctx context.Context, req CreateListRequestObject) (CreateListResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return CreateList404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return CreateList401JSONResponse(notConnected), err
	}
	b := req.Body
	l, err := core.CreateList(ctx, s.DB, c.ProfileID, core.ListChanges{
		Name:        &b.Name,
		Description: b.Description,
		Privacy:     (*string)(b.Privacy),
		SortBy:      (*string)(b.SortBy),
		SortHow:     (*string)(b.SortHow),
	})
	if p, status, ok := listProblem(err); ok {
		switch status {
		case 403:
			return CreateList403JSONResponse(p), nil
		case 422:
			return CreateList422JSONResponse(p), nil
		}
		return CreateList400JSONResponse{p}, nil
	}
	if err != nil {
		return nil, err
	}
	return CreateList201JSONResponse(s.toList(l)), nil
}

func (s *Server) GetList(ctx context.Context, req GetListRequestObject) (GetListResponseObject, error) {
	viewer, ok, err := s.reader(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return GetList404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return GetList401JSONResponse{notConnected}, err
	}
	l, err := core.GetList(ctx, s.DB, viewer, req.Id)
	if errors.Is(err, core.ErrNotFound) {
		return GetList404JSONResponse(noSuchList), nil
	}
	if err != nil {
		return nil, err
	}
	if s.unchanged(l, req.Params.IfNoneMatch) {
		return GetList304Response{}, nil
	}
	tag := s.etag(l)
	return GetList200JSONResponse{Body: s.toList(l), Headers: GetList200ResponseHeaders{ETag: &tag}}, nil
}

func (s *Server) UpdateList(ctx context.Context, req UpdateListRequestObject) (UpdateListResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return UpdateList404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return UpdateList401JSONResponse(notConnected), err
	}
	b := req.Body
	l, err := core.UpdateList(ctx, s.DB, c.ProfileID, req.Id, core.ListChanges{
		Name:        b.Name,
		Description: b.Description,
		Privacy:     (*string)(b.Privacy),
		SortBy:      (*string)(b.SortBy),
		SortHow:     (*string)(b.SortHow),
		Cover:       b.CoverItemId,
	})
	if p, status, ok := listProblem(err); ok {
		switch status {
		case 403:
			return UpdateList403JSONResponse(p), nil
		case 404:
			return UpdateList404JSONResponse(p), nil
		}
		return UpdateList400JSONResponse{p}, nil
	}
	if err != nil {
		return nil, err
	}
	return UpdateList200JSONResponse(s.toList(l)), nil
}

func (s *Server) DeleteList(ctx context.Context, req DeleteListRequestObject) (DeleteListResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return DeleteList404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return DeleteList401JSONResponse{notConnected}, err
	}
	err = core.DeleteList(ctx, s.DB, c.ProfileID, req.Id)
	if errors.Is(err, core.ErrNotFound) {
		return DeleteList404JSONResponse(noSuchList), nil
	}
	if err != nil {
		return nil, err
	}
	return DeleteList204Response{}, nil
}

func (s *Server) ListListItems(ctx context.Context, req ListListItemsRequestObject) (ListListItemsResponseObject, error) {
	viewer, ok, err := s.reader(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return ListListItems404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return ListListItems401JSONResponse{notConnected}, err
	}
	p := req.Params
	query := core.ListQuery{HideWatched: value(p.HideWatched), Page: core.Page{Page: max(value(p.Page), 1), Limit: min(max(value(p.Limit), 0), 1000)}}
	if query.Page.Limit == 0 {
		query.Page.Limit = 100
	}
	if p.Type != nil {
		query.Kind = string(*p.Type)
	}
	if p.SortBy != nil {
		query.SortBy = string(*p.SortBy)
	}
	if p.SortHow != nil {
		query.SortHow = string(*p.SortHow)
	}
	l, entries, total, err := core.ListEntries(ctx, s.DB, viewer, req.Id, query)
	if errors.Is(err, core.ErrNotFound) {
		return ListListItems404JSONResponse(noSuchList), nil
	}
	if err != nil {
		return nil, err
	}
	if s.unchanged(l, p.IfNoneMatch) {
		return ListListItems304Response{}, nil
	}
	out := ListItemsPage{List: s.toList(l), Items: make([]ListItem, 0, len(entries)), Total: total, Page: query.Page.Page, Limit: query.Page.Limit}
	for _, e := range entries {
		item := ListItem{Id: e.ID, Rank: int(e.Rank), ListedAt: e.At, Notes: e.Notes, Item: toItem(e.Item)}
		if e.Progress != nil {
			item.Progress = &ItemProgress{Watched: e.Progress.Watched, Total: e.Progress.Total, LastWatchedAt: e.Progress.LastWatchedAt}
		}
		out.Items = append(out.Items, item)
	}
	tag := s.etag(l)
	return ListListItems200JSONResponse{Body: out, Headers: ListListItems200ResponseHeaders{ETag: &tag}}, nil
}

func (s *Server) AddToList(ctx context.Context, req AddToListRequestObject) (AddToListResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return AddToList404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return AddToList401JSONResponse(notConnected), err
	}
	listings := make([]core.Listing, 0, len(req.Body.Items))
	for _, l := range req.Body.Items {
		listings = append(listings, core.Listing{Ref: fromRef(l.Item), Notes: value(l.Notes)})
	}
	result, err := core.AddToList(ctx, s.DB, c.ProfileID, req.Id, listings)
	if p, status, ok := listProblem(err); ok {
		switch status {
		case 403:
			return AddToList403JSONResponse(p), nil
		case 404:
			return AddToList404JSONResponse(p), nil
		case 422:
			return AddToList422JSONResponse(p), nil
		}
		return AddToList400JSONResponse{p}, nil
	}
	if err != nil {
		return nil, err
	}
	return AddToList200JSONResponse(SyncResult{Added: total(result.Added), Existing: total(result.Existing), NotFound: toRefs(result.NotFound)}), nil
}

func (s *Server) UpdateListItem(ctx context.Context, req UpdateListItemRequestObject) (UpdateListItemResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return UpdateListItem404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return UpdateListItem401JSONResponse(notConnected), err
	}
	err = core.SetListNotes(ctx, s.DB, c.ProfileID, req.Id, req.Item, req.Body.Notes)
	if p, status, ok := listProblem(err); ok {
		switch status {
		case 403:
			return UpdateListItem403JSONResponse(p), nil
		case 404:
			return UpdateListItem404JSONResponse(problem("not_found", "That isn’t on the list.")), nil
		}
		return UpdateListItem400JSONResponse{p}, nil
	}
	if err != nil {
		return nil, err
	}
	return UpdateListItem204Response{}, nil
}

func (s *Server) RemoveFromList(ctx context.Context, req RemoveFromListRequestObject) (RemoveFromListResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return RemoveFromList404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return RemoveFromList401JSONResponse{notConnected}, err
	}
	err = core.RemoveFromList(ctx, s.DB, c.ProfileID, req.Id, req.Item)
	if errors.Is(err, core.ErrListReadOnly) {
		p, _, _ := listProblem(err)
		return RemoveFromList403JSONResponse(p), nil
	}
	if errors.Is(err, core.ErrNotFound) {
		return RemoveFromList404JSONResponse(problem("not_found", "That isn’t on the list.")), nil
	}
	if err != nil {
		return nil, err
	}
	return RemoveFromList204Response{}, nil
}

func (s *Server) ReorderList(ctx context.Context, req ReorderListRequestObject) (ReorderListResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return ReorderList404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return ReorderList401JSONResponse(notConnected), err
	}
	err = core.ReorderList(ctx, s.DB, c.ProfileID, req.Id, req.Body.Ids)
	if p, status, ok := listProblem(err); ok {
		switch status {
		case 403:
			return ReorderList403JSONResponse(p), nil
		case 404:
			return ReorderList404JSONResponse(p), nil
		}
		return ReorderList400JSONResponse{p}, nil
	}
	if err != nil {
		return nil, err
	}
	return ReorderList204Response{}, nil
}

func (s *Server) RemoveManyFromList(ctx context.Context, req RemoveManyFromListRequestObject) (RemoveManyFromListResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return RemoveManyFromList404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return RemoveManyFromList401JSONResponse{notConnected}, err
	}
	removed, err := core.RemoveManyFromList(ctx, s.DB, c.ProfileID, req.Id, req.Body.Ids)
	if errors.Is(err, core.ErrListReadOnly) {
		p, _, _ := listProblem(err)
		return RemoveManyFromList403JSONResponse(p), nil
	}
	if errors.Is(err, core.ErrNotFound) {
		return RemoveManyFromList404JSONResponse(noSuchList), nil
	}
	if err != nil {
		return nil, err
	}
	return RemoveManyFromList200JSONResponse{Removed: removed}, nil
}

var autoListWords = map[string][2]string{
	"stopped":    {"Stopped watching", "Shows you started and haven’t watched for 90 days, or took out of Up next."},
	"favourites": {"Favourites", "Films and shows you rated 9 or 10."},
	"rewatch":    {"Due a rewatch", "Things you rated 8 or more and last watched over a year ago."},
	"household":  {"Household favourites", "Anything someone in the household rated 9 or 10."},
}

var noSuchAutoList = problem("not_found", "There’s no list of that kind for this profile.")

func toAutoList(l core.AutoList) AutoList {
	cover := make([]Item, 0, len(l.Cover))
	for _, i := range l.Cover {
		cover = append(cover, toItem(i))
	}
	words := autoListWords[l.Kind]
	return AutoList{Kind: AutoListKind(l.Kind), Name: words[0], Description: words[1], ItemCount: l.Count, Cover: cover}
}

func (s *Server) ListAutoLists(ctx context.Context, req ListAutoListsRequestObject) (ListAutoListsResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return ListAutoLists404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return ListAutoLists401JSONResponse{notConnected}, err
	}
	lists, err := core.AutoLists(ctx, s.DB, c.ProfileID)
	if err != nil {
		return nil, err
	}
	out := make(ListAutoLists200JSONResponse, 0, len(lists))
	for _, l := range lists {
		out = append(out, toAutoList(l))
	}
	return out, nil
}

func (s *Server) ListAutoListItems(ctx context.Context, req ListAutoListItemsRequestObject) (ListAutoListItemsResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return ListAutoListItems404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return ListAutoListItems401JSONResponse{notConnected}, err
	}
	p := req.Params
	query := core.ListQuery{HideWatched: value(p.HideWatched), Page: core.Page{Page: max(value(p.Page), 1), Limit: min(max(value(p.Limit), 0), 1000)}}
	if query.Page.Limit == 0 {
		query.Page.Limit = 100
	}
	if p.Type != nil {
		query.Kind = string(*p.Type)
	}
	if p.SortBy != nil {
		query.SortBy = string(*p.SortBy)
	}
	if p.SortHow != nil {
		query.SortHow = string(*p.SortHow)
	}
	l, entries, total, err := core.AutoListEntries(ctx, s.DB, c.ProfileID, string(req.Kind), query)
	if errors.Is(err, core.ErrNotFound) {
		return ListAutoListItems404JSONResponse(noSuchAutoList), nil
	}
	if err != nil {
		return nil, err
	}
	out := ListAutoListItems200JSONResponse{List: toAutoList(l), Items: make([]AutoListItem, 0, len(entries)), Total: total, Page: query.Page.Page, Limit: query.Page.Limit}
	for _, e := range entries {
		item := AutoListItem{Rank: int(e.Rank), At: e.At, Item: toItem(e.Item)}
		if e.Rating != nil {
			rating := int(*e.Rating)
			item.Rating = &rating
		}
		if e.Progress != nil {
			item.Progress = &ItemProgress{Watched: e.Progress.Watched, Total: e.Progress.Total, LastWatchedAt: e.Progress.LastWatchedAt}
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

func (s *Server) CopyAutoList(ctx context.Context, req CopyAutoListRequestObject) (CopyAutoListResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return CopyAutoList404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return CopyAutoList401JSONResponse{notConnected}, err
	}
	l, err := core.CopyAutoList(ctx, s.DB, c.ProfileID, string(req.Kind), autoListWords[string(req.Kind)][0])
	if errors.Is(err, core.ErrNotFound) {
		return CopyAutoList404JSONResponse(noSuchAutoList), nil
	}
	if p, status, ok := listProblem(err); ok && status == 422 {
		return CopyAutoList422JSONResponse(p), nil
	}
	if err != nil {
		return nil, err
	}
	return CopyAutoList201JSONResponse(s.toList(l)), nil
}
