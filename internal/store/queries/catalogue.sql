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
SELECT * FROM episodes WHERE show_id = $1 AND season_number = $2 AND episode_number = $3;

-- name: CreateEpisode :one
INSERT INTO episodes (show_id, season_number, episode_number, tmdb_episode_id)
VALUES ($1, $2, $3, $4)
ON CONFLICT (show_id, season_number, episode_number)
DO UPDATE SET tmdb_episode_id = coalesce(episodes.tmdb_episode_id, excluded.tmdb_episode_id)
RETURNING *;

-- name: CatalogueEpisodes :many
SELECT * FROM episodes WHERE id = ANY(@ids::bigint[]);

-- name: ShowEpisodes :many
SELECT * FROM episodes WHERE show_id = $1 AND (sqlc.narg(season)::integer IS NULL OR season_number = sqlc.narg(season));
