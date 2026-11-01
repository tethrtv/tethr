package dbtest

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/migrations"
)

const defaultURL = "postgres://tethr:tethr@localhost:5432/tethr?sslmode=disable"

func New(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		base = defaultURL
	}

	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatalf("connect to Postgres (is docker compose up?): %v", err)
	}
	defer func() { _ = admin.Close(ctx) }()

	template := "tethr_template_" + migrationsHash(t)
	ensureTemplate(t, admin, base, template)

	name := "tethr_test_" + randomSuffix(t)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name+" TEMPLATE "+template); err != nil {
		t.Fatalf("create test database: %v", err)
	}

	pool, err := pgxpool.New(ctx, withDatabase(t, base, name))
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		conn, err := pgx.Connect(context.Background(), base)
		if err != nil {
			t.Errorf("drop test database: %v", err)
			return
		}
		defer func() { _ = conn.Close(context.Background()) }()
		if _, err := conn.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)"); err != nil {
			t.Errorf("drop test database: %v", err)
		}
	})
	return pool
}

func ensureTemplate(t *testing.T, admin *pgx.Conn, base, template string) {
	t.Helper()
	ctx := context.Background()

	// Test binaries for different packages run in parallel, so only one may build the template.
	if _, err := admin.Exec(ctx, "SELECT pg_advisory_lock(7710)"); err != nil {
		t.Fatalf("lock template: %v", err)
	}
	defer func() { _, _ = admin.Exec(ctx, "SELECT pg_advisory_unlock(7710)") }()

	var exists bool
	if err := admin.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", template).Scan(&exists); err != nil {
		t.Fatalf("check template: %v", err)
	}
	if exists {
		return
	}

	building := template + "_building"
	if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+building+" WITH (FORCE)"); err != nil {
		t.Fatalf("clear template: %v", err)
	}
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+building); err != nil {
		t.Fatalf("create template: %v", err)
	}
	if _, err := migrations.Up(ctx, withDatabase(t, base, building)); err != nil {
		t.Fatalf("migrate template: %v", err)
	}
	if _, err := admin.Exec(ctx, "ALTER DATABASE "+building+" RENAME TO "+template); err != nil {
		t.Fatalf("finish template: %v", err)
	}
}

func migrationsHash(t *testing.T) string {
	t.Helper()
	h := sha256.New()
	err := fs.WalkDir(migrations.FS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(migrations.FS, path)
		if err != nil {
			return err
		}
		h.Write([]byte(path))
		h.Write(b)
		return nil
	})
	if err != nil {
		t.Fatalf("hash migrations: %v", err)
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

func withDatabase(t *testing.T, base, name string) string {
	t.Helper()
	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse database url: %v", err)
	}
	u.Path = "/" + name
	return u.String()
}

func randomSuffix(t *testing.T) string {
	t.Helper()
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}
