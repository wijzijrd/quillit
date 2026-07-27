package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/quillit/svc/internal/middleware"
)

// projectTypeRoles maps project type → [editorRole, memberRole].
// The first entry is always the editor (creator) role.
var projectTypeRoles = map[string][2]string{
	"campaign": {"gm", "player"},
}

func editorRoleFor(projectType string) string {
	if r, ok := projectTypeRoles[projectType]; ok {
		return r[0]
	}
	return "editor"
}

func memberRoleFor(projectType string) string {
	if r, ok := projectTypeRoles[projectType]; ok {
		return r[1]
	}
	return "member"
}

func isEditorRole(projectType, role string) bool {
	return role == editorRoleFor(projectType)
}

const inviteTTL = 7 * 24 * time.Hour

// ── Handler struct ────────────────────────────────────────────────────────────

type ProjectsHandler struct {
	db        *sql.DB
	jwtSecret []byte
}

func NewProjects(db *sql.DB, jwtSecret string) *ProjectsHandler {
	return &ProjectsHandler{db: db, jwtSecret: []byte(jwtSecret)}
}

// callerID extracts the user ID (sub) from the JWT stored in request context.
// In tests, a caller ID can be injected directly via WithTestCallerID.
func (h *ProjectsHandler) callerID(r *http.Request) (string, bool) {
	// Test helper: allow injecting caller ID without JWT.
	if id, ok := r.Context().Value(testCallerKey{}).(string); ok && id != "" {
		return id, true
	}
	mc, err := middleware.ClaimsFromContext(r.Context(), h.jwtSecret)
	if err != nil {
		return "", false
	}
	sub, _ := mc["sub"].(string)
	return sub, sub != ""
}

// ── Response types ────────────────────────────────────────────────────────────

type Project struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Type        string          `json:"type"`
	CreatedBy   string          `json:"createdBy"`
	CreatedAt   int64           `json:"createdAt"`
	MemberCount int             `json:"memberCount"`
	MyRole      string          `json:"myRole"`
	RoleLabels  [2]string       `json:"roleLabels"` // [editorLabel, memberLabel]
	Live        bool            `json:"live"`
	Members     []ProjectMember `json:"members,omitempty"`
}

