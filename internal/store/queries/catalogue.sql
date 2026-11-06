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
SELECT t.id, t.kind, t.slug, t.name, t.original_name, t.year, t.runtime_minutes, t.tmdb_id, t.wikidata_id,
    coalesce((SELECT min(value) FROM external_ids WHERE title_id = t.id AND provider = 'imdb'), '')::text AS imdb,
    coalesce((SELECT min(value) FROM external_ids WHERE title_id = t.id AND provider = 'tvdb'), '')::text AS tvdb
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

-- name: MergeEpisodeHeldPlays :exec
UPDATE held_plays SET episode_id = sqlc.arg(into_id) WHERE episode_id = sqlc.arg(from_id);

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

-- name: DueWikidata :many
SELECT t.id, t.kind, t.tmdb_id,
    coalesce((SELECT min(value) FROM external_ids WHERE title_id = t.id AND provider = 'imdb'), '')::text AS imdb,
    coalesce((SELECT min(value) FROM external_ids WHERE title_id = t.id AND provider = 'tvdb'), '')::text AS tvdb
FROM titles t
WHERE t.merged_into_id IS NULL
  AND (t.wikidata_checked_at IS NULL
   OR t.wikidata_checked_at < now() - CASE WHEN t.wikidata_id IS NULL THEN interval '7 days' ELSE interval '90 days' END)
ORDER BY t.wikidata_checked_at NULLS FIRST, t.id
LIMIT $1;

-- name: SaveWikidata :exec
UPDATE titles
SET wikidata_id = $2, name = $3, original_name = $4, year = $5, runtime_minutes = $6, wikidata_checked_at = now(), updated_at = now()
WHERE id = $1;

-- name: NameSlug :execrows
UPDATE titles t SET slug = sqlc.arg(slug), updated_at = now()
WHERE t.id = sqlc.arg(id) AND t.slug = 't-' || t.id AND NOT EXISTS (SELECT 1 FROM titles o WHERE o.slug = sqlc.arg(slug));

-- name: WikidataMissing :exec
UPDATE titles SET wikidata_checked_at = now() WHERE id = $1;

-- name: AddMissingExternalID :exec
INSERT INTO external_ids (title_id, provider, value)
SELECT sqlc.arg(title_id)::bigint, sqlc.arg(provider)::text, sqlc.arg(value)::text
WHERE NOT EXISTS (SELECT 1 FROM external_ids WHERE title_id = sqlc.arg(title_id) AND provider = sqlc.arg(provider))
ON CONFLICT DO NOTHING;

-- name: TitleSharingWikidata :one
SELECT * FROM titles
WHERE kind = $1 AND wikidata_id = $2 AND id <> $3 AND merged_into_id IS NULL
ORDER BY id
LIMIT 1;

-- name: MarkMergedActivity :exec
INSERT INTO activity_marks (profile_id, area, at)
SELECT p.profile_id, a.area, now()
FROM (
    SELECT w.profile_id FROM watch_events w WHERE w.title_id = sqlc.arg(from_id)::bigint
    UNION SELECT b.profile_id FROM playback b WHERE b.title_id = sqlc.arg(from_id)::bigint
    UNION SELECT r.profile_id FROM ratings r WHERE r.title_id = sqlc.arg(from_id)::bigint
    UNION SELECT h.profile_id FROM hidden_items h WHERE h.title_id = sqlc.arg(from_id)::bigint
    UNION SELECT l.profile_id FROM list_items i JOIN lists l ON l.id = i.list_id WHERE i.title_id = sqlc.arg(from_id)::bigint
) p
CROSS JOIN unnest(sqlc.arg(areas)::text[]) AS a (area)
ON CONFLICT (profile_id, area) DO UPDATE SET at = excluded.at;

-- name: MergeTitleEvents :exec
UPDATE watch_events SET title_id = sqlc.arg(into_id) WHERE title_id = sqlc.arg(from_id);

