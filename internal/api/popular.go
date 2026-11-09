package api

import (
	"context"
	"errors"
	"time"

	"github.com/tethrtv/tethr/internal/core"
)

func (s *Server) GetPopular(ctx context.Context, req GetPopularRequestObject) (GetPopularResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return GetPopular404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return GetPopular401JSONResponse{notConnected}, err
	}
	titles, err := core.Popular(ctx, s.DB, c.ProfileID, time.Now())
	if errors.Is(err, core.ErrOutsideHousehold) {
		return GetPopular403JSONResponse(outsideHousehold), nil
	}
	if err != nil {
		return nil, err
	}
	out := make(GetPopular200JSONResponse, 0, len(titles))
	for _, t := range titles {
		out = append(out, PopularTitle{Item: toItem(t.Item), Households: t.Households})
	}
	return out, nil
}
