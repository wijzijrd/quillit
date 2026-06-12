package handler

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/quillit/svc/internal/middleware"
)

type AdminHandler struct {
	db        *sql.DB
	jwtSecret []byte
	authURL   string
}

func NewAdmin(db *sql.DB, jwtSecret, authURL string) *AdminHandler {
	return &AdminHandler{db: db, jwtSecret: []byte(jwtSecret), authURL: authURL}
}

// proxyToAuth forwards a request to auth-svc, injecting the caller's session JWT as Bearer.
func (h *AdminHandler) proxyToAuth(w http.ResponseWriter, r *http.Request, method, path string, body io.Reader) {
	raw, ok := middleware.RawJWTFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), method, h.authURL+path, body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	req.Header.Set("Authorization", "Bearer "+raw)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("auth service unavailable: %v", err))
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBody)
}

// ── User management (proxied to auth-svc) ────────────────────────────────────

// ListUsers godoc
// @Summary      List users (admin)
// @Description  Proxies to auth-svc. Supports ?q= search and ?active= filter.
// @Tags         admin
// @Produce      json
// @Param        q       query  string  false  "Search by email or username"
// @Param        active  query  string  false  "Filter: true or false"
// @Success      200  {array}   object
// @Failure      403  {object}  ErrorResponse
// @Router       /api/admin/users [get]
func (h *AdminHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	active := r.URL.Query().Get("active")
	path := fmt.Sprintf("/auth/users?q=%s&active=%s", q, active)
	h.proxyToAuth(w, r, http.MethodGet, path, nil)
}

// UpdateUser godoc
// @Summary      Update user (admin)
// @Description  Proxies to auth-svc. Supports { active: bool } to enable/disable an account.
// @Tags         admin
// @Accept       json
// @Produce      json
// @Param        id    path  string  true  "User ID"
// @Param        body  body  object  true  "{ active: bool }"
// @Success      200  {object}  object
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /api/admin/users/{id} [patch]
func (h *AdminHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h.proxyToAuth(w, r, http.MethodPatch, "/auth/users/"+id, r.Body)
}

// DeleteUser godoc
// @Summary      Delete user (admin)
// @Description  Proxies to auth-svc. Permanently removes the user account.
// @Tags         admin
// @Produce      json
// @Param        id  path  string  true  "User ID"
// @Success      204
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /api/admin/users/{id} [delete]
func (h *AdminHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h.proxyToAuth(w, r, http.MethodDelete, "/auth/users/"+id, nil)
}

// ── Project management ────────────────────────────────────────────────────────

// AdminProject is the project view returned to admins (includes all projects, not just theirs).
type AdminProject struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	CreatedBy   string    `json:"createdBy"`
	CreatedAt   int64     `json:"createdAt"`
	MemberCount int       `json:"memberCount"`
	RoleLabels  [2]string `json:"roleLabels"`
}

// ListProjects godoc
// @Summary      List all projects (admin)
// @Description  Returns all projects with member counts. Supports ?q= name search.
// @Tags         admin
// @Produce      json
// @Param        q  query  string  false  "Search by project name"
// @Success      200  {array}   AdminProject
// @Failure      403  {object}  ErrorResponse
// @Router       /api/admin/projects [get]
func (h *AdminHandler) ListProjects(w http.ResponseWriter, r *http.Request) {
	q := "%" + r.URL.Query().Get("q") + "%"

	rows, err := h.db.QueryContext(r.Context(), `
		SELECT p.id, p.name, p.type, p.created_by, p.created_at,
		       COUNT(pm.id) AS member_count
		FROM projects p
		LEFT JOIN project_members pm ON pm.project_id = p.id
		WHERE p.name LIKE ?
		GROUP BY p.id
		ORDER BY p.created_at DESC`,
		q,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()

	projects := []AdminProject{}
	for rows.Next() {
		var p AdminProject
		if err := rows.Scan(&p.ID, &p.Name, &p.Type, &p.CreatedBy, &p.CreatedAt, &p.MemberCount); err == nil {
			p.RoleLabels = roleLabelPair(p.Type)
			projects = append(projects, p)
		}
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, projects)
}

// ListProjectMembers godoc
// @Summary      List members of any project (admin)
// @Description  Returns all members for the given project, regardless of admin's own membership.
// @Tags         admin
// @Produce      json
// @Param        id  path  string  true  "Project ID"
// @Success      200  {array}   ProjectMember
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /api/admin/projects/{id}/members [get]
func (h *AdminHandler) ListProjectMembers(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	// Verify project exists
	var count int
	_ = h.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM projects WHERE id = ?`, projectID).Scan(&count)
	if count == 0 {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	rows, err := h.db.QueryContext(r.Context(),
		`SELECT id, project_id, user_id, role, joined_at, username FROM project_members WHERE project_id = ? ORDER BY joined_at ASC`,
		projectID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()

	members := []ProjectMember{}
	for rows.Next() {
		var m ProjectMember
		if err := rows.Scan(&m.ID, &m.ProjectID, &m.UserID, &m.Role, &m.JoinedAt, &m.Username); err == nil {
			members = append(members, m)
		}
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, members)
}
