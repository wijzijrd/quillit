package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5"
	"github.com/quillit/contentengine/parse"

	"github.com/quillit/content-svc/internal/authz"
	"github.com/quillit/content-svc/internal/db/sqlc"
)

// UnknownFacetError is returned when an entry body's :::card block names a
// facet outside the effective vocabulary (global facets ∪ this project's
// facets). Fail loud (docs/web-refactor-spec.md §4.3 golden rule 6): never
// guess or silently accept — name the bad facet and the full vocabulary so
// the caller can fix it.
type UnknownFacetError struct {
	Facet      string
	Vocabulary []string
}

func (e UnknownFacetError) Error() string {
	return fmt.Sprintf("unknown facet %q (effective vocabulary: %v)", e.Facet, e.Vocabulary)
}

// effectiveFacetVocabulary returns the union of the global facet vocabulary
// and projectID's own facets (docs/web-refactor-spec.md §4.3: "Effective
// vocabulary = global ∪ project"), as both a lookup set and a sorted slice
// for error messages.
func effectiveFacetVocabulary(ctx context.Context, q *sqlc.Queries, projectID string) (map[string]bool, []string, error) {
	list, err := q.EffectiveFacetVocabulary(ctx, projectID)
	if err != nil {
		return nil, nil, err
	}
	set := make(map[string]bool, len(list))
	for _, name := range list {
		set[name] = true
	}
	return set, list, nil
}

// validateFacets walks entry's entire block tree (including blocks nested
// inside a :::secret, matching linkindex.Extract's traversal) and returns
// UnknownFacetError for the first :::card block whose facet isn't in
// vocabulary.
func validateFacets(entry *parse.Entry, vocabulary map[string]bool, sortedVocabulary []string) error {
	return validateFacetBlocks(entry.Body, vocabulary, sortedVocabulary)
}

func validateFacetBlocks(blocks []parse.Block, vocabulary map[string]bool, sortedVocabulary []string) error {
	for _, b := range blocks {
		if b.Kind == parse.BlockCard && !vocabulary[b.Facet] {
			return UnknownFacetError{Facet: b.Facet, Vocabulary: sortedVocabulary}
		}
		if err := validateFacetBlocks(b.Blocks, vocabulary, sortedVocabulary); err != nil {
			return err
		}
	}
	return nil
}

// ── HTTP endpoints ──────────────────────────────────────────────────────────
//
// Facet vocabulary management (docs/web-refactor-spec.md §7.2, CLI-aligned
// per docs/cli-spec.md §7 "config add/rm --facet"): global facets and each
// project's extra_facets analogue (project_facets). Removal never touches
// entry bodies — entries still using a removed facet fail loud at their next
// save (validateFacets, above), matching CLI `config rm` semantics; the
// delete response carries a reminder rather than the endpoint scanning
// bodies for usage.

var kebabRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// isKebabName reports whether name is lowercase, digits, and hyphens only —
// docs/cli-spec.md §7: "Facet names: lowercase, digits, hyphens (kebab-case)
// — keeps directive parsing unambiguous."
func isKebabName(name string) bool {
	return kebabRe.MatchString(name)
}

type FacetsHandler struct {
	db        *sql.DB
	q         *sqlc.Queries
	jwtSecret []byte
	checker   authz.Checker
}

func NewFacets(db *sql.DB, jwtSecret string, checker authz.Checker) *FacetsHandler {
	return &FacetsHandler{db: db, q: sqlc.New(db), jwtSecret: []byte(jwtSecret), checker: checker}
}

type facetRequest struct {
	Name string `json:"name"`
}

type facetResponse struct {
	Name    string `json:"name"`
	Added   bool   `json:"added"`
	Message string `json:"message,omitempty"`
}

