-- name: CreateImport :one
INSERT INTO imports (profile_id, source, filename, payload, status, summary)
VALUES ($1, $2, $3, $4, 'parsed', $5)
RETURNING id, profile_id, source, filename, status, summary, created_at, applied_at, undone_at;

-- name: DropParsedImports :exec
DELETE FROM imports WHERE profile_id = $1 AND status = 'parsed';

-- name: LockProfile :one
SELECT id FROM profiles WHERE id = $1 FOR UPDATE;

-- name: ImportInProgress :one
SELECT EXISTS (SELECT 1 FROM imports WHERE profile_id = $1 AND status = 'queued');

-- name: ImportByID :one
SELECT id, profile_id, source, filename, status, summary, created_at, applied_at, undone_at
FROM imports WHERE id = $1 AND profile_id = $2;

-- name: ProfileImports :many
SELECT id, profile_id, source, filename, status, summary, created_at, applied_at, undone_at
FROM imports WHERE profile_id = $1
ORDER BY created_at DESC
LIMIT 20;

-- name: QueueImport :execrows
UPDATE imports SET status = 'queued' WHERE id = $1 AND profile_id = $2 AND status = 'parsed';

-- name: NextQueuedImport :one
SELECT * FROM imports WHERE status = 'queued'
ORDER BY created_at
LIMIT 1
FOR UPDATE SKIP LOCKED;

-- name: FinishImport :exec
UPDATE imports
SET status = sqlc.arg(status), summary = sqlc.arg(summary), payload = NULL,
    applied_at = CASE WHEN sqlc.arg(status) = 'applied' THEN now() END
WHERE id = sqlc.arg(id);

-- name: MarkImportUndone :execrows
UPDATE imports SET status = 'undone', undone_at = now()
WHERE id = $1 AND profile_id = $2 AND status = 'applied' AND applied_at > now() - interval '30 days';

-- name: ExpireImports :exec
DELETE FROM imports WHERE status = 'parsed' AND created_at < now() - interval '24 hours';

-- name: ImportWatchEvent :execrows
INSERT INTO watch_events (profile_id, title_id, episode_id, watched_at, precision, action, import_id)
SELECT $1, $2, sqlc.narg(episode_id), $3, @precision, $4, $5
WHERE NOT EXISTS (
    SELECT 1 FROM watch_events
    WHERE profile_id = $1 AND title_id = $2 AND episode_id IS NOT DISTINCT FROM sqlc.narg(episode_id)
      AND (watched_at = $3 OR @precision = 'none')
);

-- name: ImportListItem :execrows
INSERT INTO list_items (list_id, title_id, season_number, episode_id, rank, notes, listed_at, import_id)
SELECT $1, $2, $3, $4, coalesce(max(rank), 0) + 1, $5, $6, $7
FROM list_items WHERE list_id = $1
ON CONFLICT (list_id, title_id, season_number, episode_id) DO NOTHING;

-- name: ImportPlayback :execrows
INSERT INTO playback (profile_id, title_id, episode_id, progress, paused_at, import_id)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (profile_id, title_id, episode_id) DO NOTHING;

-- name: ImportRating :execrows
INSERT INTO ratings (profile_id, title_id, season_number, episode_id, rating, rated_at, import_id)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (profile_id, title_id, season_number, episode_id) DO NOTHING;

-- name: ImportHiddenItem :execrows
INSERT INTO hidden_items (profile_id, section, title_id, season_number, hidden_at, import_id)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT DO NOTHING;

-- name: UndoImportedWatchEvents :execrows
DELETE FROM watch_events WHERE import_id = $1;

-- name: UndoImportedPlayback :execrows
DELETE FROM playback WHERE import_id = $1;

-- name: UndoImportedListItems :execrows
DELETE FROM list_items WHERE import_id = $1;

-- name: UndoImportedRatings :execrows
DELETE FROM ratings WHERE import_id = $1;

-- name: UndoImportedHiddenItems :execrows
DELETE FROM hidden_items WHERE import_id = $1;
