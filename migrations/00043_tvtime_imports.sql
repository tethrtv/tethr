-- +goose Up
ALTER TABLE imports DROP CONSTRAINT imports_source_check;
ALTER TABLE imports ADD CONSTRAINT imports_source_check CHECK (source IN ('trakt', 'simkl', 'letterboxd', 'mdblist', 'tvtime'));

-- +goose Down
DELETE FROM imports WHERE source = 'tvtime';
ALTER TABLE imports DROP CONSTRAINT imports_source_check;
ALTER TABLE imports ADD CONSTRAINT imports_source_check CHECK (source IN ('trakt', 'simkl', 'letterboxd', 'mdblist'));
