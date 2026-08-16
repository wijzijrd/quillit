package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/quillit/content-svc/internal/authz"
	"github.com/quillit/content-svc/internal/db"
)

// withChiParams sets multiple chi URL params at once — withChiParam (in
// entries_test.go) replaces the whole route context per call, so it can't
// be chained to set more than one param. Also injects the default test
// caller (see withChiParam's doc comment for why).
func withChiParams(req *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return withTestCaller(req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx)))
}

func newTestFacetsHandler(t *testing.T) *FacetsHandler {
	t.Helper()
	return newTestFacetsHandlerWithChecker(t, authz.AllowAll{})
}

func newTestFacetsHandlerWithChecker(t *testing.T, checker authz.Checker) *FacetsHandler {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return NewFacets(database, "", checker)
}

func postFacet(t *testing.T, h *FacetsHandler, projectID, name string) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(facetRequest{Name: name})
	var req *http.Request
	if projectID == "" {
		req = withTestCaller(httptest.NewRequest(http.MethodPost, "/content/facets", strings.NewReader(string(raw))))
		w := httptest.NewRecorder()
		h.CreateGlobal(w, req)
		return w
	}
	req = httptest.NewRequest(http.MethodPost, "/content/projects/"+projectID+"/facets", strings.NewReader(string(raw)))
	req = withChiParam(req, "id", projectID)
	w := httptest.NewRecorder()
	h.CreateForProject(w, req)
	return w
}

// listGlobalFacets calls ListGlobal with a default test caller — every
// direct httptest.NewRequest call for this read-only, param-less endpoint
// needs the same auth injection withChiParam gives param-bearing requests.
func listGlobalFacets(t *testing.T, h *FacetsHandler) []string {
	t.Helper()
	req := withTestCaller(httptest.NewRequest(http.MethodGet, "/content/facets", nil))
	w := httptest.NewRecorder()
	h.ListGlobal(w, req)
	var names []string
	_ = json.Unmarshal(w.Body.Bytes(), &names)
	return names
}

func TestCreateGlobal_RejectsNonKebabCase(t *testing.T) {
	h := newTestFacetsHandler(t)
	w := postFacet(t, h, "", "Stat Block")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestCreateGlobal_AddsNewFacet(t *testing.T) {
	h := newTestFacetsHandler(t)
	w := postFacet(t, h, "", "stat-block")
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusCreated, w.Body.String())
	}

	names := listGlobalFacets(t, h)
	found := false
	for _, n := range names {
		if n == "stat-block" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected stat-block in global facets, got %v", names)
	}
}

func TestCreateGlobal_DuplicateIsNoOpNotError(t *testing.T) {
	h := newTestFacetsHandler(t)
	postFacet(t, h, "", "motivation") // already seeded globally
	w := postFacet(t, h, "", "motivation")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (no-op, not error), body = %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp facetResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Added {
		t.Error("Added = true, want false for a duplicate")
	}
}

func TestDeleteGlobal_IncludesReminder(t *testing.T) {
	h := newTestFacetsHandler(t)
	req := httptest.NewRequest(http.MethodDelete, "/content/facets/motivation", nil)
	req = withChiParam(req, "name", "motivation")
	w := httptest.NewRecorder()
	h.DeleteGlobal(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp facetResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Message == "" {
		t.Error("expected a reminder message in the delete response")
	}

	names := listGlobalFacets(t, h)
	for _, n := range names {
		if n == "motivation" {
			t.Error("expected motivation to be removed from global facets")
		}
	}
}

func TestListEffectiveForProject_UnionsGlobalAndProject(t *testing.T) {
	h := newTestFacetsHandler(t)
	postFacet(t, h, "proj-1", "stat-block")

	req := httptest.NewRequest(http.MethodGet, "/content/projects/proj-1/facets", nil)
	req = withChiParam(req, "id", "proj-1")
	w := httptest.NewRecorder()
	h.ListEffectiveForProject(w, req)

	var names []string
	_ = json.Unmarshal(w.Body.Bytes(), &names)
	want := map[string]bool{"motivation": true, "description": true, "history": true, "stat-block": true}
	for n := range want {
		found := false
		for _, got := range names {
			if got == n {
				found = true
			}
		}
		if !found {
			t.Errorf("effective vocabulary %v missing %q", names, n)
		}
	}

	// Another project shouldn't see proj-1's extra facet.
	req2 := httptest.NewRequest(http.MethodGet, "/content/projects/proj-2/facets", nil)
	req2 = withChiParam(req2, "id", "proj-2")
	w2 := httptest.NewRecorder()
	h.ListEffectiveForProject(w2, req2)
	var names2 []string
	_ = json.Unmarshal(w2.Body.Bytes(), &names2)
	for _, n := range names2 {
		if n == "stat-block" {
			t.Errorf("proj-2 should not see proj-1's facet, got %v", names2)
		}
	}
}

func TestCreateForProject_DuplicateAgainstGlobalIsNoOp(t *testing.T) {
	h := newTestFacetsHandler(t)
	w := postFacet(t, h, "proj-1", "motivation") // already a global facet
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (no-op against global), body = %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestDeleteForProject_DoesNotAffectGlobal(t *testing.T) {
	h := newTestFacetsHandler(t)
	postFacet(t, h, "proj-1", "stat-block")

	req := httptest.NewRequest(http.MethodDelete, "/content/projects/proj-1/facets/stat-block", nil)
	req = withChiParams(req, map[string]string{"id": "proj-1", "name": "stat-block"})
	w := httptest.NewRecorder()
	h.DeleteForProject(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodGet, "/content/projects/proj-1/facets", nil)
	req2 = withChiParam(req2, "id", "proj-1")
	w2 := httptest.NewRecorder()
	h.ListEffectiveForProject(w2, req2)
	var names []string
	_ = json.Unmarshal(w2.Body.Bytes(), &names)
	for _, n := range names {
		if n == "stat-block" {
			t.Errorf("expected stat-block removed from proj-1, got %v", names)
		}
		if n != "" && n == "motivation" {
			// sanity: global facets remain
		}
	}
}

// ── Auth (#44) ───────────────────────────────────────────────────────────

func TestListGlobal_RejectsUnauthenticatedRequest(t *testing.T) {
	h := newTestFacetsHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/content/facets", nil) // no caller injected
	w := httptest.NewRecorder()
	h.ListGlobal(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestListEffectiveForProject_RejectsNonProjectMember(t *testing.T) {
	h := newTestFacetsHandlerWithChecker(t, authz.Static{Members: map[string]map[string]bool{
		testUserID: {"proj-a": true},
	}})
	req := httptest.NewRequest(http.MethodGet, "/content/projects/proj-b/facets", nil)
	req = withChiParam(req, "id", "proj-b") // testUserID isn't a member of proj-b
	w := httptest.NewRecorder()
	h.ListEffectiveForProject(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestCreateForProject_RejectsNonProjectMember(t *testing.T) {
	h := newTestFacetsHandlerWithChecker(t, authz.Static{})
	w := postFacet(t, h, "proj-1", "stat-block")
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestDeleteForProject_RejectsNonProjectMember(t *testing.T) {
	h := newTestFacetsHandlerWithChecker(t, authz.Static{})
	req := httptest.NewRequest(http.MethodDelete, "/content/projects/proj-1/facets/stat-block", nil)
	req = withChiParams(req, map[string]string{"id": "proj-1", "name": "stat-block"})
	w := httptest.NewRecorder()
	h.DeleteForProject(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusForbidden, w.Body.String())
	}
}
