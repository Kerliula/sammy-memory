package server_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/Kerliula/sammy-memory/internal/db/sqlc"
	"github.com/Kerliula/sammy-memory/internal/jobs"
	"github.com/Kerliula/sammy-memory/internal/retain"
	"github.com/Kerliula/sammy-memory/internal/server"
	"github.com/Kerliula/sammy-memory/internal/testsupport"
)

func retainBody() string {
	return fmt.Sprintf(`{"messages": [
		{"id": %q, "participant": "ana", "content": "I'm vegetarian.", "sent_at": "2026-10-30T09:00:00Z"},
		{"id": %q, "participant": "assistant", "content": "Noted."}
	]}`, uuid.NewString(), uuid.NewString())
}

// TestRetainIsIdempotent sends the same call twice, as a client retrying
// after a timeout would, and checks nothing is stored or queued twice.
func TestRetainIsIdempotent(t *testing.T) {
	ctx := context.Background()
	tx := testsupport.Tx(t)
	jobClient, err := jobs.NewInsertClient(testsupport.Pool(t))
	if err != nil {
		t.Fatalf("NewInsertClient: %v", err)
	}
	handler := server.New(testsupport.Pool(t), retain.NewService(tx, jobClient))

	workspace, err := sqlc.New(tx).InsertWorkspace(ctx, sqlc.InsertWorkspaceParams{ID: uuid.New(), Name: t.Name()})
	if err != nil {
		t.Fatalf("InsertWorkspace: %v", err)
	}
	path := "/v1/workspaces/" + workspace.ID.String() + "/sessions/" + uuid.NewString() + "/messages"
	body := retainBody()

	first := postRetain(handler, path, body)
	second := postRetain(handler, path, body)

	for name, rec := range map[string]*httptest.ResponseRecorder{"first": first, "second": second} {
		if rec.Code != http.StatusAccepted {
			t.Fatalf("%s POST status = %d, want 202; body: %s", name, rec.Code, rec.Body.String())
		}
	}
	if first.Body.String() != second.Body.String() {
		t.Fatalf("retried body differs:\nfirst:  %s\nsecond: %s", first.Body.String(), second.Body.String())
	}

	var messages, extractJobs int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM messages WHERE workspace_id = $1`, workspace.ID).Scan(&messages); err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM river_job WHERE kind = 'extract' AND args->>'workspace_id' = $1`, workspace.ID.String(),
	).Scan(&extractJobs); err != nil {
		t.Fatalf("count extract jobs: %v", err)
	}
	if messages != 2 || extractJobs != 1 {
		t.Fatalf("after two identical calls: %d messages, %d extract jobs; want 2 and 1", messages, extractJobs)
	}
}

func TestRetainErrors(t *testing.T) {
	jobClient, err := jobs.NewInsertClient(testsupport.Pool(t))
	if err != nil {
		t.Fatalf("NewInsertClient: %v", err)
	}
	handler := server.New(testsupport.Pool(t), retain.NewService(testsupport.Tx(t), jobClient))
	unknownWorkspace := "/v1/workspaces/" + uuid.NewString() + "/sessions/" + uuid.NewString() + "/messages"

	tests := []struct {
		name       string
		path       string
		body       string
		wantStatus int
	}{
		{"unknown workspace", unknownWorkspace, retainBody(), http.StatusNotFound},
		{"missing message id", unknownWorkspace, `{"messages": [{"participant": "ana", "content": "hi"}]}`, http.StatusUnprocessableEntity},
		{"invalid message id", unknownWorkspace, `{"messages": [{"id": "nope", "participant": "ana", "content": "hi"}]}`, http.StatusUnprocessableEntity},
		{"no messages", unknownWorkspace, `{"messages": []}`, http.StatusUnprocessableEntity},
		{"empty content", unknownWorkspace, `{"messages": [{"id": "` + uuid.NewString() + `", "participant": "ana", "content": ""}]}`, http.StatusUnprocessableEntity},
		{"NUL in content", unknownWorkspace, `{"messages": [{"id": "` + uuid.NewString() + `", "participant": "ana", "content": "hi\u0000there"}]}`, http.StatusUnprocessableEntity},
		{"bad session id", "/v1/workspaces/" + uuid.NewString() + "/sessions/nope/messages", retainBody(), http.StatusUnprocessableEntity},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rec := postRetain(handler, test.path, test.body)
			if rec.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", rec.Code, test.wantStatus, rec.Body.String())
			}
		})
	}
}

func postRetain(handler http.Handler, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
