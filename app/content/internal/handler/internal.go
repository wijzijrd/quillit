package handler

import (
	"database/sql"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/quillit/content-svc/internal/db/sqlc"
)

// InternalHandler serves routes reached only by other trusted services on
// the compose network (currently: svc reporting system events), never end
// users and never proxied through a per-user identity check the way every
// other handler in this package is (requireCaller/requireProjectMember —
// see helpers.go's "Cross-domain auth" section). That's a deliberate
// distinction, not an oversight: those checks answer "which end user is
// asking, and are they allowed to touch this project's data" — a question
// that only makes sense when svc is *forwarding* a real user's request.
// ProjectDeleted isn't that; it's svc telling content about something that
// already happened in svc's own domain, with no "which user" to check.
// content already trusts this network position (infra/docker-compose.yml
// gives content no exposed port at all — svc, or something on the same
// docker network, is the only thing that can ever reach it), so this route
// relies on that boundary alone, same as every content route did before #44.
type InternalHandler struct {
	db *sql.DB
	q  *sqlc.Queries
}

func NewInternal(db *sql.DB) *InternalHandler {
	return &InternalHandler{db: db, q: sqlc.New(db)}
}

// projectDeletedResponse reports how many entries this call orphaned —
// the "and-report" half of #44's orphan-and-report policy.
type projectDeletedResponse struct {
	ProjectID       string `json:"projectId"`
	EntriesOrphaned int    `json:"entriesOrphaned"`
}

// ProjectDeleted godoc
// @Summary      Notify content that a project was deleted in svc
// @Description  Orphans (stamps orphaned_at on) every not-yet-orphaned entry in the project rather than hard-deleting — see #44 PR notes for why: content has no FK to svc's projects table, so a mistaken project deletion doesn't have to cascade into irreversible entry/blob loss the way svc's own project-owned tables do. Idempotent — entries already orphaned by a prior call aren't recounted.
// @Tags         internal
// @Produce      json
// @Param        id  path  string  true  "Project ID"
// @Success      200  {object}  projectDeletedResponse
// @Router       /content/internal/projects/{id}/deleted [post]
func (h *InternalHandler) ProjectDeleted(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "missing project id")
		return
	}

	n, err := h.q.OrphanProjectEntries(r.Context(), sqlc.OrphanProjectEntriesParams{
		OrphanedAt: sql.NullInt64{Int64: nowUnix(), Valid: true},
		ProjectID:  projectID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	writeJSON(w, http.StatusOK, projectDeletedResponse{
		ProjectID:       projectID,
		EntriesOrphaned: int(n),
	})
}
