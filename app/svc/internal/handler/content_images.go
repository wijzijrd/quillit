package handler

import (
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"github.com/quillit/svc/internal/middleware"
)

// ContentImagesHandler proxies entry-image GETs to the content service.
// The response body is binary (an image), so this can't reuse
// ContentFacetsHandler.proxy, which hardcodes a JSON response
// Content-Type — this handler forwards the upstream Content-Type
// verbatim instead, for both success and error responses alike. content
// authorizes the request itself (per #44, same as ContentFacetsHandler),
// so this just forwards the caller's session JWT.
type ContentImagesHandler struct {
	contentURL string
}

func NewContentImages(contentURL string) *ContentImagesHandler {
	return &ContentImagesHandler{contentURL: contentURL}
}

// GetImage godoc
// @Summary      Get an entry image (proxied to content-svc)
// @Tags         content
// @Produce      image/*
// @Param        id        path  string  true  "Entry ID"
// @Param        filename  path  string  true  "Image filename"
// @Success      200
// @Failure      404  {object}  ErrorResponse
// @Router       /api/content/entries/{id}/images/{filename} [get]
func (h *ContentImagesHandler) GetImage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	filename := chi.URLParam(r, "filename")

	reqURL := fmt.Sprintf("%s/content/entries/%s/images/%s", h.contentURL, url.PathEscape(id), url.PathEscape(filename))
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
		writeError(w, http.StatusBadGateway, fmt.Sprintf("content service unavailable: %v", err))
		return
	}
	defer resp.Body.Close()

	copyResponseHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// forwardedResponseHeaders is the allowlist of upstream response headers
// this proxy forwards verbatim. It's deliberately narrow — this is a
// proxy, not a generic passthrough, so it forwards what content-svc set
// rather than inventing header values of its own. Alongside the usual
// caching/representation headers, it includes the two anti-XSS headers
// content-svc's GetImage sets on every image response (X-Content-Type-
// Options, Content-Security-Policy): before this allowlist existed, only
// Content-Type was copied, which silently dropped those two and left a
// stored-XSS path open for any image (e.g. SVG) served through this
// same-origin proxy.
var forwardedResponseHeaders = []string{
	"Content-Type",
	"Content-Length",
	"Content-Disposition",
	"Cache-Control",
	"ETag",
	"X-Content-Type-Options",
	"Content-Security-Policy",
}

// copyResponseHeaders copies each header in forwardedResponseHeaders from
// src to dst, skipping any that are absent or empty — same pattern as the
// single-header check this replaced.
func copyResponseHeaders(dst, src http.Header) {
	for _, name := range forwardedResponseHeaders {
		if v := src.Get(name); v != "" {
			dst.Set(name, v)
		}
	}
}
