-- name: CheckEntryPathCollision :one
SELECT 1 FROM entries WHERE project_id = ? AND directory_path = ? AND slug = ? AND id != ?;

-- name: UpdateEntryDirectoryPath :exec
UPDATE entries SET directory_path = ?, updated_at = ? WHERE id = ?;

-- name: RewriteEntryLinkTargets :exec
UPDATE entry_links SET target_path = ?
WHERE target_path = ? AND entry_id IN (SELECT id FROM entries WHERE project_id = ?);
