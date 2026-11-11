-- +goose Up

ALTER TABLE titles DROP CONSTRAINT titles_slug_key;
ALTER TABLE titles ADD UNIQUE (kind, slug);

-- +goose Down

UPDATE titles t SET slug = 't-' || t.id
WHERE t.kind = 'show' AND EXISTS (SELECT 1 FROM titles o WHERE o.kind = 'movie' AND o.slug = t.slug);
ALTER TABLE titles DROP CONSTRAINT titles_kind_slug_key;
ALTER TABLE titles ADD UNIQUE (slug);
