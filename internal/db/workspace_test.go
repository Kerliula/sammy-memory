package db_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/Kerliula/sammy-memory/internal/db/sqlc"
	"github.com/Kerliula/sammy-memory/internal/testsupport"
)

func TestMain(m *testing.M) {
	os.Exit(testsupport.Run(m))
}

func TestWorkspaceRoundTrip(t *testing.T) {
	ctx := context.Background()
	queries := sqlc.New(testsupport.Tx(t))

	inserted, err := queries.InsertWorkspace(ctx, sqlc.InsertWorkspaceParams{
		ID:   uuid.New(),
		Name: "round-trip",
	})
	if err != nil {
		t.Fatalf("InsertWorkspace: %v", err)
	}
	if inserted.Name != "round-trip" || inserted.CreatedAt.IsZero() {
		t.Fatalf("InsertWorkspace returned %+v", inserted)
	}

	got, err := queries.GetWorkspace(ctx, inserted.ID)
	if err != nil {
		t.Fatalf("GetWorkspace: %v", err)
	}
	if got != inserted {
		t.Fatalf("GetWorkspace = %+v, want %+v", got, inserted)
	}
}
