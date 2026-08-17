package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/quillit/svc/internal/handler"
	"github.com/quillit/svc/internal/middleware"
)

func TestContentEntriesProxy_ForwardsCallerJWT(t *testing.T) {
	var gotAuth, gotPath string
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"e1","slug":"tom","directoryPath":"characters/npcs","bodyHash":"abc"}]`))
	}))
	defer content.Close()

	h := handler.NewContentEntries(content.URL)
	router := chi.NewRouter()
	router.Get("/api/content/projects/{id}/entries", h.ListEntries)

	req := httptest.NewRequest(http.MethodGet, "/api/content/projects/proj-1/entries", nil)
	req = req.WithContext(middleware.WithRawJWTForTest(req.Context(), "the-raw-session-jwt"))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if want := "Bearer the-raw-session-jwt"; gotAuth != want {
		t.Errorf("Authorization header forwarded to content = %q, want %q", gotAuth, want)
	}
	if want := "/content/projects/proj-1/entries"; gotPath != want {
		t.Errorf("proxied path = %q, want %q", gotPath, want)
	}
	if body := rr.Body.String(); body == "" {
		t.Error("expected the content-svc response body to be forwarded")
	}
}

func TestContentEntriesProxy_NoJWTInContextStillProxies(t *testing.T) {
	var sawAuthHeader bool
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuthHeader = r.Header.Get("Authorization") != ""
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer content.Close()

	h := handler.NewContentEntries(content.URL)
	router := chi.NewRouter()
	router.Get("/api/content/projects/{id}/entries", h.ListEntries)

	req := httptest.NewRequest(http.MethodGet, "/api/content/projects/proj-1/entries", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if sawAuthHeader {
		t.Error("Authorization header should be omitted when no JWT is in context")
	}
}
