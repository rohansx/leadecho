//go:build integration

package integration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/rs/zerolog"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"leadecho/internal/database"
)

type env struct {
	t         *testing.T
	ctx       context.Context
	cancel    context.CancelFunc
	pgPool    *pgxpool.Pool
	redis     *goredis.Client
	pgConnStr string
}

func newTestEnv(t *testing.T) *env {
	t.Helper()

	if os.Getenv("SKIP_INTEGRATION") == "1" {
		t.Skip("SKIP_INTEGRATION=1")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)

	pgContainer, err := tcpostgres.Run(ctx,
		"pgvector/pgvector:pg16",
		tcpostgres.WithDatabase("leadecho_test"),
		tcpostgres.WithUsername("leadecho"),
		tcpostgres.WithPassword("leadecho"),
	)
	if err != nil {
		cancel()
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() {
		_ = pgContainer.Terminate(context.Background())
	})

	pgConnStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		cancel()
		t.Fatalf("postgres connection string: %v", err)
	}

	if err := waitForPostgres(pgConnStr); err != nil {
		cancel()
		t.Fatalf("postgres ready: %v", err)
	}

	if err := runMigrations(pgConnStr); err != nil {
		cancel()
		t.Fatalf("run migrations: %v", err)
	}

	pgPool, err := database.NewPostgresPool(ctx, pgConnStr)
	if err != nil {
		cancel()
		t.Fatalf("pg pool: %v", err)
	}
	t.Cleanup(pgPool.Close)

	redisContainer, err := tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		cancel()
		t.Fatalf("start redis: %v", err)
	}
	t.Cleanup(func() {
		_ = redisContainer.Terminate(context.Background())
	})

	redisURL, err := redisContainer.ConnectionString(ctx)
	if err != nil {
		cancel()
		t.Fatalf("redis connection string: %v", err)
	}

	redisOpts, err := goredis.ParseURL(redisURL)
	if err != nil {
		cancel()
		t.Fatalf("parse redis url: %v", err)
	}
	redisClient := goredis.NewClient(redisOpts)
	t.Cleanup(func() { _ = redisClient.Close() })

	if err := redisClient.Ping(ctx).Err(); err != nil {
		cancel()
		t.Fatalf("redis ping: %v", err)
	}

	return &env{
		t:         t,
		ctx:       ctx,
		cancel:    cancel,
		pgPool:    pgPool,
		redis:     redisClient,
		pgConnStr: pgConnStr,
	}
}

func runMigrations(connStr string) error {
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return fmt.Errorf("resolve migrations path")
	}
	migrationsDir := filepath.Join(filepath.Dir(file), "..", "..", "migrations")

	return goose.Up(db, migrationsDir)
}

func waitForPostgres(connStr string) error {
	var lastErr error
	for range 40 {
		db, err := sql.Open("pgx", connStr)
		if err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if err := db.Ping(); err != nil {
			lastErr = err
			_ = db.Close()
			time.Sleep(500 * time.Millisecond)
			continue
		}
		_ = db.Close()
		return nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("postgres not ready")
	}
	return lastErr
}

func (e *env) logger() zerolog.Logger {
	return zerolog.New(os.Stderr).With().Timestamp().Logger()
}

func waitFor(t *testing.T, ctx context.Context, timeout time.Duration, desc string, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("timeout waiting for %s: %v", desc, ctx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
	t.Fatalf("timeout waiting for %s after %s", desc, timeout)
}

func countRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, query string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := pool.QueryRow(ctx, query, args...).Scan(&n); err != nil {
		t.Fatalf("count query: %v", err)
	}
	return n
}
