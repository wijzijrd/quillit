package handler

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/quillit/svc/internal/middleware"
)

// ContentProxyHandler forwards content-domain requests that must be
// user-authenticated to content-svc, which is not reachable from outside
// the compose network. Membership is checked here — svc owns that data
// (content's own auth is soft until #44).
type ContentProxyHandler struct {
	db         *sql.DB
	jwtSecret  []byte
	contentURL string
	client     *http.Client
}

func NewContentProxy(db *sql.DB, jwtSecret, contentURL string) *ContentProxyHandler {
	return &ContentProxyHandler{
		db:         db,
		jwtSecret:  []byte(jwtSecret),
		contentURL: contentURL,
		// Tarball uploads are much slower than contentclient's 5s budget.
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

// ImportProject godoc
// @Summary      Import a CLI project tarball (proxied to content-svc)
// @Description  Requires session and membership in the target project. Body is a tar.gz; query params pass through (mode, onConflict, createFacets).
// @Tags         content
// @Accept       application/gzip
// @Produce      json
// @Param        id  path  string  true  "Project ID"
// @Success      200  {object}  map[string]any
// @Failure      403  {object}  ErrorResponse
// @Router       /api/content/projects/{id}/import [post]
func (h *ContentProxyHandler) ImportProject(w http.ResponseWriter, r *http.Request) {
	raw, ok := middleware.RawJWTFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	claims, err := middleware.ClaimsFromContext(r.Context(), h.jwtSecret)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	userID, _ := claims["sub"].(string)

	projectID := chi.URLParam(r, "id")
	var one int
	err = h.db.QueryRowContext(r.Context(),
		`SELECT 1 FROM project_members WHERE project_id = ? AND user_id = ?`, projectID, userID).Scan(&one)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusForbidden, "not a member of this project")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	url := fmt.Sprintf("%s/content/projects/%s/import", h.contentURL, projectID)
	if r.URL.RawQuery != "" {
		url += "?" + r.URL.RawQuery
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, url, r.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	req.Header.Set("Content-Type", r.Header.Get("Content-Type"))
	req.Header.Set("Authorization", "Bearer "+raw)

	resp, err := h.client.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("content service unavailable: %v", err))
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
