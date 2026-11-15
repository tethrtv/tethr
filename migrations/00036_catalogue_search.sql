-- +goose Up

CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS unaccent;

CREATE FUNCTION fold_name(text) RETURNS text
LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
RETURN regexp_replace(public.unaccent('public.unaccent'::regdictionary, regexp_replace(replace(lower($1 COLLATE pg_c_utf8), '&', ' and '), '^\s*(the|an|a)\s+(?=\S)', '')) COLLATE pg_c_utf8, '[^[:alnum:]]', '', 'g');

ALTER TABLE titles
    ADD COLUMN search_name text GENERATED ALWAYS AS (fold_name(name)) STORED,
    ADD COLUMN search_original text GENERATED ALWAYS AS (fold_name(original_name)) STORED;

CREATE INDEX titles_search ON titles USING gin (search_name gin_trgm_ops, search_original gin_trgm_ops) WHERE merged_into_id IS NULL;

-- +goose Down

DROP INDEX titles_search;
ALTER TABLE titles DROP COLUMN search_original, DROP COLUMN search_name;
DROP FUNCTION fold_name(text);
DROP EXTENSION IF EXISTS unaccent;
DROP EXTENSION IF EXISTS pg_trgm;
