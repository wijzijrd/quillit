-- name: ListEntryLinksForEntry :many
SELECT target_path, COALESCE(target_entry_id, ''), label, COALESCE(card_facet, ''), resolved
FROM entry_links
WHERE entry_id = ?
ORDER BY rowid;

-- name: ListEntryIDsForProject :many
SELECT id FROM entries WHERE project_id = ?;

-- name: ListDanglingLinksForProject :many
SELECT el.entry_id, el.target_path, el.label
FROM entry_links el
JOIN entries e ON e.id = el.entry_id
WHERE e.project_id = ? AND el.resolved = 0
ORDER BY el.entry_id, el.target_path;
