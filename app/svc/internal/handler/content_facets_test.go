package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/quillit/svc/internal/handler"
	"github.com/quillit/svc/internal/middleware"
)

// TestContentFacetsProxy_ForwardsCallerJWT is the #44 regression this
// package needs: content's facet endpoints now authenticate every request
// (see app/content/internal/handler/facets.go's requireCaller/
// requireProjectMember), so this proxy has to forward the caller's raw
// session JWT as a bearer token — before #44, content didn't check auth at
// all, so a proxy that forwarded nothing "worked" by accident. Without
// this, the facet-management UI (already merged, web-50-facet-management-ui)
// would 401 on every request post-#44.
func TestContentFacetsProxy_ForwardsCallerJWT(t *testing.T) {
	var gotAuth string
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer content.Close()

	h := handler.NewContentFacets(content.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/content/facets", nil)
	req = req.WithContext(middleware.WithRawJWTForTest(req.Context(), "the-raw-session-jwt"))
	rr := httptest.NewRecorder()
	h.ListGlobal(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if want := "Bearer the-raw-session-jwt"; gotAuth != want {
		t.Errorf("Authorization header forwarded to content = %q, want %q", gotAuth, want)
	}
}

// TestContentFacetsProxy_NoJWTInContextStillProxies confirms the proxy
// doesn't panic or otherwise misbehave when RawJWTFromContext has nothing
// to return (e.g. a future caller path that doesn't run RequireSession
// first) — it should just omit the header rather than fail, since content
// still applies its own 401 for that case.
func TestContentFacetsProxy_NoJWTInContextStillProxies(t *testing.T) {
	var gotAuth string
	var sawAuthHeader bool
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, sawAuthHeader = r.Header.Get("Authorization"), r.Header.Get("Authorization") != ""
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer content.Close()

	h := handler.NewContentFacets(content.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/content/facets", nil)
	rr := httptest.NewRecorder()
	h.ListGlobal(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if sawAuthHeader {
		t.Errorf("Authorization header = %q, want none when no JWT is in context", gotAuth)
	}
}
