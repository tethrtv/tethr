package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/core"
)

type Cleanup struct {
	DB  *pgxpool.Pool
	Log *slog.Logger
}

func (c Cleanup) Run(ctx context.Context) {
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	for {
		if err := core.PruneAuditLog(ctx, c.DB); err != nil && ctx.Err() == nil {
			c.Log.Error("pruning the audit log", "err", err)
		}
		if err := core.PruneHeldPlays(ctx, c.DB); err != nil && ctx.Err() == nil {
			c.Log.Error("pruning held plays", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
