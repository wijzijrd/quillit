package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/quillit/svc/internal/db/sqlc"
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

// ContentNotifier is the subset of contentclient.Client's methods
// ProjectsHandler needs to tell content about a deleted project (#44) — an
// interface here (rather than depending on the concrete contentclient.Client
// type) lets handler tests substitute a fake, the same role BlobStore plays
// in content's own entries.go.
type ContentNotifier interface {
	NotifyProjectDeleted(ctx context.Context, projectID string) error
}

type ProjectsHandler struct {
	db              *sql.DB
	q               *sqlc.Queries
	jwtSecret       []byte
	contentNotifier ContentNotifier
}

func NewProjects(db *sql.DB, jwtSecret string, contentNotifier ContentNotifier) *ProjectsHandler {
	return &ProjectsHandler{db: db, q: sqlc.New(db), jwtSecret: []byte(jwtSecret), contentNotifier: contentNotifier}
}

// callerID extracts the user ID (sub) from the JWT stored in request context.
// In tests, a caller ID can be injected directly via WithTestCallerID.
// (Implementation shared with entries/annotations — see helpers.go.)
func (h *ProjectsHandler) callerID(r *http.Request) (string, bool) {
	return callerIDFromRequest(r, h.jwtSecret)
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
	Type       string    `json:"type"`
	Label      string    `json:"label"`
	RoleLabels [2]string `json:"roleLabels"`
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

	rows, err := h.q.ListProjectsForUser(r.Context(), callerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	projects := []Project{}
	for _, row := range rows {
		p := Project{
			ID:          row.ID,
			Name:        row.Name,
			Type:        row.Type,
			CreatedBy:   row.CreatedBy,
			CreatedAt:   row.CreatedAt,
			MyRole:      row.Role,
			MemberCount: int(row.MemberCount),
			Live:        row.Live,
		}
		p.RoleLabels = roleLabelPair(p.Type)
		projects = append(projects, p)
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
	qtx := h.q.WithTx(tx)

	if err := qtx.InsertProject(r.Context(), sqlc.InsertProjectParams{
		ID:        projectID,
		Name:      body.Name,
		Type:      body.Type,
		CreatedBy: callerID,
		CreatedAt: now,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	if err := qtx.InsertProjectMember(r.Context(), sqlc.InsertProjectMemberParams{
		ID:        memberID,
		ProjectID: projectID,
		UserID:    callerID,
		Role:      editorRole,
		JoinedAt:  now,
	}); err != nil {
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

	if err := h.q.UpdateProjectName(r.Context(), sqlc.UpdateProjectNameParams{
		Name: *body.Name,
		ID:   id,
	}); err != nil {
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

	createdBy, err := h.q.GetProjectCreatedBy(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
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

	if err := h.q.DeleteProject(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	// #44: tell content this project is gone so it can apply its own
	// deletion policy to that project's entries (orphan-and-report, not
	// hard-delete — see app/content/internal/handler/internal.go). This is
	// best-effort and non-blocking: svc's own project row (and everything
	// that cascades from it via ON DELETE CASCADE — project_members,
	// project_invites, game_sessions, chat_messages) is already gone by
	// this point, so a transient content outage shouldn't turn an
	// otherwise-successful deletion into a user-facing error — the client
	// has no useful retry here (retrying DELETE on an already-deleted
	// project just 404s), and content's policy is designed to tolerate
	// finding out late (it just orphans on next contact — there's no
	// separate "am I already deleted" state to race).
	if h.contentNotifier != nil {
		notifyCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if err := h.contentNotifier.NotifyProjectDeleted(notifyCtx, id); err != nil {
			log.Printf("projects: notify content of deleted project %s failed (entries will stay live in content until the next successful notification): %v", id, err)
		}
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

	rows, err := h.q.ListProjectMembers(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	members := []ProjectMember{}
	for _, row := range rows {
		members = append(members, ProjectMember{
			ID:        row.ID,
			ProjectID: row.ProjectID,
			UserID:    row.UserID,
			Username:  row.Username,
			Role:      row.Role,
			JoinedAt:  row.JoinedAt,
		})
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
	if err := h.q.AddProjectMember(r.Context(), sqlc.AddProjectMemberParams{
		ID:        m.ID,
		ProjectID: m.ProjectID,
		UserID:    m.UserID,
		Role:      m.Role,
		JoinedAt:  m.JoinedAt,
		Username:  m.Username,
	}); err != nil {
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

	if err := h.q.DeleteProjectMember(r.Context(), sqlc.DeleteProjectMemberParams{
		ProjectID: id,
		UserID:    targetUserID,
	}); err != nil {
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
	if err := h.q.InsertProjectInvite(r.Context(), sqlc.InsertProjectInviteParams{
		ID:        inv.ID,
		Token:     inv.Token,
		ProjectID: inv.ProjectID,
		Role:      inv.Role,
		CreatedBy: inv.CreatedBy,
		ExpiresAt: inv.ExpiresAt,
	}); err != nil {
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

	if err := h.q.DeleteProjectInvite(r.Context(), sqlc.DeleteProjectInviteParams{
		Token:     token,
		ProjectID: id,
	}); err != nil {
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
	invRow, err := h.q.GetProjectInviteByToken(r.Context(), body.Token)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusBadRequest, "invalid invite token")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	inv := ProjectInvite{
		ID:        invRow.ID,
		ProjectID: invRow.ProjectID,
		Role:      invRow.Role,
		ExpiresAt: invRow.ExpiresAt,
	}
	if invRow.UsedAt.Valid {
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
	qtx := h.q.WithTx(tx)

	// Add to project_members (ignore if already a member)
	if err := qtx.JoinProjectAddMember(r.Context(), sqlc.JoinProjectAddMemberParams{
		ID:        memberID,
		ProjectID: inv.ProjectID,
		UserID:    callerID,
		Role:      inv.Role,
		JoinedAt:  now,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	// Mark invite as used
	if err := qtx.MarkProjectInviteUsed(r.Context(), sqlc.MarkProjectInviteUsedParams{
		UsedAt: sql.NullInt64{Int64: now, Valid: true},
		UsedBy: sql.NullString{String: callerID, Valid: true},
		ID:     inv.ID,
	}); err != nil {
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

// ── Internal (#44) ───────────────────────────────────────────────────────────

// ── Private helpers ───────────────────────────────────────────────────────────

// MemberRole returns the project type and userID's role within projectID.
// Exported (package-level, taking ctx/q directly rather than a
// *ProjectsHandler/*http.Request) so internal/rpc.SvcInternalServer.
// CheckMembership — the connectrpc replacement for the old HTTP-only
// InternalMembership route, called by app/content/internal/authz.SvcChecker
// — can reuse the exact same query without needing an *http.Request.
// ProjectsHandler.memberRole (below) delegates to this rather than
// duplicating the query, so every one of its existing callers in this file
// is unaffected.
// sql.ErrNoRows means either the project doesn't exist or userID isn't in
// it — callers deliberately don't need to tell those apart (both mean
// "reject").
func MemberRole(ctx context.Context, q *sqlc.Queries, projectID, userID string) (projectType, role string, err error) {
	row, err := q.GetMemberRole(ctx, sqlc.GetMemberRoleParams{
		UserID: userID,
		ID:     projectID,
	})
	if err != nil {
		return "", "", err
	}
	return row.Type, row.Role, nil
}

// memberRole returns the project type and the caller's role within projectID.
func (h *ProjectsHandler) memberRole(r *http.Request, projectID, userID string) (projectType, role string, err error) {
	return MemberRole(r.Context(), h.q, projectID, userID)
}

// fetchProject returns a single Project struct for the given caller.
func (h *ProjectsHandler) fetchProject(r *http.Request, projectID, callerID string) Project {
	row, _ := h.q.GetProjectForMember(r.Context(), sqlc.GetProjectForMemberParams{
		UserID: callerID,
		ID:     projectID,
	})
	p := Project{
		ID:          row.ID,
		Name:        row.Name,
		Type:        row.Type,
		CreatedBy:   row.CreatedBy,
		CreatedAt:   row.CreatedAt,
		MyRole:      row.Role,
		MemberCount: int(row.MemberCount),
	}
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

// ── Test helpers ──────────────────────────────────────────────────────────────

// NewProjectsForTest creates a handler that uses test context for caller
// ID, with no content notification wired up (Delete silently skips it —
// see the nil check in Delete). Tests that need to assert on the
// notification itself construct a ProjectsHandler directly with a fake
// ContentNotifier instead (see projects_test.go).
func NewProjectsForTest(db *sql.DB) *ProjectsHandler {
	return &ProjectsHandler{db: db, q: sqlc.New(db), jwtSecret: nil}
}
