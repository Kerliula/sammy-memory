-- +goose Up
-- Every table carries workspace_id, and child rows reference their parents
-- through (workspace_id, id) pairs, so the schema itself refuses a row that
-- points into another workspace.

-- A participant is anyone who takes part in conversations: a human user or
-- an AI agent. Memory is kept per participant. People and things that are
-- only talked about are not participants; they become entities later.
CREATE TABLE participants (
    id uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 255),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, name),
    UNIQUE (workspace_id, id)
);

CREATE TABLE sessions (
    id uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, id)
);

CREATE TABLE messages (
    -- Chosen by the caller, so a retried call can't store a message twice.
    id uuid PRIMARY KEY,
    seq bigint GENERATED ALWAYS AS IDENTITY,
    workspace_id uuid NOT NULL,
    session_id uuid NOT NULL,
    participant_id uuid NOT NULL,
    content text NOT NULL CHECK (content <> ''),
    sent_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (workspace_id, session_id) REFERENCES sessions (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, participant_id) REFERENCES participants (workspace_id, id) ON DELETE CASCADE
);

CREATE INDEX messages_session_id_sent_at_idx ON messages (session_id, sent_at, seq);
CREATE INDEX messages_participant_id_idx ON messages (participant_id);

-- +goose Down
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS participants;
