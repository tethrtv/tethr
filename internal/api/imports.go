package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"

	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/traktexport"
)

const maxUpload = 100 << 20

func (s *Server) profileCaller(ctx context.Context, profileID *int64) (core.Caller, bool, error) {
	if _, bearer := ctx.Value(bearerKey).(string); bearer || profileID == nil {
		return s.caller(ctx)
	}
	accountID, _, ok, err := s.signedIn(ctx)
	if !ok {
		return core.Caller{}, false, err
	}
	c, err := core.HouseholdCaller(ctx, s.DB, accountID, *profileID)
	if errors.Is(err, core.ErrNotFound) {
		return core.Caller{}, true, err
	}
	return c, err == nil, err
}

func toImport(i core.Import) Import {
	count := func(c core.ImportCount) ImportCount {
		return ImportCount{Total: c.Total, Added: c.Added, Skipped: c.Skipped, Unmatched: c.Unmatched}
	}
	return Import{
		Id:        i.ID,
		Source:    ImportSource(i.Source),
		Filename:  i.Filename,
		Status:    ImportStatus(i.Status),
		CreatedAt: i.CreatedAt,
		AppliedAt: i.AppliedAt,
		UndoneAt:  i.UndoneAt,
		Summary: ImportSummary{
			History:   count(i.Summary.History),
			Watchlist: count(i.Summary.Watchlist),
			Playback:  count(i.Summary.Playback),
			Ratings:   count(i.Summary.Ratings),
			Hidden:    count(i.Summary.Hidden),
			Unmatched: i.Summary.Unmatched,
		},
	}
}

func (s *Server) ListImports(ctx context.Context, req ListImportsRequestObject) (ListImportsResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return ListImports404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return ListImports401JSONResponse{notConnected}, err
	}
	imports, err := core.Imports(ctx, s.DB, c.ProfileID)
	if err != nil {
		return nil, err
	}
	out := make(ListImports200JSONResponse, 0, len(imports))
	for _, i := range imports {
		out = append(out, toImport(i))
	}
	return out, nil
}

func (s *Server) CreateImport(ctx context.Context, req CreateImportRequestObject) (CreateImportResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return CreateImport404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return CreateImport401JSONResponse(notConnected), err
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, maxUpload+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxUpload {
		return CreateImport413JSONResponse(problem("too_big", "Exports up to 100 MB can be imported.")), nil
	}
	file, err := traktexport.Read(bytes.NewReader(body), int64(len(body)))
	var bad traktexport.BadFileError
	switch {
	case errors.Is(err, traktexport.ErrNotAnExport):
		return CreateImport400JSONResponse{problem("not_an_export", "That isn’t an export file Tethr can read. Download the zip from your old tracker’s settings and send it as it is.")}, nil
	case errors.As(err, &bad):
		return CreateImport400JSONResponse{problem("unreadable_export", bad.Name+" in that export couldn’t be read. Try downloading the export again.")}, nil
	case err != nil:
		return nil, err
	}
	name := strings.TrimSpace(value(req.Params.Filename))
	imp, err := core.CreateImport(ctx, s.DB, c.ProfileID, name, file)
	switch {
	case errors.Is(err, core.ErrEmptyImport):
		return CreateImport400JSONResponse{problem("empty_export", "That export has no history, watchlist, resume points, ratings or hidden items Tethr can match.")}, nil
	case errors.Is(err, core.ErrImportBusy):
		return CreateImport409JSONResponse(problem("import_running", "An import is already running for this profile. Try again when it’s finished.")), nil
	case err != nil:
		return nil, err
	}
	return CreateImport201JSONResponse(toImport(imp)), nil
}

func (s *Server) GetImport(ctx context.Context, req GetImportRequestObject) (GetImportResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return GetImport404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return GetImport401JSONResponse{notConnected}, err
	}
	imp, err := core.ImportByID(ctx, s.DB, c.ProfileID, req.Id)
	if errors.Is(err, core.ErrNotFound) {
		return GetImport404JSONResponse(noSuchImport), nil
	}
	if err != nil {
		return nil, err
	}
	return GetImport200JSONResponse(toImport(imp)), nil
}

var noSuchImport = problem("not_found", "There’s no import with that ID.")

func (s *Server) ApplyImport(ctx context.Context, req ApplyImportRequestObject) (ApplyImportResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return ApplyImport404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return ApplyImport401JSONResponse{notConnected}, err
	}
	imp, err := core.QueueImport(ctx, s.DB, c.ProfileID, req.Id)
	if errors.Is(err, core.ErrNotFound) {
		return ApplyImport404JSONResponse(problem("not_found", "There’s no import with that ID waiting to be applied. Imports are kept for 24 hours.")), nil
	}
	if err != nil {
		return nil, err
	}
	return ApplyImport202JSONResponse(toImport(imp)), nil
}

func (s *Server) UndoImport(ctx context.Context, req UndoImportRequestObject) (UndoImportResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return UndoImport404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return UndoImport401JSONResponse{notConnected}, err
	}
	imp, err := core.UndoImport(ctx, s.DB, c.ProfileID, req.Id)
	if errors.Is(err, core.ErrNotFound) {
		return UndoImport404JSONResponse(problem("not_found", "There’s no applied import with that ID from the last 30 days.")), nil
	}
	if err != nil {
		return nil, err
	}
	return UndoImport200JSONResponse(toImport(imp)), nil
}

func (s *Server) ListRatings(ctx context.Context, req ListRatingsRequestObject) (ListRatingsResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return ListRatings404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return ListRatings401JSONResponse{notConnected}, err
	}
	kind := ""
	if req.Params.Type != nil {
		kind = string(*req.Params.Type)
	}
	var ratings []int16
	for _, r := range value(req.Params.Rating) {
		ratings = append(ratings, int16(r))
	}
	entries, err := core.Ratings(ctx, s.DB, c.ProfileID, kind, ratings)
	if err != nil {
		return nil, err
	}
	out := make(ListRatings200JSONResponse, 0, len(entries))
	for _, e := range entries {
		out = append(out, Rating{Id: e.ID, Rating: int(e.Rating), RatedAt: e.At, Item: toItem(e.Item)})
	}
	return out, nil
}

func (s *Server) Rate(ctx context.Context, req RateRequestObject) (RateResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return Rate404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return Rate401JSONResponse(notConnected), err
	}
	ratings := make([]core.Rating, 0, len(req.Body.Items))
	for _, r := range req.Body.Items {
		ratings = append(ratings, core.Rating{Ref: fromRef(r.Item), Rating: int16(min(max(r.Rating, 0), 11)), At: r.RatedAt})
	}
	result, err := core.Rate(ctx, s.DB, c.ProfileID, ratings)
	if err != nil {
		return nil, err
	}
	return Rate200JSONResponse(SyncResult{Added: total(result.Added), Existing: 0, NotFound: toRefs(result.NotFound)}), nil
}

func (s *Server) RemoveRating(ctx context.Context, req RemoveRatingRequestObject) (RemoveRatingResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return RemoveRating404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return RemoveRating401JSONResponse{notConnected}, err
	}
	err = core.RemoveRating(ctx, s.DB, c.ProfileID, req.Id)
	if errors.Is(err, core.ErrNotFound) {
		return RemoveRating404JSONResponse(problem("not_found", "There’s no rating with that ID.")), nil
	}
	if err != nil {
		return nil, err
	}
	return RemoveRating204Response{}, nil
}
