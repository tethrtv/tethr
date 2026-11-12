-- +goose Up

ALTER TABLE external_ids DROP CONSTRAINT external_ids_provider_check;
ALTER TABLE external_ids ADD CONSTRAINT external_ids_provider_check CHECK (provider IN (
    'imdb', 'tvdb', 'tvmaze', 'letterboxd', 'plex', 'trakt_film', 'trakt_show', 'trakt_slug', 'eidr', 'isan', 'mal', 'anidb',
    'anilist', 'rottentomatoes', 'metacritic', 'appletv', 'netflix', 'primevideo', 'disneyplus'
));

-- +goose Down

DELETE FROM external_ids WHERE provider = 'trakt_show';
ALTER TABLE external_ids DROP CONSTRAINT external_ids_provider_check;
ALTER TABLE external_ids ADD CONSTRAINT external_ids_provider_check CHECK (provider IN (
    'imdb', 'tvdb', 'tvmaze', 'letterboxd', 'plex', 'trakt_film', 'trakt_slug', 'eidr', 'isan', 'mal', 'anidb', 'anilist',
    'rottentomatoes', 'metacritic', 'appletv', 'netflix', 'primevideo', 'disneyplus'
));
