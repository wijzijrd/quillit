package handler_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestContentEntriesProxy_PassesThroughNon200StatusAndContentType(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"not a member"}`))
	}))
	defer content.Close()

	h := handler.NewContentEntries(content.URL)
	router := chi.NewRouter()
	router.Get("/api/content/projects/{id}/entries", h.ListEntries)

	req := httptest.NewRequest(http.MethodGet, "/api/content/projects/proj-1/entries", nil)
	req = req.WithContext(middleware.WithRawJWTForTest(req.Context(), "some-jwt"))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusForbidden)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
	if body := rr.Body.String(); body != `{"error":"not a member"}` {
		t.Errorf("body = %q, want the content-svc response body to be forwarded", body)
	}
}

func TestContentEntriesProxy_GetEntry(t *testing.T) {
	var gotPath, gotMethod string
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"e1","title":"Tom"}`))
	}))
	defer content.Close()

	h := handler.NewContentEntries(content.URL)
	router := chi.NewRouter()
	router.Get("/api/content/entries/{id}", h.GetEntry)

	req := httptest.NewRequest(http.MethodGet, "/api/content/entries/e1", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if gotMethod != http.MethodGet || gotPath != "/content/entries/e1" {
		t.Errorf("proxied %s %s, want GET /content/entries/e1", gotMethod, gotPath)
	}
}

func TestContentEntriesProxy_UpdateEntry_ForwardsBody(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"e1"}`))
	}))
	defer content.Close()

	h := handler.NewContentEntries(content.URL)
	router := chi.NewRouter()
	router.Patch("/api/content/entries/{id}", h.UpdateEntry)

	req := httptest.NewRequest(http.MethodPatch, "/api/content/entries/e1", strings.NewReader(`{"body":"new body"}`))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if gotMethod != http.MethodPatch || gotPath != "/content/entries/e1" {
		t.Errorf("proxied %s %s, want PATCH /content/entries/e1", gotMethod, gotPath)
	}
	if gotBody != `{"body":"new body"}` {
		t.Errorf("proxied body = %q, want the original request body forwarded verbatim", gotBody)
	}
}

func TestContentEntriesProxy_DeleteEntry(t *testing.T) {
	var gotMethod, gotPath string
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer content.Close()

	h := handler.NewContentEntries(content.URL)
	router := chi.NewRouter()
	router.Delete("/api/content/entries/{id}", h.DeleteEntry)

	req := httptest.NewRequest(http.MethodDelete, "/api/content/entries/e1", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if gotMethod != http.MethodDelete || gotPath != "/content/entries/e1" {
		t.Errorf("proxied %s %s, want DELETE /content/entries/e1", gotMethod, gotPath)
	}
}

func TestContentEntriesProxy_CreateEntry_ForwardsBody(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"e2"}`))
	}))
	defer content.Close()

	h := handler.NewContentEntries(content.URL)
	router := chi.NewRouter()
	router.Post("/api/content/projects/{id}/entries", h.CreateEntry)

	req := httptest.NewRequest(http.MethodPost, "/api/content/projects/proj-1/entries", strings.NewReader(`{"slug":"tom","directoryPath":"","body":"---\nname: Tom\n---\n"}`))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if gotMethod != http.MethodPost || gotPath != "/content/projects/proj-1/entries" {
		t.Errorf("proxied %s %s, want POST /content/projects/proj-1/entries", gotMethod, gotPath)
	}
	if gotBody == "" {
		t.Error("expected the create request body to be forwarded")
	}
}

func TestContentEntriesProxy_RenderEntry_ForwardsQueryAndHTMLContentType(t *testing.T) {
	var gotPath, gotQuery string
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<p>rendered</p>`))
	}))
	defer content.Close()

	h := handler.NewContentEntries(content.URL)
	router := chi.NewRouter()
	router.Get("/api/content/entries/{id}/render", h.RenderEntry)

	req := httptest.NewRequest(http.MethodGet, "/api/content/entries/e1/render?view=player", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if gotPath != "/content/entries/e1/render" {
		t.Errorf("proxied path = %q, want /content/entries/e1/render", gotPath)
	}
	if gotQuery != "view=player" {
		t.Errorf("proxied query = %q, want view=player", gotQuery)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/html; charset=utf-8", ct)
	}
	if body := rr.Body.String(); body != "<p>rendered</p>" {
		t.Errorf("body = %q, want the rendered HTML forwarded verbatim", body)
	}
}

func TestContentEntriesProxy_RenderEntry_PassesThroughNon200Status(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":"unknown facet"}`))
	}))
	defer content.Close()

	h := handler.NewContentEntries(content.URL)
	router := chi.NewRouter()
	router.Get("/api/content/entries/{id}/render", h.RenderEntry)

	req := httptest.NewRequest(http.MethodGet, "/api/content/entries/e1/render?card=motivation", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}
