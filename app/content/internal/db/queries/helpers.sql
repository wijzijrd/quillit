-- name: GetProjectIDForEntry :one
SELECT project_id FROM entries WHERE id = ?;
