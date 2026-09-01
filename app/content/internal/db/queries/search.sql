-- name: InsertSearchIndexEntry :exec
INSERT INTO entries_fts (entry_id, title, tags, body)
VALUES (?, ?, ?, ?);

-- name: DeleteSearchIndexEntry :exec
DELETE FROM entries_fts WHERE entry_id = ?;
