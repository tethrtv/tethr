-- +goose Up

CREATE TABLE seasons (
    id bigint PRIMARY KEY DEFAULT nextval('catalogue_id'),
    show_id bigint NOT NULL REFERENCES titles (id),
    season_number integer NOT NULL,
    UNIQUE (show_id, season_number)
);

INSERT INTO seasons (show_id, season_number)
SELECT show_id, season_number FROM episodes
UNION SELECT title_id, season_number FROM ratings WHERE season_number IS NOT NULL
UNION SELECT title_id, season_number FROM hidden_items WHERE season_number IS NOT NULL
UNION SELECT title_id, season_number FROM list_items WHERE season_number IS NOT NULL
ORDER BY 1, 2;

ALTER TABLE ratings ADD COLUMN season_id bigint REFERENCES seasons (id);
UPDATE ratings r SET season_id = s.id FROM seasons s WHERE s.show_id = r.title_id AND s.season_number = r.season_number;
ALTER TABLE ratings DROP COLUMN season_number;
ALTER TABLE ratings ADD UNIQUE NULLS NOT DISTINCT (profile_id, title_id, season_id, episode_id);

ALTER TABLE hidden_items ADD COLUMN season_id bigint REFERENCES seasons (id);
UPDATE hidden_items h SET season_id = s.id FROM seasons s WHERE s.show_id = h.title_id AND s.season_number = h.season_number;
ALTER TABLE hidden_items DROP COLUMN season_number;
ALTER TABLE hidden_items ADD UNIQUE NULLS NOT DISTINCT (profile_id, section, title_id, season_id);

ALTER TABLE list_items ADD COLUMN season_id bigint REFERENCES seasons (id);
UPDATE list_items i SET season_id = s.id FROM seasons s WHERE s.show_id = i.title_id AND s.season_number = i.season_number;
ALTER TABLE list_items DROP COLUMN season_number;
ALTER TABLE list_items ADD UNIQUE NULLS NOT DISTINCT (list_id, title_id, season_id, episode_id);

-- +goose Down

ALTER TABLE list_items ADD COLUMN season_number integer;
UPDATE list_items i SET season_number = s.season_number FROM seasons s WHERE s.id = i.season_id;
ALTER TABLE list_items DROP COLUMN season_id;
ALTER TABLE list_items ADD UNIQUE NULLS NOT DISTINCT (list_id, title_id, season_number, episode_id);

ALTER TABLE hidden_items ADD COLUMN season_number integer;
UPDATE hidden_items h SET season_number = s.season_number FROM seasons s WHERE s.id = h.season_id;
ALTER TABLE hidden_items DROP COLUMN season_id;
ALTER TABLE hidden_items ADD UNIQUE NULLS NOT DISTINCT (profile_id, section, title_id, season_number);

ALTER TABLE ratings ADD COLUMN season_number integer;
UPDATE ratings r SET season_number = s.season_number FROM seasons s WHERE s.id = r.season_id;
ALTER TABLE ratings DROP COLUMN season_id;
ALTER TABLE ratings ADD UNIQUE NULLS NOT DISTINCT (profile_id, title_id, season_number, episode_id);

DROP TABLE seasons;
