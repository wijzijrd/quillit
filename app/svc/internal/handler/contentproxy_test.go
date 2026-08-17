package handler_test

import (
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	_ "modernc.org/sqlite"

	"github.com/quillit/svc/internal/handler"
	"github.com/quillit/svc/internal/middleware"
)

var testJWTSecret = []byte("test-secret")

// setupContentProxyDB seeds a project (p1) and a member (u1), but callers of
// this helper add the project_members row themselves so both the
// member/non-member cases can share the same schema+seed setup.
func setupContentProxyDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	schema := `
	CREATE TABLE projects (
		id TEXT PRIMARY KEY, name TEXT NOT NULL, type TEXT NOT NULL DEFAULT 'campaign',
		created_by TEXT NOT NULL, created_at INTEGER NOT NULL
	);
	CREATE TABLE project_members (
		id TEXT PRIMARY KEY, project_id TEXT NOT NULL REFERENCES projects(id),
		user_id TEXT NOT NULL, role TEXT NOT NULL, joined_at INTEGER NOT NULL,
		username TEXT NOT NULL DEFAULT '',
		UNIQUE(project_id, user_id)
	);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	if _, err := db.Exec(`INSERT INTO projects VALUES ('p1','TestProject','campaign','u1',?)`, now); err != nil {
		t.Fatal(err)
	}
	return db
}

// contentProxyJWT mints a real session JWT the same shape RequireSession
// would produce, so ImportProject's use of middleware.RawJWTFromContext /
// middleware.ClaimsFromContext exercises real parsing rather than a bypass.
func contentProxyJWT(t *testing.T, userID string) string {
	t.Helper()
	claims := jwt.MapClaims{"sub": userID}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(testJWTSecret)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

// contentProxyRequest drives ImportProject through a real chi router (for
// {id} URL-param extraction) with the raw JWT injected into context the way
// middleware.RequireSession does, via the exported middleware.WithRawJWTForTest.
func contentProxyRequest(t *testing.T, h *handler.ContentProxyHandler, method, path string, body io.Reader, userID string) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	r.Post("/api/content/projects/{id}/import", h.ImportProject)

	req := httptest.NewRequest(method, path, body)
	if userID != "" {
		req = req.WithContext(middleware.WithRawJWTForTest(req.Context(), contentProxyJWT(t, userID)))
	}
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}

func TestContentProxyImport_RequiresMembership(t *testing.T) {
	var hit bool
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"applied":false}`))
	}))
	defer backend.Close()

	db := setupContentProxyDB(t) // u1 seeded as project creator but NOT a project_members row
	h := handler.NewContentProxy(db, string(testJWTSecret), backend.URL)

	rr := contentProxyRequest(t, h, http.MethodPost, "/api/content/projects/p1/import", strings.NewReader("x"), "u1")
	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rr.Code)
	}
	if hit {
		t.Error("backend was reached despite non-membership")
	}
}

func TestContentProxyImport_ProxiesForMember(t *testing.T) {
	var gotAuth, gotQuery, gotBody string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.RawQuery
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"applied":true}`))
	}))
	defer backend.Close()

	db := setupContentProxyDB(t)
	now := time.Now().Unix()
	if _, err := db.Exec(`INSERT INTO project_members VALUES ('m1','p1','u1','gm',?,'')`, now); err != nil {
		t.Fatal(err)
	}
	h := handler.NewContentProxy(db, string(testJWTSecret), backend.URL)

	rr := contentProxyRequest(t, h, http.MethodPost, "/api/content/projects/p1/import?mode=apply&onConflict=skip", strings.NewReader("TARBALL"), "u1")

	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"applied":true`) {
		t.Errorf("response = %d %s", rr.Code, rr.Body.String())
	}
	if !strings.HasPrefix(gotAuth, "Bearer ") {
		t.Errorf("Authorization = %q, want Bearer <session JWT>", gotAuth)
	}
	if gotQuery != "mode=apply&onConflict=skip" {
		t.Errorf("query = %q, not passed through", gotQuery)
	}
	if gotBody != "TARBALL" {
		t.Errorf("body = %q, not streamed through", gotBody)
	}
}
