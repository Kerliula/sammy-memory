package retain_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/Kerliula/sammy-memory/internal/db/sqlc"
	"github.com/Kerliula/sammy-memory/internal/jobs"
	"github.com/Kerliula/sammy-memory/internal/retain"
	"github.com/Kerliula/sammy-memory/internal/testsupport"
)

func TestMain(m *testing.M) {
	os.Exit(testsupport.Run(m))
}

// failingInserter stands in for River when the enqueue fails, e.g. because
// the database dropped mid-transaction.
type failingInserter struct{}

func (failingInserter) InsertTx(context.Context, pgx.Tx, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	return nil, errors.New("enqueue failed")
}

func TestRetainStoresMessagesAndJob(t *testing.T) {
	ctx := context.Background()
	tx := testsupport.Tx(t)
	workspaceID := insertWorkspace(t, tx)
	// Nanoseconds, like time.Now(); Postgres keeps only microseconds.
	sentAt := time.Date(2026, 10, 30, 9, 0, 0, 123456789, time.UTC)
	first, second := uuid.New(), uuid.New()

	stored, err := retain.NewService(tx, insertClient(t)).Retain(ctx, retain.Request{
		WorkspaceID: workspaceID,
		SessionID:   uuid.New(),
		Messages: []retain.Message{
			{ID: first, Participant: "ana", Content: "I'm vegetarian.", SentAt: &sentAt},
			{ID: second, Participant: "assistant", Content: "Noted."},
		},
	})
	if err != nil {
		t.Fatalf("Retain: %v", err)
	}

	if len(stored) != 2 || stored[0].ID != first || stored[1].ID != second {
		t.Fatalf("Retain = %+v, want messages %s and %s in order", stored, first, second)
	}
	if stored[0].Participant != "ana" || !stored[0].SentAt.Equal(sentAt.Round(time.Microsecond)) {
		t.Fatalf("first message = %+v, want participant ana sent at %v", stored[0], sentAt)
	}
	if stored[1].Participant != "assistant" || !stored[1].SentAt.Equal(stored[1].CreatedAt) {
		t.Fatalf("second message = %+v, want sent_at defaulted to created_at", stored[1])
	}
	want := [][]uuid.UUID{{first, second}}
	if got := extractJobs(t, tx, workspaceID); !slices.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("extract jobs = %v, want %v", got, want)
	}
}

// TestRetainSkipsStoredMessages is the retry case: messages whose id is
// already stored are not stored or queued again; only new ones are.
func TestRetainSkipsStoredMessages(t *testing.T) {
	ctx := context.Background()
	tx := testsupport.Tx(t)
	service := retain.NewService(tx, insertClient(t))
	workspaceID := insertWorkspace(t, tx)
	sessionID := uuid.New()
	old := retain.Message{ID: uuid.New(), Participant: "ana", Content: "I'm vegetarian."}
	added := retain.Message{ID: uuid.New(), Participant: "ana", Content: "And I hate cilantro."}

	request := retain.Request{WorkspaceID: workspaceID, SessionID: sessionID, Messages: []retain.Message{old}}
	original, err := service.Retain(ctx, request)
	if err != nil {
		t.Fatalf("first Retain: %v", err)
	}
	retried, err := service.Retain(ctx, request)
	if err != nil {
		t.Fatalf("retried Retain: %v", err)
	}
	if !slices.Equal(retried, original) {
		t.Fatalf("retry returned %+v, want the original %+v", retried, original)
	}

	request.Messages = []retain.Message{old, added}
	if _, err := service.Retain(ctx, request); err != nil {
		t.Fatalf("Retain with one new message: %v", err)
	}

	want := [][]uuid.UUID{{old.ID}, {added.ID}}
	if got := extractJobs(t, tx, workspaceID); !slices.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("extract jobs = %v, want %v (one per call that stored something)", got, want)
	}
	if got := countRows(t, tx, "messages", workspaceID); got != 2 {
		t.Fatalf("messages = %d, want 2", got)
	}
}

// TestRetainRollsBackWhenEnqueueFails proves B1: if the extraction job
// can't be queued, no message is stored either.
func TestRetainRollsBackWhenEnqueueFails(t *testing.T) {
	tx := testsupport.Tx(t)
	workspaceID := insertWorkspace(t, tx)

	_, err := retain.NewService(tx, failingInserter{}).Retain(context.Background(), retain.Request{
		WorkspaceID: workspaceID,
		SessionID:   uuid.New(),
		Messages:    []retain.Message{{ID: uuid.New(), Participant: "ana", Content: "I'm vegetarian."}},
	})
	if err == nil {
		t.Fatal("Retain succeeded, want the enqueue error")
	}

	for _, table := range []string{"sessions", "participants", "messages"} {
		if got := countRows(t, tx, table, workspaceID); got != 0 {
			t.Errorf("%s rows after failed enqueue = %d, want 0", table, got)
		}
	}
}

