-- name: GetUserIDByEmail :one
SELECT id FROM users WHERE email = ?;

-- name: DeleteUnusedResetTokens :exec
DELETE FROM password_reset_tokens WHERE user_id = ? AND used = 0;

-- name: InsertResetToken :exec
INSERT INTO password_reset_tokens (id, user_id, token_hash, expires_at, used, created_at)
VALUES (?, ?, ?, ?, 0, ?);

-- name: GetValidResetToken :one
SELECT id, user_id FROM password_reset_tokens
WHERE token_hash = ? AND used = 0 AND expires_at > ?;

-- name: UpdateUserPassword :exec
UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?;

-- name: MarkResetTokenUsed :exec
UPDATE password_reset_tokens SET used = 1 WHERE id = ?;
