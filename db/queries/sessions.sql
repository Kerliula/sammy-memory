-- name: CreateSessionIfMissing :exec
INSERT INTO sessions (id, workspace_id)
VALUES ($1, $2)
ON CONFLICT (id) DO NOTHING;
