// Package testutil starts real Postgres and Redis instances for integration
// tests using testcontainers-go. Tests that use it are skipped with -short.
package testutil

import (
	"context"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"

	"pantheon-spins/server/internal/db"
	"pantheon-spins/server/internal/dbmigrate"
	"pantheon-spins/server/internal/wallet"
)

// Env holds live connections shared by a test package.
type Env struct {
	Pool  *pgxpool.Pool
	Redis *redis.Client
}

// Main starts the containers, runs the package's tests and tears down.
// Use it from TestMain. With -short it runs the tests without containers;
// integration tests must call SkipIfShort.
func Main(m *testing.M, env **Env) {
	os.Exit(run(m, env))
}

func run(m *testing.M, env **Env) int {
	flag.Parse()
	if testing.Short() {
		return m.Run()
	}
	ctx := context.Background()
	pg, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("pantheon"),
		tcpostgres.WithUsername("pantheon"),
		tcpostgres.WithPassword("pantheon"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "testutil: start postgres:", err)
		return 1
	}
	defer func() { _ = pg.Terminate(ctx) }()

	rc, err := tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		fmt.Fprintln(os.Stderr, "testutil: start redis:", err)
		return 1
	}
	defer func() { _ = rc.Terminate(ctx) }()

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintln(os.Stderr, "testutil:", err)
		return 1
	}
	if err := dbmigrate.Up(dsn); err != nil {
		fmt.Fprintln(os.Stderr, "testutil:", err)
		return 1
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "testutil:", err)
		return 1
	}
	cfg.MaxConns = 32
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "testutil:", err)
		return 1
	}
	defer pool.Close()

	redisURL, err := rc.ConnectionString(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "testutil:", err)
		return 1
	}
	ropts, err := redis.ParseURL(redisURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "testutil:", err)
		return 1
	}
	rdb := redis.NewClient(ropts)
	defer rdb.Close()

	*env = &Env{Pool: pool, Redis: rdb}
	return m.Run()
}

// SkipIfShort skips integration tests under -short.
func SkipIfShort(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: needs Docker (run without -short)")
	}
}

// NewUser creates a user whose wallet starts at balance and returns its ID.
func NewUser(t *testing.T, pool *pgxpool.Pool, balance int64) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	name := "u" + uuid.NewString()[:12]
	hash := "unused"
	var id uuid.UUID
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		q := db.New(tx)
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Email: name + "@example.test", Username: name, PasswordHash: &hash, Avatar: "raven",
		})
		if err != nil {
			return err
		}
		id = u.ID
		return wallet.Open(ctx, q, u.ID, balance)
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return id
}