func TestRetainRejectsReusedMessageID(t *testing.T) {
	ctx := context.Background()
	tx := testsupport.Tx(t)
	service := retain.NewService(tx, insertClient(t))
	request := retain.Request{
		WorkspaceID: insertWorkspace(t, tx),
		SessionID:   uuid.New(),
		Messages:    []retain.Message{{ID: uuid.New(), Participant: "ana", Content: "I'm vegetarian."}},
	}
	if _, err := service.Retain(ctx, request); err != nil {
		t.Fatalf("first Retain: %v", err)
	}

	request.Messages[0].Content = "I eat fish again."
	if _, err := service.Retain(ctx, request); !errors.Is(err, retain.ErrMessageIDReused) {
		t.Fatalf("Retain with a reused id error = %v, want ErrMessageIDReused", err)
	}
}

// TestRetainKeepsMessageIDsInTheirWorkspace proves B3 for messages: an id
// stored in workspace A can't be read or claimed through workspace B.
func TestRetainKeepsMessageIDsInTheirWorkspace(t *testing.T) {
	ctx := context.Background()
	tx := testsupport.Tx(t)
	service := retain.NewService(tx, insertClient(t))
	messages := []retain.Message{{ID: uuid.New(), Participant: "ana", Content: "hi"}}

	if _, err := service.Retain(ctx, retain.Request{
		WorkspaceID: insertWorkspace(t, tx), SessionID: uuid.New(), Messages: messages,
	}); err != nil {
		t.Fatalf("Retain in workspace A: %v", err)
	}
	_, err := service.Retain(ctx, retain.Request{
		WorkspaceID: insertWorkspace(t, tx), SessionID: uuid.New(), Messages: messages,
	})
	if !errors.Is(err, retain.ErrMessageIDReused) {
		t.Fatalf("Retain in workspace B error = %v, want ErrMessageIDReused", err)
	}
}

func TestRetainRejectsUnknownWorkspace(t *testing.T) {
	_, err := retain.NewService(testsupport.Tx(t), insertClient(t)).Retain(context.Background(), retain.Request{
		WorkspaceID: uuid.New(),
		SessionID:   uuid.New(),
		Messages:    []retain.Message{{ID: uuid.New(), Participant: "ana", Content: "hi"}},
	})
	if !errors.Is(err, retain.ErrWorkspaceNotFound) {
		t.Fatalf("Retain error = %v, want ErrWorkspaceNotFound", err)
	}
}

// TestRetainKeepsSessionsInTheirWorkspace proves B3: a session id that
// belongs to workspace A can't be written to through workspace B.
func TestRetainKeepsSessionsInTheirWorkspace(t *testing.T) {
	ctx := context.Background()
	tx := testsupport.Tx(t)
	service := retain.NewService(tx, insertClient(t))
	sessionID := uuid.New()

	if _, err := service.Retain(ctx, retain.Request{
		WorkspaceID: insertWorkspace(t, tx), SessionID: sessionID,
		Messages: []retain.Message{{ID: uuid.New(), Participant: "ana", Content: "hi"}},
	}); err != nil {
		t.Fatalf("Retain in workspace A: %v", err)
	}
	_, err := service.Retain(ctx, retain.Request{
		WorkspaceID: insertWorkspace(t, tx), SessionID: sessionID,
		Messages: []retain.Message{{ID: uuid.New(), Participant: "ana", Content: "hi"}},
	})
	if !errors.Is(err, retain.ErrSessionNotFound) {
		t.Fatalf("Retain in workspace B error = %v, want ErrSessionNotFound", err)
	}
}

func insertWorkspace(t *testing.T, tx pgx.Tx) uuid.UUID {
	t.Helper()
	workspace, err := sqlc.New(tx).InsertWorkspace(context.Background(), sqlc.InsertWorkspaceParams{
		ID:   uuid.New(),
		Name: t.Name(),
	})
	if err != nil {
		t.Fatalf("InsertWorkspace: %v", err)
	}
	return workspace.ID
}

func insertClient(t *testing.T) *river.Client[pgx.Tx] {
	t.Helper()
	client, err := jobs.NewInsertClient(testsupport.Pool(t))
	if err != nil {
		t.Fatalf("NewInsertClient: %v", err)
	}
	return client
}

func countRows(t *testing.T, tx pgx.Tx, table string, workspaceID uuid.UUID) int {
	t.Helper()
	var count int
	query := "SELECT count(*) FROM " + table + " WHERE workspace_id = $1"
	if err := tx.QueryRow(context.Background(), query, workspaceID).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

// extractJobs returns the message ids of each extract job queued for the
// workspace, oldest job first.
func extractJobs(t *testing.T, tx pgx.Tx, workspaceID uuid.UUID) [][]uuid.UUID {
	t.Helper()
	rows, err := tx.Query(context.Background(),
		`SELECT args FROM river_job WHERE kind = 'extract' AND args->>'workspace_id' = $1 ORDER BY id`,
		workspaceID.String(),
	)
	if err != nil {
		t.Fatalf("query extract jobs: %v", err)
	}
	encoded, err := pgx.CollectRows(rows, pgx.RowTo[[]byte])
	if err != nil {
		t.Fatalf("collect extract jobs: %v", err)
	}
	messageIDs := make([][]uuid.UUID, len(encoded))
	for i, args := range encoded {
		var extract jobs.ExtractArgs
		if err := json.Unmarshal(args, &extract); err != nil {
			t.Fatalf("decode extract args: %v", err)
		}
		messageIDs[i] = extract.MessageIDs
	}
	return messageIDs
}
