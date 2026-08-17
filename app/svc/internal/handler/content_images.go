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

	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
