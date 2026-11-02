// Package retain stores incoming messages and queues them for extraction,
// both in one transaction, so a stored message is never left unprocessed.
package retain

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/Kerliula/sammy-memory/internal/db/sqlc"
	"github.com/Kerliula/sammy-memory/internal/jobs"
)

var (
	ErrWorkspaceNotFound = errors.New("workspace not found")
	// ErrSessionNotFound also covers a session id owned by another
	// workspace, so callers can't probe ids across workspaces.
	ErrSessionNotFound = errors.New("session not found")
	ErrMessageIDReused = errors.New("message id was already used for a different message")
)

// sessionForeignKey is the constraint that rejects a message pointing at a
// session in another workspace.
const sessionForeignKey = "messages_workspace_id_session_id_fkey"

// TxBeginner starts a transaction. *pgxpool.Pool satisfies it, and so does
// pgx.Tx (as a savepoint), which lets tests roll everything back.
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// JobInserter enqueues a job inside a transaction. *river.Client[pgx.Tx]
// satisfies it.
type JobInserter interface {
	InsertTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

// Message is one message as the caller sent it. The caller picks ID, so
// sending the same message again stores nothing new.
type Message struct {
	ID          uuid.UUID
	Participant string
	Content     string
	SentAt      *time.Time // nil means "when Sammy received it"
}

// Request is one retain call. Sessions and participants are created on first use.
type Request struct {
	WorkspaceID uuid.UUID
	SessionID   uuid.UUID
	Messages    []Message
}

// StoredMessage is a message as Sammy saved it.
type StoredMessage struct {
	ID          uuid.UUID
	Participant string
	Content     string
	SentAt      time.Time
	CreatedAt   time.Time
}

type Service struct {
	db   TxBeginner
	jobs JobInserter
}

func NewService(db TxBeginner, jobs JobInserter) *Service {
	return &Service{db: db, jobs: jobs}
}

// Retain stores req's new messages and enqueues one extraction job for
// them. Either both happen or neither does. Messages that were already
// stored are skipped, so a retried call is safe.
func (s *Service) Retain(ctx context.Context, req Request) ([]StoredMessage, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin retain tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after Commit

	queries := sqlc.New(tx)

	if _, err := queries.GetWorkspace(ctx, req.WorkspaceID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrWorkspaceNotFound
		}
		return nil, fmt.Errorf("get workspace: %w", err)
	}

	// A session id already owned by another workspace is left alone here;
	// the message insert below then fails on sessionForeignKey.
	if err := queries.CreateSessionIfMissing(ctx, sqlc.CreateSessionIfMissingParams{
		ID:          req.SessionID,
		WorkspaceID: req.WorkspaceID,
	}); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	participantIDs, err := ensureParticipants(ctx, queries, req)
	if err != nil {
		return nil, err
	}

	params := make([]sqlc.InsertMessagesParams, len(req.Messages))
	for i, message := range req.Messages {
		var sentAt pgtype.Timestamptz
		if message.SentAt != nil {
			sentAt = pgtype.Timestamptz{Time: message.SentAt.Round(time.Microsecond), Valid: true}
		}
		params[i] = sqlc.InsertMessagesParams{
			ID:            message.ID,
			WorkspaceID:   req.WorkspaceID,
			SessionID:     req.SessionID,
			ParticipantID: participantIDs[message.Participant],
			Content:       message.Content,
			SentAt:        sentAt,
		}
	}
	// A concurrent call with the same message id waits here until the
	// first one commits, then skips the message.
	var newIDs []uuid.UUID
	var insertErr error
	queries.InsertMessages(ctx, params).QueryRow(func(_ int, id uuid.UUID, err error) {
		switch {
		case insertErr != nil:
		case errors.Is(err, pgx.ErrNoRows): // already stored
		case err != nil:
			insertErr = err
		default:
			newIDs = append(newIDs, id)
		}
	})
	if insertErr != nil {
		var pgErr *pgconn.PgError
		if errors.As(insertErr, &pgErr) && pgErr.ConstraintName == sessionForeignKey {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("insert messages: %w", insertErr)
	}

	stored, err := matchStored(ctx, queries, req)
	if err != nil {
		return nil, err
	}

	if len(newIDs) == 0 {
		return stored, nil // a retry: everything was already stored and queued
	}
	extract := jobs.ExtractArgs{WorkspaceID: req.WorkspaceID, MessageIDs: newIDs}
	if _, err := s.jobs.InsertTx(ctx, tx, extract, nil); err != nil {
		return nil, fmt.Errorf("enqueue extract job: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit retain tx: %w", err)
	}
	return stored, nil
}

// ensureParticipants creates the request's participants that don't exist
// yet and returns every participant's id by name. Names are inserted in
// sorted order, so two calls creating the same new names can't deadlock.
func ensureParticipants(ctx context.Context, queries *sqlc.Queries, req Request) (map[string]uuid.UUID, error) {
	names := make([]string, len(req.Messages))
	for i, message := range req.Messages {
		names[i] = message.Participant
	}
	slices.Sort(names)
	names = slices.Compact(names)

	if err := queries.CreateParticipants(ctx, sqlc.CreateParticipantsParams{
		WorkspaceID: req.WorkspaceID,
		Names:       names,
	}); err != nil {
		return nil, fmt.Errorf("create participants: %w", err)
	}
	rows, err := queries.ListParticipants(ctx, sqlc.ListParticipantsParams{
		WorkspaceID: req.WorkspaceID,
		Names:       names,
	})
	if err != nil {
		return nil, fmt.Errorf("list participants: %w", err)
	}
	ids := make(map[string]uuid.UUID, len(rows))
	for _, row := range rows {
		ids[row.Name] = row.ID
	}
	return ids, nil
}

// matchStored loads the stored copy of every requested message and checks
// it is the same message. A skipped insert means the id was already taken:
// by this message on an earlier try (fine), or by a different message,
// possibly in another workspace (the caller reused an id by mistake).
func matchStored(ctx context.Context, queries *sqlc.Queries, req Request) ([]StoredMessage, error) {
	messageIDs := make([]uuid.UUID, len(req.Messages))
	for i, message := range req.Messages {
		messageIDs[i] = message.ID
	}
	rows, err := queries.ListMessagesByIDs(ctx, sqlc.ListMessagesByIDsParams{
		WorkspaceID: req.WorkspaceID,
		Ids:         messageIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	byID := make(map[uuid.UUID]sqlc.ListMessagesByIDsRow, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}

	stored := make([]StoredMessage, len(req.Messages))
	for i, message := range req.Messages {
		row, ok := byID[message.ID]
		if !ok ||
			row.SessionID != req.SessionID ||
			row.Participant != message.Participant ||
			row.Content != message.Content ||
			// Postgres keeps microseconds, so compare at that precision.
			(message.SentAt != nil && !row.SentAt.Equal(message.SentAt.Round(time.Microsecond))) {
			return nil, ErrMessageIDReused
		}
		stored[i] = StoredMessage{
			ID:          row.ID,
			Participant: row.Participant,
			Content:     row.Content,
			SentAt:      row.SentAt,
			CreatedAt:   row.CreatedAt,
		}
	}
	return stored, nil
}
