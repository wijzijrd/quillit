-- Queries backing app/svc/internal/handler/chat_ws.go.

-- name: CountChatProjectMembership :one
SELECT COUNT(*) FROM project_members WHERE project_id = ? AND user_id = ?;

-- name: GetRunningGameSessionID :one
SELECT id FROM game_sessions WHERE project_id = ? AND status = 'running';

-- name: InsertChatMessage :exec
INSERT INTO chat_messages (id, session_id, project_id, sender_id, type, body, entry_id, card_title, card_body, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
