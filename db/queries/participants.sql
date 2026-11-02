-- DO NOTHING takes no lock on participants that already exist, so
-- concurrent calls naming the same participants don't wait on each other.
-- name: CreateParticipants :exec
INSERT INTO participants (id, workspace_id, name)
SELECT gen_random_uuid(), @workspace_id::uuid, name
FROM unnest(@names::text[]) AS name
ON CONFLICT (workspace_id, name) DO NOTHING;

-- name: ListParticipants :many
SELECT id, name
FROM participants
WHERE workspace_id = @workspace_id AND name = ANY(@names::text[]);
