package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/Kerliula/sammy-memory/internal/retain"
)

var retainRoute = huma.Operation{
	OperationID:   "retain",
	Method:        http.MethodPost,
	Path:          "/v1/workspaces/{workspaceID}/sessions/{sessionID}/messages",
	Summary:       "Retain messages",
	Description:   "Stores messages and queues them for extraction. The session and participants are created on first use. Messages whose id is already stored are skipped, so retrying a call is safe.",
	DefaultStatus: http.StatusAccepted,
	// Room for 100 ASCII messages at the 65,536-character limit, plus JSON.
	MaxBodyBytes: 8 << 20,
}

// The patterns reject NUL characters, which Postgres text can't store.
type retainMessageBody struct {
	ID          uuid.UUID  `json:"id" format:"uuid" doc:"Your id for the message. Generate it once and reuse it when retrying, so the message is never stored twice."`
	Participant string     `json:"participant" minLength:"1" maxLength:"255" pattern:"^[^\\x00]*$" doc:"Who said the message: your identifier for the participant, unique within the workspace."`
	Content     string     `json:"content" minLength:"1" maxLength:"65536" pattern:"^[^\\x00]*$"`
	SentAt      *time.Time `json:"sent_at,omitempty" doc:"When the message was said. Defaults to when Sammy receives it."`
}

type retainInput struct {
	WorkspaceID string `path:"workspaceID" format:"uuid"`
	SessionID   string `path:"sessionID" format:"uuid"`
	Body        struct {
		Messages []retainMessageBody `json:"messages" minItems:"1" maxItems:"100"`
	}
}

type storedMessageBody struct {
	ID          uuid.UUID `json:"id"`
	Participant string    `json:"participant"`
	Content     string    `json:"content"`
	SentAt      time.Time `json:"sent_at"`
	CreatedAt   time.Time `json:"created_at"`
}

type retainOutput struct {
	Body struct {
		Messages []storedMessageBody `json:"messages"`
	}
}

func (server *Server) retainMessages(ctx context.Context, input *retainInput) (*retainOutput, error) {
	workspaceID, err := uuid.Parse(input.WorkspaceID)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("invalid workspace id")
	}
	sessionID, err := uuid.Parse(input.SessionID)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("invalid session id")
	}

	request := retain.Request{
		WorkspaceID: workspaceID,
		SessionID:   sessionID,
		Messages:    make([]retain.Message, len(input.Body.Messages)),
	}
	for i, message := range input.Body.Messages {
		request.Messages[i] = retain.Message(message)
	}

	stored, err := server.retainer.Retain(ctx, request)
	switch {
	case errors.Is(err, retain.ErrWorkspaceNotFound):
		return nil, huma.Error404NotFound("workspace not found")
	case errors.Is(err, retain.ErrSessionNotFound):
		return nil, huma.Error404NotFound("session not found")
	case errors.Is(err, retain.ErrMessageIDReused):
		return nil, huma.Error422UnprocessableEntity(err.Error())
	case err != nil:
		return nil, err
	}

	response := &retainOutput{}
	response.Body.Messages = make([]storedMessageBody, len(stored))

	for i, message := range stored {
		response.Body.Messages[i] = storedMessageBody(message)
	}

	return response, nil
}
