-- Skips a message whose id is already stored, which makes retries safe.
-- Returns the id when stored, no row when skipped. Sent as one batch.
-- name: InsertMessages :batchone
INSERT INTO messages (id, workspace_id, session_id, participant_id, content, sent_at)
VALUES (
    @id, @workspace_id, @session_id, @participant_id, @content,
    COALESCE(sqlc.narg('sent_at')::timestamptz, now())
)
ON CONFLICT (id) DO NOTHING
RETURNING id;

-- name: ListMessagesByIDs :many
SELECT m.id, m.session_id, p.name AS participant, m.content, m.sent_at, m.created_at
FROM messages m
JOIN participants p ON p.id = m.participant_id
WHERE m.workspace_id = @workspace_id AND m.id = ANY(@ids::uuid[])
ORDER BY m.seq;
