-- name: TitleByID :one
SELECT * FROM titles WHERE id = $1 AND kind = $2;

-- name: TitleBySlug :one
SELECT * FROM titles WHERE slug = $1 AND kind = $2;

-- name: TitleByTMDB :one
SELECT * FROM titles WHERE tmdb_id = $1 AND kind = $2;

-- name: TitleByExternalID :one
SELECT t.*
FROM titles t
JOIN external_ids e ON e.title_id = t.id
WHERE e.provider = $1 AND e.value = $2 AND t.kind = $3;

-- name: CreateTitle :one
INSERT INTO titles (id, kind, slug, tmdb_id)
SELECT n, sqlc.arg(kind), 't-' || n, sqlc.narg(tmdb_id) FROM nextval('catalogue_id') AS n
ON CONFLICT (kind, tmdb_id) DO UPDATE SET updated_at = titles.updated_at
RETURNING *;

-- name: AddExternalID :exec
INSERT INTO external_ids (title_id, provider, value) VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING;

-- name: SetTitleTMDB :exec
UPDATE titles SET tmdb_id = $2, updated_at = now() WHERE id = $1 AND tmdb_id IS NULL;

-- name: CatalogueTitles :many
SELECT t.id, t.kind, t.slug, t.name, t.year, t.tmdb_id,
    coalesce((SELECT value FROM external_ids WHERE title_id = t.id AND provider = 'imdb' LIMIT 1), '')::text AS imdb,
    coalesce((SELECT value FROM external_ids WHERE title_id = t.id AND provider = 'tvdb' LIMIT 1), '')::text AS tvdb
FROM titles t
WHERE t.id = ANY(@ids::bigint[]);

-- name: EpisodeByID :one
SELECT * FROM episodes WHERE id = $1;

-- name: EpisodeByTMDB :one
SELECT * FROM episodes WHERE tmdb_episode_id = $1;

-- name: EpisodeByNumber :one
SELECT * FROM episodes WHERE show_id = $1 AND season_number = $2 AND episode_number = $3 AND removed_at IS NULL;

-- name: CreateEpisode :one
INSERT INTO episodes (show_id, season_number, episode_number, tmdb_episode_id)
VALUES ($1, $2, $3, $4)
ON CONFLICT (show_id, season_number, episode_number) WHERE removed_at IS NULL
DO UPDATE SET tmdb_episode_id = coalesce(episodes.tmdb_episode_id, excluded.tmdb_episode_id)
RETURNING *;

-- name: CatalogueEpisodes :many
SELECT * FROM episodes WHERE id = ANY(@ids::bigint[]);

-- name: ShowEpisodes :many
SELECT * FROM episodes WHERE show_id = $1 FOR UPDATE;

-- name: RemoveEpisode :exec
UPDATE episodes SET removed_at = now() WHERE id = $1;

-- name: UpdateEpisode :exec
UPDATE episodes
SET season_number = $2, episode_number = $3, tmdb_episode_id = $4, first_aired = $5, removed_at = NULL
WHERE id = $1;

-- name: AddOutlineEpisode :exec
INSERT INTO episodes (show_id, season_number, episode_number, tmdb_episode_id, first_aired)
VALUES ($1, $2, $3, $4, $5);

-- name: MergeEpisodeEvents :exec
UPDATE watch_events SET episode_id = sqlc.arg(into_id) WHERE episode_id = sqlc.arg(from_id);

-- name: DropClashingPlayback :exec
DELETE FROM playback p
WHERE p.episode_id = sqlc.arg(from_id)
  AND EXISTS (SELECT 1 FROM playback q WHERE q.profile_id = p.profile_id AND q.episode_id = sqlc.arg(into_id));

-- name: MergeEpisodePlayback :exec
UPDATE playback SET episode_id = sqlc.arg(into_id) WHERE episode_id = sqlc.arg(from_id);

-- name: DropClashingListItems :exec
DELETE FROM list_items l
WHERE l.episode_id = sqlc.arg(from_id)
  AND EXISTS (SELECT 1 FROM list_items m WHERE m.list_id = l.list_id AND m.episode_id = sqlc.arg(into_id));

-- name: MergeEpisodeListItems :exec
UPDATE list_items SET episode_id = sqlc.arg(into_id) WHERE episode_id = sqlc.arg(from_id);

-- name: DropClashingRatings :exec
DELETE FROM ratings r
WHERE r.episode_id = sqlc.arg(from_id)
  AND EXISTS (SELECT 1 FROM ratings s WHERE s.profile_id = r.profile_id AND s.episode_id = sqlc.arg(into_id));

-- name: MergeEpisodeRatings :exec
UPDATE ratings SET episode_id = sqlc.arg(into_id) WHERE episode_id = sqlc.arg(from_id);

-- name: MergeEpisodeWatching :exec
UPDATE watching SET episode_id = sqlc.arg(into_id) WHERE episode_id = sqlc.arg(from_id);

-- name: DeleteEpisode :exec
DELETE FROM episodes WHERE id = $1;

-- name: OutlineDone :exec
UPDATE titles SET outline_refreshed_at = now(), tmdb_missing_at = sqlc.narg(missing_at) WHERE id = $1;

-- name: DueOutlines :many
SELECT t.id, t.tmdb_id
FROM titles t
WHERE t.kind = 'show' AND t.tmdb_id IS NOT NULL
  AND EXISTS (SELECT 1 FROM watch_events w WHERE w.title_id = t.id)
  AND (t.outline_refreshed_at IS NULL
    OR t.outline_refreshed_at < now() - CASE
      WHEN EXISTS (
        SELECT 1 FROM episodes e
        WHERE e.show_id = t.id AND e.removed_at IS NULL
          AND (e.first_aired IS NULL OR e.first_aired > current_date - 90)
      ) THEN interval '7 days'
      ELSE interval '90 days'
    END)
ORDER BY t.outline_refreshed_at NULLS FIRST
LIMIT $1;
