-- +goose Up
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE workspaces (
    id uuid PRIMARY KEY,
    name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS workspaces;
DROP EXTENSION IF EXISTS btree_gist;
DROP EXTENSION IF EXISTS vector;