-- name: DropClashingTitlePlayback :exec
DELETE FROM playback p
WHERE p.title_id = sqlc.arg(from_id)
  AND EXISTS (
    SELECT 1 FROM playback q
    WHERE q.profile_id = p.profile_id AND q.title_id = sqlc.arg(into_id) AND q.episode_id IS NOT DISTINCT FROM p.episode_id);

-- name: MergeTitlePlayback :exec
UPDATE playback SET title_id = sqlc.arg(into_id) WHERE title_id = sqlc.arg(from_id);

-- name: MergeTitleWatching :exec
UPDATE watching SET title_id = sqlc.arg(into_id) WHERE title_id = sqlc.arg(from_id);

-- name: DropClashingTitleListItems :exec
DELETE FROM list_items l
WHERE l.title_id = sqlc.arg(from_id)
  AND EXISTS (
    SELECT 1 FROM list_items m
    WHERE m.list_id = l.list_id AND m.title_id = sqlc.arg(into_id)
      AND m.season_number IS NOT DISTINCT FROM l.season_number AND m.episode_id IS NOT DISTINCT FROM l.episode_id);

-- name: MergeTitleListItems :exec
UPDATE list_items SET title_id = sqlc.arg(into_id) WHERE title_id = sqlc.arg(from_id);

-- name: DropClashingTitleRatings :exec
DELETE FROM ratings r
WHERE r.title_id = sqlc.arg(from_id)
  AND EXISTS (
    SELECT 1 FROM ratings s
    WHERE s.profile_id = r.profile_id AND s.title_id = sqlc.arg(into_id)
      AND s.season_number IS NOT DISTINCT FROM r.season_number AND s.episode_id IS NOT DISTINCT FROM r.episode_id);

-- name: MergeTitleRatings :exec
UPDATE ratings SET title_id = sqlc.arg(into_id) WHERE title_id = sqlc.arg(from_id);

-- name: DropClashingTitleHidden :exec
DELETE FROM hidden_items h
WHERE h.title_id = sqlc.arg(from_id)
  AND EXISTS (
    SELECT 1 FROM hidden_items g
    WHERE g.profile_id = h.profile_id AND g.section = h.section AND g.title_id = sqlc.arg(into_id)
      AND g.season_number IS NOT DISTINCT FROM h.season_number);

-- name: MergeTitleHidden :exec
UPDATE hidden_items SET title_id = sqlc.arg(into_id) WHERE title_id = sqlc.arg(from_id);

-- name: MergeTitleHeldPlays :exec
UPDATE held_plays SET title_id = sqlc.arg(into_id) WHERE title_id = sqlc.arg(from_id);

-- name: MergeTitleExternalIDs :exec
UPDATE external_ids SET title_id = sqlc.arg(into_id) WHERE title_id = sqlc.arg(from_id);

-- name: MoveEpisode :exec
UPDATE episodes SET show_id = sqlc.arg(into_id) WHERE id = sqlc.arg(id);

-- name: MarkTitleMerged :exec
UPDATE titles SET merged_into_id = sqlc.arg(into_id), updated_at = now()
WHERE id = sqlc.arg(from_id) OR merged_into_id = sqlc.arg(from_id);

-- name: SearchLibrary :many
SELECT t.id
FROM titles t
WHERE t.merged_into_id IS NULL
  AND (t.name ILIKE '%' || sqlc.arg(words)::text || '%' OR t.original_name ILIKE '%' || sqlc.arg(words) || '%')
  AND (sqlc.narg(kind)::text IS NULL OR t.kind = sqlc.narg(kind))
  AND t.id IN (
    SELECT w.title_id FROM watch_events w WHERE w.profile_id = sqlc.arg(profile_id)::bigint
    UNION SELECT r.title_id FROM ratings r WHERE r.profile_id = sqlc.arg(profile_id)::bigint
    UNION SELECT p.title_id FROM playback p WHERE p.profile_id = sqlc.arg(profile_id)::bigint
    UNION SELECT i.title_id FROM list_items i JOIN lists l ON l.id = i.list_id WHERE l.profile_id = sqlc.arg(profile_id)::bigint
  )
ORDER BY t.name ILIKE sqlc.arg(words) || '%' DESC, t.name
LIMIT 20;
