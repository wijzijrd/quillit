-- Queries backing app/svc/internal/handler/game_sessions.go.

-- name: CountProjectMembership :one
SELECT COUNT(*) FROM project_members WHERE project_id = ? AND user_id = ?;

-- name: GetGameSession :one
SELECT id, project_id, status, started_by, started_at, stopped_by, stopped_at FROM game_sessions WHERE id = ?;

-- name: GetRunningGameSession :one
SELECT id, project_id, status, started_by, started_at, stopped_by, stopped_at FROM game_sessions WHERE project_id = ? AND status = 'running';

-- name: InsertGameSession :exec
INSERT INTO game_sessions (id, project_id, status, started_by, started_at) VALUES (?, ?, 'running', ?, ?);

-- name: StopGameSession :one
UPDATE game_sessions SET status = 'stopped', stopped_by = ?, stopped_at = ?
WHERE project_id = ? AND status = 'running'
RETURNING id, project_id, status, started_by, started_at, stopped_by, stopped_at;

-- name: CountGameSessionByIDAndProject :one
SELECT COUNT(*) FROM game_sessions WHERE id = ? AND project_id = ?;

-- name: ListChatMessages :many
SELECT id, session_id, project_id, sender_id, type, body, entry_id, card_title, card_body, created_at
FROM chat_messages WHERE session_id = ? ORDER BY created_at ASC;

-- name: ListGameSessionsForProject :many
SELECT id, project_id, status, started_by, started_at, stopped_by, stopped_at FROM game_sessions WHERE project_id = ? ORDER BY started_at DESC LIMIT 100;