type ProjectMember struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	UserID    string `json:"userId"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	JoinedAt  int64  `json:"joinedAt"`
}

type ProjectInvite struct {
	ID        string `json:"id"`
	Token     string `json:"token"`
	ProjectID string `json:"projectId"`
	Role      string `json:"role"`
	CreatedBy string `json:"createdBy"`
	ExpiresAt int64  `json:"expiresAt"`
}

type ProjectType struct {
	Type        string    `json:"type"`
	Label       string    `json:"label"`
	RoleLabels  [2]string `json:"roleLabels"`
}

// ── Types endpoint ────────────────────────────────────────────────────────────

// Types godoc
// @Summary      List project types
// @Tags         projects
// @Produce      json
// @Success      200  {array}  ProjectType
// @Router       /api/projects/types [get]
func (h *ProjectsHandler) Types(w http.ResponseWriter, r *http.Request) {
	types := []ProjectType{
		{Type: "campaign", Label: "Campaign", RoleLabels: [2]string{"GM", "Player"}},
	}
	writeJSON(w, http.StatusOK, types)
}

// ── List / Create ─────────────────────────────────────────────────────────────

// List godoc
// @Summary      List projects for the current user
// @Tags         projects
// @Produce      json
// @Success      200  {array}   Project
// @Failure      401  {object}  ErrorResponse
// @Router       /api/projects [get]
func (h *ProjectsHandler) List(w http.ResponseWriter, r *http.Request) {
	callerID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	rows, err := h.db.QueryContext(r.Context(), `
		SELECT p.id, p.name, p.type, p.created_by, p.created_at,
		       pm.role,
		       (SELECT COUNT(*) FROM project_members WHERE project_id = p.id) AS member_count,
		       EXISTS(SELECT 1 FROM game_sessions gs WHERE gs.project_id = p.id AND gs.status = 'running') AS live
		FROM projects p
		JOIN project_members pm ON pm.project_id = p.id AND pm.user_id = ?
		WHERE p.type != 'global'
		ORDER BY p.created_at DESC
	`, callerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()

	projects := []Project{}
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.Type, &p.CreatedBy, &p.CreatedAt, &p.MyRole, &p.MemberCount, &p.Live); err != nil {
			writeError(w, http.StatusInternalServerError, "db error")
			return
		}
		p.RoleLabels = roleLabelPair(p.Type)
		projects = append(projects, p)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, projects)
}

// Create godoc
// @Summary      Create a project
// @Tags         projects
// @Accept       json
// @Produce      json
// @Param        body  body      object  true  "Project name and type"
// @Success      201   {object}  Project
// @Failure      400   {object}  ErrorResponse
// @Router       /api/projects [post]
func (h *ProjectsHandler) Create(w http.ResponseWriter, r *http.Request) {
	callerID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var body struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	if body.Type == "" {
		body.Type = "campaign"
	}
	if _, ok := projectTypeRoles[body.Type]; !ok {
		writeError(w, http.StatusBadRequest, "unsupported project type")
		return
	}

	now := nowUnix()
	projectID := newID()
	memberID := newID()
	editorRole := editorRoleFor(body.Type)

	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(r.Context(),
		"INSERT INTO projects (id, name, type, created_by, created_at) VALUES (?, ?, ?, ?, ?)",
		projectID, body.Name, body.Type, callerID, now,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	if _, err := tx.ExecContext(r.Context(),
		"INSERT INTO project_members (id, project_id, user_id, role, joined_at, username) VALUES (?, ?, ?, ?, ?, '')",
		memberID, projectID, callerID, editorRole, now,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	writeJSON(w, http.StatusCreated, Project{
		ID: projectID, Name: body.Name, Type: body.Type,
		CreatedBy: callerID, CreatedAt: now,
		MemberCount: 1, MyRole: editorRole,
		RoleLabels: roleLabelPair(body.Type),
	})
}

// ── Get / Update / Delete ─────────────────────────────────────────────────────

// Update godoc
// @Summary      Update a project
// @Tags         projects
// @Accept       json
// @Produce      json
// @Param        id    path      string  true  "Project ID"
// @Param        body  body      object  true  "Fields to update"
// @Success      200   {object}  Project
// @Failure      403   {object}  ErrorResponse
// @Failure      404   {object}  ErrorResponse
// @Router       /api/projects/{id} [patch]
func (h *ProjectsHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	callerID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	projectType, myRole, err := h.memberRole(r, id, callerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if !isEditorRole(projectType, myRole) {
		writeError(w, http.StatusForbidden, "editor access required")
		return
	}

	var body struct {
		Name *string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == nil || *body.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}

	if _, err := h.db.ExecContext(r.Context(),
		"UPDATE projects SET name = ? WHERE id = ?", *body.Name, id,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	writeJSON(w, http.StatusOK, h.fetchProject(r, id, callerID))
}

// Delete godoc
// @Summary      Delete a project
// @Tags         projects
// @Param        id  path  string  true  "Project ID"
// @Success      204
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /api/projects/{id} [delete]
func (h *ProjectsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	callerID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var createdBy string
	if err := h.db.QueryRowContext(r.Context(),
		"SELECT created_by FROM projects WHERE id = ?", id,
	).Scan(&createdBy); errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	if createdBy != callerID {
		writeError(w, http.StatusForbidden, "only the project creator can delete it")
		return
	}

	if _, err := h.db.ExecContext(r.Context(),
		"DELETE FROM projects WHERE id = ?", id,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Members ───────────────────────────────────────────────────────────────────

// ListMembers godoc
// @Summary      List project members
// @Tags         projects
// @Produce      json
// @Param        id  path  string  true  "Project ID"
// @Success      200  {array}   ProjectMember
// @Failure      403  {object}  ErrorResponse
// @Router       /api/projects/{id}/members [get]
func (h *ProjectsHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	callerID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Caller must be a member of this project
	if _, _, err := h.memberRole(r, id, callerID); err != nil {
		writeError(w, http.StatusForbidden, "not a project member")
		return
	}

	rows, err := h.db.QueryContext(r.Context(),
		"SELECT id, project_id, user_id, username, role, joined_at FROM project_members WHERE project_id = ? ORDER BY joined_at ASC", id,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()

	members := []ProjectMember{}
	for rows.Next() {
		var m ProjectMember
		if err := rows.Scan(&m.ID, &m.ProjectID, &m.UserID, &m.Username, &m.Role, &m.JoinedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "db error")
			return
		}
		members = append(members, m)
	}
	writeJSON(w, http.StatusOK, members)
}

// AddMember godoc
// @Summary      Add a member to a project
// @Tags         projects
// @Accept       json
// @Produce      json
// @Param        id    path  string  true  "Project ID"
// @Param        body  body  object  true  "userId and role"
// @Success      201   {object}  ProjectMember
// @Failure      403   {object}  ErrorResponse
// @Router       /api/projects/{id}/members [post]
func (h *ProjectsHandler) AddMember(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	callerID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	projectType, myRole, err := h.memberRole(r, id, callerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if !isEditorRole(projectType, myRole) {
		writeError(w, http.StatusForbidden, "editor access required")
		return
	}

	var body struct {
		UserID   string `json:"userId"`
		Role     string `json:"role"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.UserID == "" || body.Role == "" {
		writeError(w, http.StatusBadRequest, "userId and role required")
		return
	}

	now := nowUnix()
	m := ProjectMember{ID: newID(), ProjectID: id, UserID: body.UserID, Username: body.Username, Role: body.Role, JoinedAt: now}
	if _, err := h.db.ExecContext(r.Context(),
		"INSERT OR IGNORE INTO project_members (id, project_id, user_id, role, joined_at, username) VALUES (?, ?, ?, ?, ?, ?)",
		m.ID, m.ProjectID, m.UserID, m.Role, m.JoinedAt, m.Username,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

// RemoveMember godoc
// @Summary      Remove a member from a project
// @Tags         projects
// @Param        id      path  string  true  "Project ID"
// @Param        userId  path  string  true  "User ID to remove"
// @Success      204
// @Failure      403  {object}  ErrorResponse
// @Router       /api/projects/{id}/members/{userId} [delete]
func (h *ProjectsHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	targetUserID := chi.URLParam(r, "userId")
	callerID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	projectType, myRole, err := h.memberRole(r, id, callerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	// Editors can remove anyone; members can only leave (remove themselves)
	if !isEditorRole(projectType, myRole) && targetUserID != callerID {
		writeError(w, http.StatusForbidden, "editor access required")
		return
	}

	if _, err := h.db.ExecContext(r.Context(),
		"DELETE FROM project_members WHERE project_id = ? AND user_id = ?", id, targetUserID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Invites ───────────────────────────────────────────────────────────────────

// CreateInvite godoc
// @Summary      Generate an invite link for a project
// @Tags         projects
// @Accept       json
// @Produce      json
// @Param        id    path  string  true  "Project ID"
// @Param        body  body  object  false "role override (defaults to member role)"
// @Success      201   {object}  ProjectInvite
// @Failure      403   {object}  ErrorResponse
// @Router       /api/projects/{id}/invite [post]
func (h *ProjectsHandler) CreateInvite(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	callerID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	projectType, myRole, err := h.memberRole(r, id, callerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if !isEditorRole(projectType, myRole) {
		writeError(w, http.StatusForbidden, "editor access required")
		return
	}

	var body struct {
		Role string `json:"role"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Role == "" {
		body.Role = memberRoleFor(projectType)
	}

	inv := ProjectInvite{
		ID:        newID(),
		Token:     newID(), // 24-char hex token
		ProjectID: id,
		Role:      body.Role,
		CreatedBy: callerID,
		ExpiresAt: time.Now().Add(inviteTTL).Unix(),
	}
	if _, err := h.db.ExecContext(r.Context(),
		"INSERT INTO project_invites (id, token, project_id, role, created_by, expires_at) VALUES (?, ?, ?, ?, ?, ?)",
		inv.ID, inv.Token, inv.ProjectID, inv.Role, inv.CreatedBy, inv.ExpiresAt,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusCreated, inv)
}

// RevokeInvite godoc
// @Summary      Revoke an invite token
// @Tags         projects
// @Param        id     path  string  true  "Project ID"
// @Param        token  path  string  true  "Invite token"
// @Success      204
// @Failure      403  {object}  ErrorResponse
// @Router       /api/projects/{id}/invite/{token} [delete]
func (h *ProjectsHandler) RevokeInvite(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	token := chi.URLParam(r, "token")
	callerID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	projectType, myRole, err := h.memberRole(r, id, callerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if !isEditorRole(projectType, myRole) {
		writeError(w, http.StatusForbidden, "editor access required")
		return
	}

	if _, err := h.db.ExecContext(r.Context(),
		"DELETE FROM project_invites WHERE token = ? AND project_id = ?", token, id,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Join godoc
// @Summary      Join a project via invite token
// @Tags         projects
// @Accept       json
// @Produce      json
// @Param        body  body  object  true  "invite token"
// @Success      200   {object}  ProjectMember
// @Failure      400   {object}  ErrorResponse
// @Failure      410   {object}  ErrorResponse
// @Router       /api/projects/join [post]
func (h *ProjectsHandler) Join(w http.ResponseWriter, r *http.Request) {
	callerID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Token == "" {
		writeError(w, http.StatusBadRequest, "token required")
		return
	}

	// Look up the invite
	var inv ProjectInvite
	var usedAt sql.NullInt64
	err := h.db.QueryRowContext(r.Context(),
		"SELECT id, project_id, role, expires_at, used_at FROM project_invites WHERE token = ?", body.Token,
	).Scan(&inv.ID, &inv.ProjectID, &inv.Role, &inv.ExpiresAt, &usedAt)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusBadRequest, "invalid invite token")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	if usedAt.Valid {
		writeError(w, http.StatusGone, "invite already used")
		return
	}
	if time.Now().Unix() > inv.ExpiresAt {
		writeError(w, http.StatusGone, "invite expired")
		return
	}

	now := nowUnix()
	memberID := newID()

	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer tx.Rollback()

	// Add to project_members (ignore if already a member)
	if _, err := tx.ExecContext(r.Context(),
		"INSERT OR IGNORE INTO project_members (id, project_id, user_id, role, joined_at, username) VALUES (?, ?, ?, ?, ?, '')",
		memberID, inv.ProjectID, callerID, inv.Role, now,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	// Mark invite as used
	if _, err := tx.ExecContext(r.Context(),
		"UPDATE project_invites SET used_at = ?, used_by = ? WHERE id = ?",
		now, callerID, inv.ID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	writeJSON(w, http.StatusOK, ProjectMember{
		ID:        memberID,
		ProjectID: inv.ProjectID,
		UserID:    callerID,
		Role:      inv.Role,
		JoinedAt:  now,
	})
}

// ── Private helpers ───────────────────────────────────────────────────────────

// memberRole returns the project type and the caller's role within projectID.
func (h *ProjectsHandler) memberRole(r *http.Request, projectID, userID string) (projectType, role string, err error) {
	err = h.db.QueryRowContext(r.Context(), `
		SELECT p.type, pm.role
		FROM projects p
		JOIN project_members pm ON pm.project_id = p.id AND pm.user_id = ?
		WHERE p.id = ?
	`, userID, projectID).Scan(&projectType, &role)
	return
}

// fetchProject returns a single Project struct for the given caller.
func (h *ProjectsHandler) fetchProject(r *http.Request, projectID, callerID string) Project {
	var p Project
	_ = h.db.QueryRowContext(r.Context(), `
		SELECT p.id, p.name, p.type, p.created_by, p.created_at,
		       pm.role,
		       (SELECT COUNT(*) FROM project_members WHERE project_id = p.id)
		FROM projects p
		JOIN project_members pm ON pm.project_id = p.id AND pm.user_id = ?
		WHERE p.id = ?
	`, callerID, projectID).Scan(
		&p.ID, &p.Name, &p.Type, &p.CreatedBy, &p.CreatedAt, &p.MyRole, &p.MemberCount,
	)
	p.RoleLabels = roleLabelPair(p.Type)
	return p
}

// roleLabelPair returns the display label pair for a project type.
var roleLabelPair = func(projectType string) [2]string {
	switch projectType {
	case "campaign":
		return [2]string{"GM", "Player"}
	default:
		return [2]string{"Editor", "Member"}
	}
}

// ── Project Categories ────────────────────────────────────────────────────────

// ListProjectCategories returns the project's own categories plus global ones it has opted into.
func (h *ProjectsHandler) ListProjectCategories(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectId")
	if projectID == "global" {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	callerID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if _, _, err := h.memberRole(r, projectID, callerID); err != nil {
		writeError(w, http.StatusForbidden, "not a project member")
		return
	}

	rows, err := h.db.QueryContext(r.Context(), `
		SELECT id, name, icon, color, sort_order, project_id, created_at, updated_at, 'own' AS source
		FROM categories WHERE project_id = ?
		UNION ALL
		SELECT c.id, c.name, c.icon, c.color, c.sort_order, c.project_id, c.created_at, c.updated_at, 'global' AS source
		FROM categories c
		JOIN project_global_categories pgc ON pgc.category_id = c.id AND pgc.project_id = ?
		WHERE c.project_id = 'global'
		ORDER BY sort_order
	`, projectID, projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()

	cats := []Category{}
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.Name, &c.Icon, &c.Color, &c.SortOrder, &c.ProjectID, &c.CreatedAt, &c.UpdatedAt, &c.Source); err != nil {
			writeError(w, http.StatusInternalServerError, "scan error")
			return
		}
		cats = append(cats, c)
	}
	rows.Close() // close before fetching tags to avoid connection exhaustion

	for i := range cats {
		cats[i].DefaultTags = h.projectCategoryDefaultTags(r, cats[i].ID)
	}
	writeJSON(w, http.StatusOK, cats)
}

// CreateProjectCategory creates a category owned by this project.
func (h *ProjectsHandler) CreateProjectCategory(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectId")
	callerID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	projectType, myRole, err := h.memberRole(r, projectID, callerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if !isEditorRole(projectType, myRole) {
		writeError(w, http.StatusForbidden, "editor access required")
		return
	}

	var body struct {
		Name      string `json:"name"`
		Icon      string `json:"icon"`
		Color     string `json:"color"`
		SortOrder int    `json:"sortOrder"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}

	now := nowUnix()
	id := newID()
	if _, err := h.db.ExecContext(r.Context(),
		`INSERT INTO categories (id, name, icon, color, sort_order, project_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, body.Name, body.Icon, body.Color, body.SortOrder, projectID, now, now,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "insert failed")
		return
	}
	writeJSON(w, http.StatusCreated, Category{
		ID: id, Name: body.Name, Icon: body.Icon, Color: body.Color,
		SortOrder: body.SortOrder, ProjectID: projectID, Source: "own",
		DefaultTags: []CategoryDefaultTag{}, CreatedAt: now, UpdatedAt: now,
	})
}

// OptInGlobalCategory adds a global category to this project's category set.
func (h *ProjectsHandler) OptInGlobalCategory(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectId")
	catID := chi.URLParam(r, "catId")
	callerID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	projectType, myRole, err := h.memberRole(r, projectID, callerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if !isEditorRole(projectType, myRole) {
		writeError(w, http.StatusForbidden, "editor access required")
		return
	}

	// Verify the category exists and is global before adding to junction.
	var c Category
	if err := h.db.QueryRowContext(r.Context(),
		`SELECT id, name, icon, color, sort_order, project_id, created_at, updated_at
		 FROM categories WHERE id = ? AND project_id = 'global'`, catID,
	).Scan(&c.ID, &c.Name, &c.Icon, &c.Color, &c.SortOrder, &c.ProjectID, &c.CreatedAt, &c.UpdatedAt); err != nil {
		writeError(w, http.StatusNotFound, "global category not found")
		return
	}

	if _, err := h.db.ExecContext(r.Context(),
		`INSERT OR IGNORE INTO project_global_categories (project_id, category_id) VALUES (?, ?)`,
		projectID, catID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	c.Source = "global"
	c.DefaultTags = h.projectCategoryDefaultTags(r, c.ID)
	writeJSON(w, http.StatusCreated, c)
}

// RemoveProjectCategory removes an own category or opts out of a global one.
func (h *ProjectsHandler) RemoveProjectCategory(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectId")
	catID := chi.URLParam(r, "catId")
	callerID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	projectType, myRole, err := h.memberRole(r, projectID, callerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if !isEditorRole(projectType, myRole) {
		writeError(w, http.StatusForbidden, "editor access required")
		return
	}

	var storedProjectID string
	if err := h.db.QueryRowContext(r.Context(),
		`SELECT project_id FROM categories WHERE id = ?`, catID,
	).Scan(&storedProjectID); errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "category not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	if storedProjectID == projectID {
		// Own category — delete it entirely.
		if _, err := h.db.ExecContext(r.Context(), `DELETE FROM categories WHERE id = ?`, catID); err != nil {
			writeError(w, http.StatusInternalServerError, "db error")
			return
		}
	} else if storedProjectID == "global" {
		// Global category — remove opt-in from junction table only.
		if _, err := h.db.ExecContext(r.Context(),
			`DELETE FROM project_global_categories WHERE project_id = ? AND category_id = ?`,
			projectID, catID,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "db error")
			return
		}
	} else {
		// Belongs to a different project — not removable from here.
		writeError(w, http.StatusNotFound, "category not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// projectCategoryDefaultTags fetches default tags for a category.
func (h *ProjectsHandler) projectCategoryDefaultTags(r *http.Request, catID string) []CategoryDefaultTag {
	rows, err := h.db.QueryContext(r.Context(),
		`SELECT id, category_id, label, sort_order FROM category_default_tags WHERE category_id = ? ORDER BY sort_order`, catID)
	if err != nil {
		return []CategoryDefaultTag{}
	}
	defer rows.Close()
	tags := []CategoryDefaultTag{}
	for rows.Next() {
		var t CategoryDefaultTag
		_ = rows.Scan(&t.ID, &t.CategoryID, &t.Label, &t.SortOrder)
		tags = append(tags, t)
	}
	return tags
}

// ── Test helpers ──────────────────────────────────────────────────────────────

// NewProjectsForTest creates a handler that uses test context for caller ID.
func NewProjectsForTest(db *sql.DB) *ProjectsHandler {
	return &ProjectsHandler{db: db, jwtSecret: nil}
}

type testCallerKey struct{}

// WithTestCallerID injects a caller ID into a request context for tests.
func WithTestCallerID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, testCallerKey{}, id)
}
