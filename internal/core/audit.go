package core

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

func PruneAuditLog(ctx context.Context, db *pgxpool.Pool) error {
	return store.New(db).PruneAuditLog(ctx)
}

func audit(ctx context.Context, q *store.Queries, accountID int64, kind, device string) error {
	details, err := json.Marshal(map[string]string{"user_agent": device})
	if err != nil {
		return err
	}
	return q.Audit(ctx, store.AuditParams{AccountID: accountID, Kind: kind, Details: details})
}
