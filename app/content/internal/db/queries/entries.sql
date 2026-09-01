-- name: GetEntryMeta :one
SELECT id, project_id, slug, directory_path, title, tags, COALESCE(owner_user_id,''), created_at, updated_at, COALESCE(body_hash,''), orphaned_at FROM entries WHERE id = ?;

-- name: ListEntriesForProject :many
SELECT id, project_id, slug, directory_path, title, tags, COALESCE(owner_user_id,''), created_at, updated_at, COALESCE(body_hash,''), orphaned_at FROM entries WHERE project_id = ? ORDER BY directory_path, slug;

-- name: InsertEntry :exec
INSERT INTO entries (id, project_id, slug, directory_path, title, tags, owner_user_id, created_at, updated_at, body_hash)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateEntry :exec
UPDATE entries SET slug = ?, directory_path = ?, title = ?, tags = ?, updated_at = ?, body_hash = ? WHERE id = ?;

-- name: DeleteEntry :exec
DELETE FROM entries WHERE id = ?;
