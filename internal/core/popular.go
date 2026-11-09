package core

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

const (
	popularDays   = 7
	popularSettle = 14
	popularFloor  = 10
	popularRound  = 5
	popularMax    = 10
)

type PopularTitle struct {
	Item       Item
	Households int
}

func Popular(ctx context.Context, db *pgxpool.Pool, profileID int64, now time.Time) ([]PopularTitle, error) {
	q := store.New(db)
	if err := checkOutside(ctx, q, profileID); err != nil {
		return nil, err
	}
	rows, err := q.PopularTitles(ctx, store.PopularTitlesParams{
		Since:     now.AddDate(0, 0, -popularDays),
		Settled:   now.AddDate(0, 0, -popularSettle),
		Floor:     popularFloor,
		MaxTitles: popularMax,
	})
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.TitleID)
	}
	c, err := loadCatalogue(ctx, q, profileID, ids, nil)
	if err != nil {
		return nil, err
	}
	out := make([]PopularTitle, 0, len(rows))
	for _, r := range rows {
		n := int(r.Households)
		out = append(out, PopularTitle{Item: c.item(r.TitleID, nil, nil), Households: n - n%popularRound})
	}
	return out, nil
}
