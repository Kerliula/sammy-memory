-- name: InsertWorkspace :one
INSERT INTO workspaces (id, name)
VALUES ($1, $2)
RETURNING id, name, created_at;

-- name: GetWorkspace :one
SELECT id, name, created_at
FROM workspaces
WHERE id = $1;
