package jobs_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/Kerliula/sammy-memory/internal/db/sqlc"
	"github.com/Kerliula/sammy-memory/internal/jobs"
	"github.com/Kerliula/sammy-memory/internal/retain"
	"github.com/Kerliula/sammy-memory/internal/testsupport"
)

func TestMain(m *testing.M) {
	os.Exit(testsupport.Run(m))
}

// TestRetainedMessagesReachExtractWorker runs the whole pipeline with a
// live worker: retain commits messages and a job, the worker picks the job
// up and finds the messages. These rows are committed, not rolled back,
// because the worker reads them from its own connections.
func TestRetainedMessagesReachExtractWorker(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	client, events := startWorker(t)

	workspace, err := sqlc.New(pool).InsertWorkspace(ctx, sqlc.InsertWorkspaceParams{ID: uuid.New(), Name: t.Name()})
	if err != nil {
		t.Fatalf("InsertWorkspace: %v", err)
	}
	if _, err := retain.NewService(pool, client).Retain(ctx, retain.Request{
		WorkspaceID: workspace.ID,
		SessionID:   uuid.New(),
		Messages:    []retain.Message{{ID: uuid.New(), Participant: "ana", Content: "I'm vegetarian."}},
	}); err != nil {
		t.Fatalf("Retain: %v", err)
	}

	if got := waitForJob(t, events, workspace.ID); got != river.EventKindJobCompleted {
		t.Fatalf("extract job ended with %s, want %s", got, river.EventKindJobCompleted)
	}
}

// TestExtractCancelsMissingMessages checks that a job whose messages don't
// exist gives up instead of retrying forever.
func TestExtractCancelsMissingMessages(t *testing.T) {
	client, events := startWorker(t)
	workspaceID := uuid.New()

	args := jobs.ExtractArgs{WorkspaceID: workspaceID, MessageIDs: []uuid.UUID{uuid.New()}}
	if _, err := client.Insert(context.Background(), args, nil); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	if got := waitForJob(t, events, workspaceID); got != river.EventKindJobCancelled {
		t.Fatalf("extract job ended with %s, want %s", got, river.EventKindJobCancelled)
	}
}

func startWorker(t *testing.T) (*river.Client[pgx.Tx], <-chan *river.Event) {
	t.Helper()
	client, err := jobs.NewWorkerClient(testsupport.Pool(t))
	if err != nil {
		t.Fatalf("NewWorkerClient: %v", err)
	}
	events, cancel := client.Subscribe(river.EventKindJobCompleted, river.EventKindJobCancelled, river.EventKindJobFailed)
	t.Cleanup(cancel)
	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("start worker: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_ = client.Stop(stopCtx)
	})
	return client, events
}

// waitForJob returns how the extract job for workspaceID ended.
func waitForJob(t *testing.T, events <-chan *river.Event, workspaceID uuid.UUID) river.EventKind {
	t.Helper()
	timeout := time.After(15 * time.Second)
	for {
		select {
		case event := <-events:
			var args jobs.ExtractArgs
			if err := json.Unmarshal(event.Job.EncodedArgs, &args); err != nil || args.WorkspaceID != workspaceID {
				continue
			}
			return event.Kind
		case <-timeout:
			t.Fatalf("extract job for workspace %s didn't finish within 15s", workspaceID)
		}
	}
}
