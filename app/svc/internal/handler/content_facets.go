package handler

import (
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"github.com/quillit/svc/internal/middleware"
)

// ContentFacetsHandler proxies facet-vocabulary requests to the content
// service. Content owns the facets/project_facets tables and all validation
// (kebab-case check, effective-vocabulary union — see #38's
// app/content/internal/handler/facets.go). content is an internal-only
// service (docker-compose exposes no public port for it) reachable only
// from svc, but as of #44 it authenticates and authorizes every request
// per-caller anyway — the network boundary proves the request came from
// svc, not which end user svc is forwarding it for. So this proxy is no
// longer just "gate behind RequireSession and forward verbatim": it also
// has to forward the caller's own identity (the raw session JWT) as a
// bearer token, or every proxied call would 401 against content. svc is
// still the BFF and still gates these routes behind RequireSession first
// (same as AdminHandler.proxyToAuth in admin.go) — that's what makes
// RawJWTFromContext available to forward.
type ContentFacetsHandler struct {
	contentURL string
}

func NewContentFacets(contentURL string) *ContentFacetsHandler {
	return &ContentFacetsHandler{contentURL: contentURL}
}

// proxy forwards method+path (content-service-relative, e.g. "/content/facets")
// to the content service, streaming the request body through, forwarding
// the caller's session JWT as a bearer token (#44 — see type doc), and
// copying the response status/body back verbatim.
func (h *ContentFacetsHandler) proxy(w http.ResponseWriter, r *http.Request, method, path string, body io.Reader) {
	req, err := http.NewRequestWithContext(r.Context(), method, h.contentURL+path, body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if raw, ok := middleware.RawJWTFromContext(r.Context()); ok {
		req.Header.Set("Authorization", "Bearer "+raw)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("content service unavailable: %v", err))
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBody)
}

// ── Global facets ────────────────────────────────────────────────────────────

// ListGlobal godoc
// @Summary      List global facets
// @Description  Proxies to the content service.
// @Tags         facets
// @Produce      json
// @Success      200  {array}  string
// @Router       /api/content/facets [get]
func (h *ContentFacetsHandler) ListGlobal(w http.ResponseWriter, r *http.Request) {
	h.proxy(w, r, http.MethodGet, "/content/facets", nil)
}

// CreateGlobal godoc
// @Summary      Add a global facet
// @Description  Proxies to the content service. Body: { name: string } (kebab-case).
// @Tags         facets
// @Accept       json
// @Produce      json
// @Success      200  {object}  object
// @Success      201  {object}  object
// @Failure      400  {object}  ErrorResponse
// @Router       /api/content/facets [post]
func (h *ContentFacetsHandler) CreateGlobal(w http.ResponseWriter, r *http.Request) {
	h.proxy(w, r, http.MethodPost, "/content/facets", r.Body)
}

// DeleteGlobal godoc
// @Summary      Remove a global facet
// @Description  Proxies to the content service. Entry bodies are untouched — the response carries a reminder.
// @Tags         facets
// @Produce      json
// @Param        name  path  string  true  "Facet name"
// @Success      200   {object}  object
// @Router       /api/content/facets/{name} [delete]
func (h *ContentFacetsHandler) DeleteGlobal(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	h.proxy(w, r, http.MethodDelete, "/content/facets/"+url.PathEscape(name), nil)
}

// ── Project facets ───────────────────────────────────────────────────────────

// ListForProject godoc
// @Summary      List a project's effective facet vocabulary
// @Description  Proxies to the content service. Union of global facets and this project's own.
// @Tags         facets
// @Produce      json
// @Param        id  path  string  true  "Project ID"
// @Success      200  {array}  string
// @Router       /api/content/projects/{id}/facets [get]
func (h *ContentFacetsHandler) ListForProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h.proxy(w, r, http.MethodGet, "/content/projects/"+url.PathEscape(id)+"/facets", nil)
}

// CreateForProject godoc
// @Summary      Add a project-specific facet
// @Description  Proxies to the content service. Body: { name: string } (kebab-case).
// @Tags         facets
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "Project ID"
// @Success      200  {object}  object
// @Success      201  {object}  object
// @Failure      400  {object}  ErrorResponse
// @Router       /api/content/projects/{id}/facets [post]
func (h *ContentFacetsHandler) CreateForProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h.proxy(w, r, http.MethodPost, "/content/projects/"+url.PathEscape(id)+"/facets", r.Body)
}

// DeleteForProject godoc
// @Summary      Remove a project-specific facet
// @Description  Proxies to the content service. Only removes from this project's own facets; entry bodies are untouched — the response carries a reminder.
// @Tags         facets
// @Produce      json
// @Param        id    path  string  true  "Project ID"
// @Param        name  path  string  true  "Facet name"
// @Success      200   {object}  object
// @Router       /api/content/projects/{id}/facets/{name} [delete]
func (h *ContentFacetsHandler) DeleteForProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	name := chi.URLParam(r, "name")
	h.proxy(w, r, http.MethodDelete, "/content/projects/"+url.PathEscape(id)+"/facets/"+url.PathEscape(name), nil)
}
