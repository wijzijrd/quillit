-- name: CountUsers :one
SELECT COUNT(*) FROM users;

-- name: CountUsersByUsername :one
SELECT COUNT(*) FROM users WHERE username = ?;

-- name: InsertUser :exec
INSERT INTO users (id, email, username, password_hash, role, created_at, updated_at)
VALUES (?, ?, ?, ?, 'user', ?, ?);

-- name: GetUserForLogin :one
SELECT id, password_hash, role, active FROM users WHERE email = ?;

-- name: GetUserRoleActive :one
SELECT role, active FROM users WHERE id = ?;

-- name: SearchUsers :many
SELECT id, email, username FROM users
WHERE email LIKE ? OR username LIKE ?
ORDER BY username
LIMIT 20;

-- name: UpdateUserActive :execrows
UPDATE users SET active = ?, updated_at = ? WHERE id = ?;

-- name: GetUserByID :one
SELECT id, email, username, role, active, created_at FROM users WHERE id = ?;

-- name: DeleteUser :execrows
DELETE FROM users WHERE id = ?;

-- name: CountUsersByEmail :one
SELECT COUNT(*) FROM users WHERE email = ?;

-- name: InsertAdminUser :exec
INSERT INTO users (id, email, username, password_hash, role, created_at, updated_at)
VALUES (?, ?, ?, ?, 'admin', ?, ?);
