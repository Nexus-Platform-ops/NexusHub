// Package store owns the two database pools and the Redis client.
//
// Isolation model (see docs/ARCHITECTURE.md): the social connection uses the
// `nexus_social` role which has zero privileges on the `dating` schema, while
// the dating connection uses `nexus_dating`, which only sees a minimal
// friend-id view of the social schema. RLS on dating tables additionally
// requires the per-session flag app.dating_access=service, which the API sets
// only after a JWT-authenticated request.
package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Store bundles the isolated pools and Redis.
type Store struct {
	Social *pgxpool.Pool
	Dating *pgxpool.Pool
	RDB    *redis.Client
}

func poolConfig(url string) (*pgxpool.Config, error) {
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	cfg, err := pgxpool.ParseConfig(url + sep + "default_query_exec_mode=simple_protocol")
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	cfg.ConnConfig.RuntimeParams["application_name"] = "nexus-api"
	return cfg, nil
}

// OpenPool opens a pool with the same simple-protocol configuration the API
// pools use. Migration files contain multiple statements, which the extended
// query protocol rejects.
func OpenPool(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := poolConfig(url)
	if err != nil {
		return nil, err
	}
	return pgxpool.NewWithConfig(ctx, cfg)
}

// New opens both pools and pings them plus Redis.
func New(ctx context.Context, socialURL, datingURL, redisAddr string) (*Store, error) {
	socialCfg, err := poolConfig(socialURL)
	if err != nil {
		return nil, err
	}
	datingCfg, err := poolConfig(datingURL)
	if err != nil {
		return nil, err
	}

	social, err := pgxpool.NewWithConfig(ctx, socialCfg)
	if err != nil {
		return nil, fmt.Errorf("connect social pool: %w", err)
	}
	dating, err := pgxpool.NewWithConfig(ctx, datingCfg)
	if err != nil {
		social.Close()
		return nil, fmt.Errorf("connect dating pool: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := social.Ping(pingCtx); err != nil {
		social.Close()
		dating.Close()
		return nil, fmt.Errorf("ping social db: %w", err)
	}
	if err := dating.Ping(pingCtx); err != nil {
		social.Close()
		dating.Close()
		return nil, fmt.Errorf("ping dating db: %w", err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		social.Close()
		dating.Close()
		_ = rdb.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return &Store{Social: social, Dating: dating, RDB: rdb}, nil
}

// Close releases every pooled connection.
func (s *Store) Close() {
	s.Social.Close()
	s.Dating.Close()
	_ = s.RDB.Close()
}

// DatingConn runs fn on one dating-pool connection inside a transaction that
// carries the RLS service flag. The flag is transaction-local (is_local),
// so a pooled connection is always clean when handed to the next borrower.
func (s *Store) DatingConn(ctx context.Context, fn func(q Querier) error) error {
	conn, err := s.Dating.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire dating conn: %w", err)
	}
	defer conn.Release()
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin dating tx: %w", err)
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('app.dating_access','service', true)"); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("set dating access flag: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

// Querier is the subset of pgxconn used by dating handlers.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Migrate applies every .sql file in dir, in lexical order, exactly once per
// file by recording applied filenames in schema_migrations.
func Migrate(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	if _, err := pool.Exec(ctx,
		`CREATE TABLE IF NOT EXISTS schema_migrations (
		   filename text PRIMARY KEY,
		   applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read migration dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE filename=$1)`, name,
		).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		sqlBytes, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO schema_migrations (filename) VALUES ($1)`, name); err != nil {
			return fmt.Errorf("record %s: %w", name, err)
		}
	}
	return nil
}
