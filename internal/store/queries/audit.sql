-- name: Audit :exec
INSERT INTO audit_log (account_id, profile_id, kind, subject, details)
VALUES ($1, $2, $3, $4, $5);

-- name: CountAudit :one
SELECT count(*) FROM audit_log
WHERE account_id = $1 AND kind = $2 AND created_at > $3;

-- name: PruneAuditLog :exec
DELETE FROM audit_log WHERE created_at < now() - interval '90 days';

-- name: LockAccount :exec
SELECT id FROM accounts WHERE id = $1 FOR UPDATE;
