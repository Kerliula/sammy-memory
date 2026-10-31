// Package testsupport boots one real Postgres (pgvector image) per test
// package, applies migrations once, and hands each test a transaction
// that is rolled back on cleanup. No database mocks, no compose DB.
package testsupport

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Kerliula/sammy-memory/internal/db"
)

// Image is the same pgvector image docker-compose.yml runs.
const Image = "pgvector/pgvector:0.8.7-pg18"

var pool *pgxpool.Pool

// Run is called from a package's TestMain: it starts the container,
// applies goose + River migrations once, runs the tests, then removes
// the container.
func Run(m *testing.M) int {
	ctx := context.Background()

	container, err := postgres.Run(ctx, Image,
		postgres.WithDatabase("sammy"),
		postgres.WithUsername("sammy"),
		postgres.WithPassword("sammy"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start test postgres: %v\n", err)
		return 1
	}
	defer func() { _ = container.Terminate(ctx) }()

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "test postgres connection string: %v\n", err)
		return 1
	}
	pool, err = db.NewPool(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "test postgres pool: %v\n", err)
		return 1
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		fmt.Fprintf(os.Stderr, "test postgres migrate: %v\n", err)
		return 1
	}
	return m.Run()
}

// Pool returns the shared pool for the test package. Prefer Tx for
// anything that writes.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if pool == nil {
		t.Fatal("testsupport: TestMain must call testsupport.Run")
	}
	return pool
}

// Tx begins a transaction that is rolled back when the test finishes,
// so tests stay isolated without truncating tables.
func Tx(t *testing.T) pgx.Tx {
	t.Helper()
	tx, err := Pool(t).Begin(context.Background())
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return tx
}
