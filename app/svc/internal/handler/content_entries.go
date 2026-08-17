package handler

import (
	"io"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"github.com/quillit/svc/internal/middleware"
)

// ContentEntriesHandler proxies a project's entry-list GET to the content
// service, for #126's delta-push CLI follow-up: quillit push --delta needs
// to see what's already on the server (path + body hash) before deciding
// what to pack. content authorizes the request itself (per #44, same
// pattern as ContentFacetsHandler) — this just forwards the caller's
// session JWT.
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
	h.proxy(w, r, "/content/projects/"+url.PathEscape(id)+"/entries")
}

// proxy forwards a GET to path (content-service-relative) to the content
// service, forwarding the caller's raw session JWT as a bearer token, and
// copying the response status/body/Content-Type back verbatim. A private
// copy of the same three-line pattern ContentFacetsHandler.proxy already
// implements — not worth extracting into a shared helper for one more
// call site, matching this codebase's existing per-handler duplication
// (ContentFacetsHandler and ContentProxyHandler each have their own too).
func (h *ContentEntriesHandler) proxy(w http.ResponseWriter, r *http.Request, path string) {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, h.contentURL+path, nil)
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

	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	} else {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
