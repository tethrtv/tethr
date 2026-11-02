package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/core"
)

type Imports struct {
	DB  *pgxpool.Pool
	Log *slog.Logger
}

func (im Imports) Run(ctx context.Context) {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		if err := im.Apply(ctx); err != nil && ctx.Err() == nil {
			im.Log.Error("applying imports", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (im Imports) Apply(ctx context.Context) error {
	if err := core.ExpireImports(ctx, im.DB); err != nil {
		return err
	}
	for {
		found, err := core.ApplyNextImport(ctx, im.DB)
		if err != nil || !found {
			return err
		}
	}
}
