-- Queries backing app/svc/internal/handler/contentproxy.go.

-- name: CheckProjectMembership :one
SELECT 1 FROM project_members WHERE project_id = ? AND user_id = ?;
