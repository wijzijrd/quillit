package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/quillit/svc/internal/handler"
	"github.com/quillit/svc/internal/middleware"
)

func withImageRouteParams(req *http.Request, id, filename string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	rctx.URLParams.Add("filename", filename)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestContentImagesProxy_ForwardsCallerJWTAndStreamsImage(t *testing.T) {
	var gotAuth, gotPath string
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("fake-png-bytes"))
	}))
	defer content.Close()

	h := handler.NewContentImages(content.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/content/entries/e1/images/portrait.png", nil)
	req = req.WithContext(middleware.WithRawJWTForTest(req.Context(), "the-raw-session-jwt"))
	req = withImageRouteParams(req, "e1", "portrait.png")
	rr := httptest.NewRecorder()
	h.GetImage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if want := "Bearer the-raw-session-jwt"; gotAuth != want {
		t.Errorf("Authorization header forwarded to content = %q, want %q", gotAuth, want)
	}
	if want := "/content/entries/e1/images/portrait.png"; gotPath != want {
		t.Errorf("proxied path = %q, want %q", gotPath, want)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", ct)
	}
	if body := rr.Body.String(); body != "fake-png-bytes" {
		t.Errorf("body = %q, want %q", body, "fake-png-bytes")
	}
}

func TestContentImagesProxy_NoJWTInContextStillProxies(t *testing.T) {
	var sawAuthHeader bool
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuthHeader = r.Header.Get("Authorization") != ""
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("fake-png-bytes"))
	}))
	defer content.Close()

	h := handler.NewContentImages(content.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/content/entries/e1/images/portrait.png", nil)
	req = withImageRouteParams(req, "e1", "portrait.png")
	rr := httptest.NewRecorder()
	h.GetImage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if sawAuthHeader {
		t.Error("Authorization header should be omitted when no JWT is in context")
	}
}

func TestContentImagesProxy_PassesThroughUpstreamErrorStatus(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}))
	defer content.Close()

	h := handler.NewContentImages(content.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/content/entries/e1/images/missing.png", nil)
	req = withImageRouteParams(req, "e1", "missing.png")
	rr := httptest.NewRecorder()
	h.GetImage(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rr.Code, http.StatusNotFound, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}
