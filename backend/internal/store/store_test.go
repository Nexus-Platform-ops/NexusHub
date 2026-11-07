package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestMigrate verifies the file-based migration runner applies SQL exactly
// once and surfaces syntax errors, using the superuser URL.
func TestMigrate(t *testing.T) {
	ctx := context.Background()
	url := envOrStr("TEST_SUPERUSER_URL",
		"postgres://postgres:postgres@127.0.0.1:5432/nexus?sslmode=disable&default_query_exec_mode=simple_protocol")
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("superuser db unavailable: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, `DROP TABLE IF EXISTS public.mig_test_a`)
		_, _ = pool.Exec(ctx, `DELETE FROM schema_migrations WHERE filename IN ('001_a.sql','002_b.sql')`)
	}()

	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("001_a.sql", "CREATE TABLE IF NOT EXISTS public.mig_test_a (id int);")
	write("002_bad.sql", "THIS IS NOT VALID SQL;")

	if err := Migrate(ctx, pool, dir); err == nil {
		t.Fatal("expected error from invalid SQL")
	}
	if err := os.Remove(filepath.Join(dir, "002_bad.sql")); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, pool, dir); err != nil {
		t.Fatalf("apply: %v", err)
	}
	var exists bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name='mig_test_a')`).Scan(&exists); err != nil || !exists {
		t.Fatalf("table missing (err=%v exists=%v)", err, exists)
	}
	// Re-run must be a no-op.
	if err := Migrate(ctx, pool, dir); err != nil {
		t.Fatalf("re-run: %v", err)
	}
}

// TestPingTimeout ensures New fails fast on an unreachable Redis.
func TestNewFailsOnBadRedis(t *testing.T) {
	socialURL := envOrStr("TEST_SOCIAL_URL",
		"postgres://nexus_social:dev_social@127.0.0.1:5432/nexus?sslmode=disable")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := New(ctx, socialURL, socialURL, "127.0.0.1:1"); err == nil {
		t.Fatal("expected redis connection failure")
	}
}

func envOrStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fmt.Sprint(def)
}

// TestNewSuccessAndClose exercises the real two-pool + Redis startup path.
func TestNewSuccessAndClose(t *testing.T) {
	socialURL := envOrStr("TEST_SOCIAL_URL",
		"postgres://nexus_social:dev_social@127.0.0.1:5432/nexus?sslmode=disable")
	ctx := context.Background()
	s, err := New(ctx, socialURL, socialURL,
		envOrStr("TEST_REDIS_ADDR", "127.0.0.1:6379"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if s.Social == nil || s.Dating == nil || s.RDB == nil {
		t.Fatal("nil components in Store")
	}
	err = s.DatingConn(ctx, func(q Querier) error {
		var one int
		return q.QueryRow(ctx, `SELECT 1`).Scan(&one)
	})
	if err != nil {
		t.Fatalf("DatingConn: %v", err)
	}
	s.Close()
}
