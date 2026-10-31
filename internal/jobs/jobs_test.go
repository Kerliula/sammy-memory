package jobs_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Kerliula/sammy-memory/internal/db/sqlc"
	"github.com/Kerliula/sammy-memory/internal/jobs"
	"github.com/Kerliula/sammy-memory/internal/testsupport"
)

func TestMain(m *testing.M) {
	os.Exit(testsupport.Run(m))
}

// TestInsertTxRollsBackWithWorkspace is the pattern at the heart of
// retain: a row insert and a job enqueue in one transaction must both
// vanish when the transaction rolls back.
func TestInsertTxRollsBackWithWorkspace(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)

	client, err := jobs.NewInsertClient(pool)
	if err != nil {
		t.Fatalf("NewInsertClient: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	queries := sqlc.New(tx)
	workspace, err := queries.InsertWorkspace(ctx, sqlc.InsertWorkspaceParams{
		ID:   uuid.New(),
		Name: "tx-rollback",
	})
	if err != nil {
		t.Fatalf("InsertWorkspace: %v", err)
	}
	if _, err := client.InsertTx(ctx, tx, jobs.NoopArgs{}, nil); err != nil {
		t.Fatalf("InsertTx: %v", err)
	}

	// Inside the transaction both writes are visible.
	if _, err := queries.GetWorkspace(ctx, workspace.ID); err != nil {
		t.Fatalf("GetWorkspace in tx: %v", err)
	}
	var jobsInTx int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind = 'noop'`).Scan(&jobsInTx); err != nil {
		t.Fatalf("count river_job in tx: %v", err)
	}
	if jobsInTx != 1 {
		t.Fatalf("river_job count in tx = %d, want 1", jobsInTx)
	}

	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	// After rollback neither write exists.
	poolQueries := sqlc.New(pool)
	if _, err := poolQueries.GetWorkspace(ctx, workspace.ID); err != pgx.ErrNoRows {
		t.Fatalf("GetWorkspace after rollback error = %v, want pgx.ErrNoRows", err)
	}
	var jobsAfter int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind = 'noop'`).Scan(&jobsAfter); err != nil {
		t.Fatalf("count river_job after rollback: %v", err)
	}
	if jobsAfter != 0 {
		t.Fatalf("river_job count after rollback = %d, want 0", jobsAfter)
	}
}
