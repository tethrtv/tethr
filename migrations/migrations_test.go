package migrations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/tethrtv/tethr/internal/dbtest"
	"github.com/tethrtv/tethr/migrations"
)

func TestDownAndUpAgain(t *testing.T) {
	db := stdlib.OpenDBFromPool(dbtest.New(t))
	defer func() { _ = db.Close() }()

	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := p.DownTo(ctx, 0); err != nil {
		t.Fatalf("down: %v", err)
	}
	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("up again: %v", err)
	}
}

func TestIDsCountFromOne(t *testing.T) {
	pool := dbtest.New(t)
	db := stdlib.OpenDBFromPool(pool)
	defer func() { _ = db.Close() }()

	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := p.DownTo(ctx, 23); err != nil {
		t.Fatalf("down: %v", err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO accounts (email, name) VALUES ('nathan@example.com', 'Nathan');
INSERT INTO households (owner_account_id) SELECT id FROM accounts;
INSERT INTO profiles (household_id, account_id, username, name, kind) SELECT id, owner_account_id, 'nathan', 'Nathan', 'owner' FROM households;

INSERT INTO titles (id, kind, slug) SELECT n, 'show', 't-' || n FROM nextval('catalogue_id') AS n;
INSERT INTO titles (kind, slug, name, year) VALUES ('movie', 'heat-1995', 'Heat', 1995);
INSERT INTO titles (id, kind, slug, merged_into_id) SELECT n, 'movie', 't-' || n, 1000000001 FROM nextval('catalogue_id') AS n;
INSERT INTO episodes (show_id, season_number, episode_number) VALUES (1000000000, 1, 1);
INSERT INTO external_ids (title_id, provider, value) VALUES (1000000001, 'imdb', 'tt0113277');

INSERT INTO watch_events (profile_id, title_id, episode_id, watched_at, action) SELECT id, 1000000000, 1000000003, now(), 'scrobble' FROM profiles;
INSERT INTO watch_events (profile_id, title_id, watched_at, action) SELECT id, 1000000001, now(), 'watch' FROM profiles;
INSERT INTO playback (profile_id, title_id, progress, paused_at) SELECT id, 1000000001, 40, now() FROM profiles;
INSERT INTO watching (profile_id, title_id, episode_id, progress, started_at, expires_at) SELECT id, 1000000000, 1000000003, 10, now(), now() FROM profiles;
INSERT INTO hidden_items (profile_id, section, title_id) SELECT id, 'dropped', 1000000000 FROM profiles;
INSERT INTO ratings (profile_id, title_id, rating) SELECT id, 1000000001, 9 FROM profiles;
INSERT INTO episode_orders (show_id, tmdb_group_id, tmdb_type) VALUES (1000000000, 'dvd', 3);
INSERT INTO profile_show_settings (profile_id, show_id, episode_order_id) SELECT p.id, 1000000000, o.id FROM profiles p, episode_orders o;
INSERT INTO media_servers (account_id, kind, secret_hash) SELECT id, 'jellyfin', '\x01' FROM accounts;
INSERT INTO media_server_users (server_id, external_id, name) SELECT id, 'u1', 'Nathan' FROM media_servers;
INSERT INTO held_plays (server_user_id, title_id, episode_id, action, watched_at) SELECT id, 1000000000, 1000000003, 'watch', now() FROM media_server_users;

INSERT INTO lists (id, profile_id, kind, name, slug) OVERRIDING SYSTEM VALUE SELECT 1, id, 'watchlist', 'Watchlist', 'watchlist' FROM profiles;
INSERT INTO lists (id, profile_id, kind, name, slug) OVERRIDING SYSTEM VALUE SELECT 2, id, 'custom', 'Old', 'old' FROM profiles;
INSERT INTO lists (profile_id, kind, name, slug) SELECT id, 'custom', 'Friday', 'friday' FROM profiles;
INSERT INTO lists (profile_id, kind, name, slug) SELECT id, 'custom', 'Sunday', 'sunday' FROM profiles;
INSERT INTO list_items (list_id, title_id, rank) VALUES (1000000000, 1000000001, 1);
INSERT INTO list_items (list_id, title_id, rank) VALUES (1, 1000000000, 1);
INSERT INTO list_items (list_id, title_id, episode_id, rank) VALUES (1000000001, 1000000000, 1000000003, 1);
UPDATE lists SET cover_item_id = 1000000000000 WHERE id = 1000000000;
INSERT INTO list_follows (profile_id, list_id) SELECT id, 1000000001 FROM profiles;
INSERT INTO list_reports (list_id, reason) VALUES (1, 'spam');
INSERT INTO old_list_slugs (profile_id, slug, list_id) SELECT id, 'fri', 1000000000 FROM profiles;
INSERT INTO up_next_lists (profile_id, list_id) SELECT id, 2 FROM profiles;
`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := p.UpTo(ctx, 24); err != nil {
		t.Fatalf("up: %v", err)
	}

	check := func(query string, want int64) {
		t.Helper()
		var got int64
		if err := pool.QueryRow(ctx, query).Scan(&got); err != nil || got != want {
			t.Errorf("%s = %d, %v, want %d", query, got, err, want)
		}
	}
	check("SELECT id FROM titles WHERE slug = 't-1'", 1)
	check("SELECT id FROM titles WHERE slug = 'heat-1995'", 2)
	check("SELECT merged_into_id FROM titles WHERE slug = 't-3'", 2)
	check("SELECT id FROM episodes WHERE show_id = 1", 4)
	check("SELECT title_id FROM external_ids", 2)
	check("SELECT id FROM watch_events WHERE title_id = 1 AND episode_id = 4", 1)
	check("SELECT id FROM watch_events WHERE title_id = 2 AND episode_id IS NULL", 2)
	check("SELECT id FROM playback WHERE title_id = 2", 1)
	check("SELECT episode_id FROM watching WHERE title_id = 1", 4)
	check("SELECT title_id FROM hidden_items", 1)
	check("SELECT id FROM ratings WHERE title_id = 2", 1)
	check("SELECT show_id FROM episode_orders", 1)
	check("SELECT show_id FROM profile_show_settings", 1)
	check("SELECT episode_id FROM held_plays WHERE title_id = 1", 4)

	check("SELECT id FROM lists WHERE slug = 'friday'", 1)
	check("SELECT id FROM lists WHERE slug = 'sunday'", 2)
	check("SELECT id FROM lists WHERE slug = 'watchlist'", 3)
	check("SELECT id FROM lists WHERE slug = 'old'", 4)
	check("SELECT cover_item_id FROM lists WHERE slug = 'friday'", 1)
	check("SELECT id FROM list_items WHERE list_id = 1 AND title_id = 2", 1)
	check("SELECT id FROM list_items WHERE list_id = 3 AND title_id = 1", 2)
	check("SELECT id FROM list_items WHERE list_id = 2 AND episode_id = 4", 3)
	check("SELECT list_id FROM list_follows", 2)
	check("SELECT list_id FROM list_reports", 3)
	check("SELECT list_id FROM old_list_slugs", 1)
	check("SELECT list_id FROM up_next_lists", 4)

	check("SELECT nextval('catalogue_id')", 5)
	check("SELECT nextval(pg_get_serial_sequence('watch_events', 'id'))", 3)
	check("SELECT nextval(pg_get_serial_sequence('playback', 'id'))", 2)
	check("SELECT nextval(pg_get_serial_sequence('ratings', 'id'))", 2)
	check("SELECT nextval(pg_get_serial_sequence('list_items', 'id'))", 4)
	check("SELECT nextval(pg_get_serial_sequence('lists', 'id'))", 5)
	check("SELECT seqmax FROM pg_sequence WHERE seqrelid = 'catalogue_id'::regclass", 1147483648)
	check("SELECT seqmax FROM pg_sequence WHERE seqrelid = pg_get_serial_sequence('lists', 'id')::regclass", 1147483648)
	check(`SELECT count(*) FROM information_schema.columns WHERE column_name = 'id' AND identity_generation = 'ALWAYS'
		AND table_name IN ('watch_events', 'playback', 'ratings', 'list_items', 'lists')`, 5)
	check(`SELECT count(*) FROM pg_constraint WHERE contype = 'f'
		AND confrelid IN ('titles'::regclass, 'episodes'::regclass, 'lists'::regclass, 'list_items'::regclass)`, 24)

	if _, err := p.DownTo(ctx, 23); err != nil {
		t.Fatalf("down again: %v", err)
	}
	check("SELECT id FROM titles WHERE slug = 't-1000000000'", 1000000000)
	check("SELECT id FROM watch_events WHERE title_id = 1000000000 AND episode_id = 1000000003", 1000000000000)
	check("SELECT id FROM lists WHERE slug = 'friday'", 1000000000)
}
