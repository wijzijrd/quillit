package handler

import (
	"io"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"github.com/quillit/svc/internal/middleware"
)

// ContentEntriesHandler proxies entry-domain requests to the content
// service: listing (#126's delta-push follow-up), and — as of #48 — full
// CRUD plus render. content authorizes every request itself (per #44,
// same pattern as ContentFacetsHandler) — this just forwards the
// caller's session JWT.
type ContentEntriesHandler struct {
	contentURL string
}

func NewContentEntries(contentURL string) *ContentEntriesHandler {
	return &ContentEntriesHandler{contentURL: contentURL}
}

// ListEntries godoc
// @Summary      List a project's entries (proxied to content-svc)
// @Tags         content
// @Produce      json
// @Param        id  path  string  true  "Project ID"
// @Success      200  {array}  map[string]any
// @Failure      403  {object}  ErrorResponse
// @Router       /api/content/projects/{id}/entries [get]
func (h *ContentEntriesHandler) ListEntries(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h.proxy(w, r, http.MethodGet, "/content/projects/"+url.PathEscape(id)+"/entries", nil)
}

// GetEntry godoc
// @Summary      Get an entry, including its resolved body (proxied to content-svc)
// @Tags         content
// @Produce      json
// @Param        id  path  string  true  "Entry ID"
// @Success      200  {object}  map[string]any
// @Failure      404  {object}  ErrorResponse
// @Router       /api/content/entries/{id} [get]
func (h *ContentEntriesHandler) GetEntry(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h.proxy(w, r, http.MethodGet, "/content/entries/"+url.PathEscape(id), nil)
}

// UpdateEntry godoc
// @Summary      Update an entry (proxied to content-svc)
// @Tags         content
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "Entry ID"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  ErrorResponse
// @Router       /api/content/entries/{id} [patch]
func (h *ContentEntriesHandler) UpdateEntry(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h.proxy(w, r, http.MethodPatch, "/content/entries/"+url.PathEscape(id), r.Body)
}

// DeleteEntry godoc
// @Summary      Delete an entry (proxied to content-svc)
// @Tags         content
// @Produce      json
// @Param        id  path  string  true  "Entry ID"
// @Success      200  {object}  map[string]any
// @Router       /api/content/entries/{id} [delete]
func (h *ContentEntriesHandler) DeleteEntry(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h.proxy(w, r, http.MethodDelete, "/content/entries/"+url.PathEscape(id), nil)
}

// CreateEntry godoc
// @Summary      Create an entry in a project (proxied to content-svc)
// @Tags         content
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "Project ID"
// @Success      201  {object}  map[string]any
// @Failure      400  {object}  ErrorResponse
// @Router       /api/content/projects/{id}/entries [post]
func (h *ContentEntriesHandler) CreateEntry(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h.proxy(w, r, http.MethodPost, "/content/projects/"+url.PathEscape(id)+"/entries", r.Body)
}

// RenderEntry godoc
// @Summary      Render an entry to an HTML fragment (proxied to content-svc)
// @Description  view and card are mutually exclusive; query params pass through verbatim.
// @Tags         content
// @Produce      html
// @Param        id     path   string  true   "Entry ID"
// @Param        view   query  string  false  "dm (default) or player"
// @Param        card   query  string  false  "facet name"
// @Success      200    {string}  string
// @Failure      404    {object}  ErrorResponse
// @Router       /api/content/entries/{id}/render [get]
func (h *ContentEntriesHandler) RenderEntry(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	reqURL := h.contentURL + "/content/entries/" + url.PathEscape(id) + "/render"
	if r.URL.RawQuery != "" {
		reqURL += "?" + r.URL.RawQuery
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, reqURL, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if raw, ok := middleware.RawJWTFromContext(r.Context()); ok {
		req.Header.Set("Authorization", "Bearer "+raw)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "content service unavailable")
		return
	}
	defer resp.Body.Close()

	// Render's response is text/html, not JSON — the JSON-hardcoded proxy
	// below would corrupt it, same reasoning as #128's image proxy.
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// proxy forwards method+path (content-service-relative, e.g.
// "/content/entries/e1") to the content service, streaming body through
// if non-nil, forwarding the caller's raw session JWT as a bearer token,
// and copying the response status/body back verbatim as JSON — matches
// ContentFacetsHandler.proxy's exact shape (this package's established
// pattern for a JSON-only content proxy; RenderEntry above needs its own
// Content-Type-preserving path instead, since its response isn't JSON).
func (h *ContentEntriesHandler) proxy(w http.ResponseWriter, r *http.Request, method, path string, body io.Reader) {
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
		writeError(w, http.StatusBadGateway, "content service unavailable")
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBody)
}
