-- Queries backing app/svc/internal/handler/admin.go.

-- name: AdminListProjects :many
SELECT p.id, p.name, p.type, p.created_by, p.created_at,
       COUNT(pm.id) AS member_count
FROM projects p
LEFT JOIN project_members pm ON pm.project_id = p.id
WHERE p.name LIKE ?
GROUP BY p.id
ORDER BY p.created_at DESC;

-- name: CountProjectByID :one
SELECT COUNT(*) FROM projects WHERE id = ?;

-- name: AdminListProjectMembers :many
SELECT id, project_id, user_id, role, joined_at, username FROM project_members WHERE project_id = ? ORDER BY joined_at ASC;
