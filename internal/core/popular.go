package core

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

const (
	popularDays  = 7
	popularFloor = 10
	popularMax   = 10
)

type PopularTitle struct {
	Item       Item
	Households int
}

// Titles only show once enough households have played them that no one person's viewing can be picked out.
func Popular(ctx context.Context, db *pgxpool.Pool, profileID int64, now time.Time) ([]PopularTitle, error) {
	q := store.New(db)
	rows, err := q.PopularTitles(ctx, store.PopularTitlesParams{Since: now.AddDate(0, 0, -popularDays), Floor: popularFloor, MaxTitles: popularMax})
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
		out = append(out, PopularTitle{Item: c.item(r.TitleID, nil, nil), Households: int(r.Households)})
	}
	return out, nil
}
