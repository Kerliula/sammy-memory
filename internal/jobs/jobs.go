// Package jobs registers Sammy's River job types and workers.
package jobs

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/Kerliula/sammy-memory/internal/db/sqlc"
)

// ExtractArgs asks a worker to turn newly retained messages into facts.
// Retain enqueues it in the same transaction that stores the messages.
type ExtractArgs struct {
	WorkspaceID uuid.UUID   `json:"workspace_id"`
	MessageIDs  []uuid.UUID `json:"message_ids"`
}

// Kind returns the River job kind stored in river_job.kind.
func (ExtractArgs) Kind() string { return "extract" }

// ExtractWorker is a stub until fact extraction lands: it loads the job's
// messages, which proves retain → queue → worker works end to end.
type ExtractWorker struct {
	river.WorkerDefaults[ExtractArgs]
	pool *pgxpool.Pool
}

// Work implements river.Worker.
func (w *ExtractWorker) Work(ctx context.Context, job *river.Job[ExtractArgs]) error {
	messages, err := sqlc.New(w.pool).ListMessagesByIDs(ctx, sqlc.ListMessagesByIDsParams{
		WorkspaceID: job.Args.WorkspaceID,
		Ids:         job.Args.MessageIDs,
	})
	if err != nil {
		return fmt.Errorf("list messages: %w", err)
	}
	if len(messages) == 0 {
		// Retrying can't make the messages appear, so give up for good.
		return river.JobCancel(fmt.Errorf("none of the job's %d messages exist", len(job.Args.MessageIDs)))
	}
	slog.InfoContext(ctx, "extract stub: messages loaded",
		"workspace_id", job.Args.WorkspaceID,
		"messages", len(messages),
	)
	return nil
}

// NewWorkers returns the worker registry with every job type registered.
func NewWorkers(pool *pgxpool.Pool) *river.Workers {
	workers := river.NewWorkers()
	river.AddWorker(workers, &ExtractWorker{pool: pool})
	return workers
}

// NewWorkerClient builds a River client that both enqueues and works jobs.
func NewWorkerClient(pool *pgxpool.Pool) (*river.Client[pgx.Tx], error) {
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Logger: slog.Default(), // River's own default is a text logger on stdout
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 10},
		},
		Workers: NewWorkers(pool),
	})
	if err != nil {
		return nil, fmt.Errorf("create river worker client: %w", err)
	}
	return client, nil
}

// NewInsertClient builds an insert-only River client. It never starts
// workers; it exists so callers can enqueue with Insert / InsertTx.
func NewInsertClient(pool *pgxpool.Pool) (*river.Client[pgx.Tx], error) {
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		return nil, fmt.Errorf("create river insert client: %w", err)
	}
	return client, nil
}
