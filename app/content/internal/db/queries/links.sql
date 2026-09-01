-- name: DeleteEntryLinks :exec
DELETE FROM entry_links WHERE entry_id = ?;

-- name: InsertEntryLink :exec
INSERT INTO entry_links (entry_id, target_path, target_entry_id, label, card_facet, resolved)
VALUES (?, ?, ?, ?, ?, ?);

-- name: FindEntryIDAtPath :one
SELECT id FROM entries WHERE project_id = ? AND directory_path = ? AND slug = ?;
