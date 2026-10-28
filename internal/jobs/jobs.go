// Package jobs registers Sammy's River job types and workers.
package jobs

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

// NoopArgs is a placeholder job that proves the River wiring end to end.
// Real job types (extraction, consolidation) arrive in later phases.
type NoopArgs struct{}

// Kind returns the River job kind stored in river_job.kind.
func (NoopArgs) Kind() string { return "noop" }

// NoopWorker does nothing and succeeds.
type NoopWorker struct {
	river.WorkerDefaults[NoopArgs]
}

// Work implements river.Worker.
func (w *NoopWorker) Work(_ context.Context, _ *river.Job[NoopArgs]) error {
	return nil
}

// NewWorkers returns the worker registry with every job type registered.
func NewWorkers() *river.Workers {
	workers := river.NewWorkers()
	river.AddWorker(workers, &NoopWorker{})
	return workers
}

// NewWorkerClient builds a River client that both enqueues and works jobs.
func NewWorkerClient(pool *pgxpool.Pool) (*river.Client[pgx.Tx], error) {
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 10},
		},
		Workers: NewWorkers(),
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
