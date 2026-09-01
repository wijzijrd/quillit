-- Queries backing app/svc/internal/handler/projects.go.

-- name: ListProjectsForUser :many
SELECT p.id, p.name, p.type, p.created_by, p.created_at,
       pm.role,
       (SELECT COUNT(*) FROM project_members WHERE project_id = p.id) AS member_count,
       EXISTS(SELECT 1 FROM game_sessions gs WHERE gs.project_id = p.id AND gs.status = 'running') AS live
FROM projects p
JOIN project_members pm ON pm.project_id = p.id AND pm.user_id = ?
WHERE p.type != 'global'
ORDER BY p.created_at DESC;

-- name: InsertProject :exec
INSERT INTO projects (id, name, type, created_by, created_at) VALUES (?, ?, ?, ?, ?);

-- name: InsertProjectMember :exec
INSERT INTO project_members (id, project_id, user_id, role, joined_at, username)
VALUES (?, ?, ?, ?, ?, '');

-- name: UpdateProjectName :exec
UPDATE projects SET name = ? WHERE id = ?;

-- name: GetProjectCreatedBy :one
SELECT created_by FROM projects WHERE id = ?;

-- name: DeleteProject :exec
DELETE FROM projects WHERE id = ?;

-- name: ListProjectMembers :many
SELECT id, project_id, user_id, username, role, joined_at FROM project_members WHERE project_id = ? ORDER BY joined_at ASC;

-- name: AddProjectMember :exec
INSERT OR IGNORE INTO project_members (id, project_id, user_id, role, joined_at, username)
VALUES (?, ?, ?, ?, ?, ?);

-- name: DeleteProjectMember :exec
DELETE FROM project_members WHERE project_id = ? AND user_id = ?;

-- name: InsertProjectInvite :exec
INSERT INTO project_invites (id, token, project_id, role, created_by, expires_at) VALUES (?, ?, ?, ?, ?, ?);

-- name: DeleteProjectInvite :exec
DELETE FROM project_invites WHERE token = ? AND project_id = ?;

-- name: GetProjectInviteByToken :one
SELECT id, project_id, role, expires_at, used_at FROM project_invites WHERE token = ?;

-- name: JoinProjectAddMember :exec
INSERT OR IGNORE INTO project_members (id, project_id, user_id, role, joined_at, username)
VALUES (?, ?, ?, ?, ?, '');

-- name: MarkProjectInviteUsed :exec
UPDATE project_invites SET used_at = ?, used_by = ? WHERE id = ?;

-- name: GetMemberRole :one
SELECT p.type, pm.role
FROM projects p
JOIN project_members pm ON pm.project_id = p.id AND pm.user_id = ?
WHERE p.id = ?;

-- name: GetProjectForMember :one
SELECT p.id, p.name, p.type, p.created_by, p.created_at,
       pm.role,
       (SELECT COUNT(*) FROM project_members WHERE project_id = p.id) AS member_count
FROM projects p
JOIN project_members pm ON pm.project_id = p.id AND pm.user_id = ?
WHERE p.id = ?;
