-- name: FindEntryAtPathForImport :one
SELECT id FROM entries WHERE project_id = ? AND directory_path = ? AND slug = ?;

-- name: InsertProjectFacetNew :exec
INSERT INTO project_facets (project_id, name) VALUES (?, ?);

-- name: UpdateEntryOverwrite :exec
UPDATE entries SET title = ?, tags = ?, updated_at = ?, body_hash = ? WHERE id = ?;

-- name: InsertImportedEntry :exec
INSERT INTO entries (id, project_id, slug, directory_path, title, tags, owner_user_id, created_at, updated_at, body_hash)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: CheckEntrySlugAtPath :one
SELECT 1 FROM entries WHERE project_id = ? AND directory_path = ? AND slug = ?;

