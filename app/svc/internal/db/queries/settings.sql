-- Queries backing app/svc/internal/handler/settings.go.

-- name: GetUserSettings :one
SELECT settings FROM user_settings WHERE user_id = ?;

-- name: UpsertUserSettings :exec
INSERT INTO user_settings (user_id, settings, updated_at) VALUES (?, ?, ?)
ON CONFLICT(user_id) DO UPDATE SET settings = excluded.settings, updated_at = excluded.updated_at;