// ListGlobal godoc
// @Summary  List global facets
// @Description  Any authenticated caller — the global vocabulary isn't project-scoped, so there's no membership to check (see #44 PR notes).
// @Tags     facets
// @Produce  json
// @Success  200  {array}  string
// @Router   /content/facets [get]
func (h *FacetsHandler) ListGlobal(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireCaller(w, r, h.jwtSecret); !ok {
		return
	}
	names, err := h.q.ListGlobalFacetNames(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	if names == nil {
		names = []string{}
	}
	writeJSON(w, http.StatusOK, names)
}

// CreateGlobal godoc
// @Summary  Add a global facet
// @Description  Duplicate names (already in the global vocabulary) are a no-op, not an error.
// @Tags     facets
// @Accept   json
// @Produce  json
// @Success  200  {object}  facetResponse
// @Success  201  {object}  facetResponse
// @Failure  400  {object}  ErrorResponse
// @Router   /content/facets [post]
func (h *FacetsHandler) CreateGlobal(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireCaller(w, r, h.jwtSecret); !ok {
		return
	}
	var req facetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}
	if !isKebabName(req.Name) {
		writeError(w, http.StatusBadRequest, "facet name must be lowercase letters, digits, and hyphens (kebab-case)")
		return
	}

	n, err := h.q.InsertGlobalFacet(r.Context(), req.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	if n == 0 {
		writeJSON(w, http.StatusOK, facetResponse{Name: req.Name, Added: false, Message: "facet already exists in the global vocabulary"})
		return
	}
	writeJSON(w, http.StatusCreated, facetResponse{Name: req.Name, Added: true})
}

// DeleteGlobal godoc
// @Summary  Remove a global facet
// @Description  Doesn't touch entry bodies — entries still using the facet fail loud at their next save.
// @Tags     facets
// @Produce  json
// @Param    name  path  string  true  "Facet name"
// @Success  200   {object}  facetResponse
// @Router   /content/facets/{name} [delete]
func (h *FacetsHandler) DeleteGlobal(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireCaller(w, r, h.jwtSecret); !ok {
		return
	}
	name := chi.URLParam(r, "name")
	if err := h.q.DeleteGlobalFacet(r.Context(), name); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, facetResponse{
		Name:    name,
		Message: "removed from the global vocabulary — entry bodies are untouched; any entry still using this facet will fail validation at its next save or render",
	})
}

// ListEffectiveForProject godoc
// @Summary  List a project's effective facet vocabulary
// @Description  The union of global facets and this project's own facets — what entry-write validation and the editor's facet picker consume.
// @Tags     facets
// @Produce  json
// @Param    id   path  string  true  "Project ID"
// @Success  200  {array}  string
// @Router   /content/projects/{id}/facets [get]
func (h *FacetsHandler) ListEffectiveForProject(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	if _, ok := requireProjectMember(w, r, h.jwtSecret, h.checker, projectID); !ok {
		return
	}
	_, sorted, err := effectiveFacetVocabulary(r.Context(), h.q, projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	if sorted == nil {
		sorted = []string{}
	}
	writeJSON(w, http.StatusOK, sorted)
}

// CreateForProject godoc
// @Summary  Add a project facet
// @Description  A no-op, not an error, if name is already in the project's effective vocabulary (global or project-scoped).
// @Tags     facets
// @Accept   json
// @Produce  json
// @Param    id    path  string  true  "Project ID"
// @Success  200   {object}  facetResponse
// @Success  201   {object}  facetResponse
// @Failure  400   {object}  ErrorResponse
// @Router   /content/projects/{id}/facets [post]
func (h *FacetsHandler) CreateForProject(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	if _, ok := requireProjectMember(w, r, h.jwtSecret, h.checker, projectID); !ok {
		return
	}
	var req facetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}
	if !isKebabName(req.Name) {
		writeError(w, http.StatusBadRequest, "facet name must be lowercase letters, digits, and hyphens (kebab-case)")
		return
	}

	vocab, _, err := effectiveFacetVocabulary(r.Context(), h.q, projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	if vocab[req.Name] {
		writeJSON(w, http.StatusOK, facetResponse{Name: req.Name, Added: false, Message: "facet already in this project's effective vocabulary"})
		return
	}

	if err := h.q.InsertProjectFacet(r.Context(), sqlc.InsertProjectFacetParams{ProjectID: projectID, Name: req.Name}); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusCreated, facetResponse{Name: req.Name, Added: true})
}

// DeleteForProject godoc
// @Summary  Remove a project facet
// @Description  Doesn't touch entry bodies — entries still using the facet fail loud at their next save. Only removes from this project's own facets; a facet inherited from the global vocabulary is unaffected.
// @Tags     facets
// @Produce  json
// @Param    id    path  string  true  "Project ID"
// @Param    name  path  string  true  "Facet name"
// @Success  200   {object}  facetResponse
// @Router   /content/projects/{id}/facets/{name} [delete]
func (h *FacetsHandler) DeleteForProject(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	if _, ok := requireProjectMember(w, r, h.jwtSecret, h.checker, projectID); !ok {
		return
	}
	name := chi.URLParam(r, "name")
	if err := h.q.DeleteProjectFacet(r.Context(), sqlc.DeleteProjectFacetParams{ProjectID: projectID, Name: name}); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, facetResponse{
		Name:    name,
		Message: "removed from this project's facets — entry bodies are untouched; any entry still using this facet will fail validation at its next save or render",
	})
}
