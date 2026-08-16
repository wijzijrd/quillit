package handler_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/quillit/svc/internal/handler"
)

func setupProjectsDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		t.Fatal(err)
	}
	schema := `
	CREATE TABLE projects (
		id TEXT PRIMARY KEY, name TEXT NOT NULL, type TEXT NOT NULL DEFAULT 'campaign',
		created_by TEXT NOT NULL, created_at INTEGER NOT NULL
	);
	CREATE TABLE project_members (
		id TEXT PRIMARY KEY, project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
		user_id TEXT NOT NULL, role TEXT NOT NULL, joined_at INTEGER NOT NULL,
		username TEXT NOT NULL DEFAULT '',
		UNIQUE(project_id, user_id)
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	return db
}

// fakeContentNotifier is a test double for handler.ContentNotifier —
// records every call so tests can assert Delete actually placed the
// cross-domain notification (#44), and can be configured to fail so tests
// can assert that failure doesn't block the project deletion itself.
type fakeContentNotifier struct {
	mu       sync.Mutex
	notified []string
	err      error
}

func (f *fakeContentNotifier) NotifyProjectDeleted(_ context.Context, projectID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.notified = append(f.notified, projectID)
	return f.err
}

func (f *fakeContentNotifier) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.notified))
	copy(out, f.notified)
	return out
}

func seedProject(t *testing.T, db *sql.DB, id, createdBy string) {
	t.Helper()
	now := time.Now().Unix()
	if _, err := db.Exec(`INSERT INTO projects (id, name, type, created_by, created_at) VALUES (?, ?, 'campaign', ?, ?)`,
		id, "Test Project", createdBy, now); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO project_members (id, project_id, user_id, role, joined_at) VALUES (?, ?, ?, 'gm', ?)`,
		id+"-m-"+createdBy, id, createdBy, now); err != nil {
		t.Fatalf("seed member: %v", err)
	}
}

func projectsRouter(h *handler.ProjectsHandler) chi.Router {
	r := chi.NewRouter()
	r.Delete("/api/projects/{id}", h.Delete)
	r.Get("/internal/projects/{id}/members/{userId}", h.InternalMembership)
	return r
}

func deleteProject(t *testing.T, r chi.Router, projectID, callerID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, "/api/projects/"+projectID, nil)
	if callerID != "" {
		req = req.WithContext(handler.WithTestCallerID(req.Context(), callerID))
	}
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}

// ── Delete + cross-domain notification (#44) ────────────────────────────────

func TestDelete_CreatorSucceedsAndNotifiesContent(t *testing.T) {
	db := setupProjectsDB(t)
	seedProject(t, db, "proj-1", "user-1")
	notifier := &fakeContentNotifier{}
	h := handler.NewProjects(db, "test-secret", notifier)
	r := projectsRouter(h)

	rr := deleteProject(t, r, "proj-1", "user-1")
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rr.Code, http.StatusNoContent, rr.Body.String())
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM projects WHERE id = ?`, "proj-1").Scan(&count); err != nil {
		t.Fatalf("count projects: %v", err)
	}
	if count != 0 {
		t.Errorf("expected project row to be deleted, count = %d", count)
	}

	if got := notifier.calls(); len(got) != 1 || got[0] != "proj-1" {
		t.Errorf("content notifications = %v, want exactly one call for proj-1", got)
	}
}

func TestDelete_NonCreatorForbiddenAndDoesNotNotifyContent(t *testing.T) {
	db := setupProjectsDB(t)
	seedProject(t, db, "proj-1", "user-1")
	notifier := &fakeContentNotifier{}
	h := handler.NewProjects(db, "test-secret", notifier)
	r := projectsRouter(h)

	rr := deleteProject(t, r, "proj-1", "user-2")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", rr.Code, http.StatusForbidden, rr.Body.String())
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM projects WHERE id = ?`, "proj-1").Scan(&count); err != nil {
		t.Fatalf("count projects: %v", err)
	}
	if count != 1 {
		t.Error("project was deleted despite a non-creator caller")
	}
	if got := notifier.calls(); len(got) != 0 {
		t.Errorf("content notifications = %v, want none for a forbidden delete", got)
	}
}

// TestDelete_ContentNotifyFailureDoesNotBlockDeletion is the resilience
// half of #44's acceptance criteria: a transient content outage must not
// turn an otherwise-successful project deletion into a user-facing error
// (svc's own project row — and everything that cascades from it — is
// already gone by the time the notification is attempted).
func TestDelete_ContentNotifyFailureDoesNotBlockDeletion(t *testing.T) {
	db := setupProjectsDB(t)
	seedProject(t, db, "proj-1", "user-1")
	notifier := &fakeContentNotifier{err: context.DeadlineExceeded}
	h := handler.NewProjects(db, "test-secret", notifier)
	r := projectsRouter(h)

	rr := deleteProject(t, r, "proj-1", "user-1")
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rr.Code, http.StatusNoContent, rr.Body.String())
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM projects WHERE id = ?`, "proj-1").Scan(&count); err != nil {
		t.Fatalf("count projects: %v", err)
	}
	if count != 0 {
		t.Error("expected project row to be deleted even though content notification failed")
	}
	if got := notifier.calls(); len(got) != 1 {
		t.Errorf("content notifications = %v, want exactly one attempted call", got)
	}
}

// TestDelete_NilNotifierIsSkippedSilently confirms a ProjectsHandler wired
// with no notifier at all (e.g. NewProjectsForTest, or a future deployment
// that hasn't set CONTENT_SERVICE_URL) still deletes successfully — Delete
// must guard the nil case rather than panic.
func TestDelete_NilNotifierIsSkippedSilently(t *testing.T) {
	db := setupProjectsDB(t)
	seedProject(t, db, "proj-1", "user-1")
	h := handler.NewProjects(db, "test-secret", nil)
	r := projectsRouter(h)

	rr := deleteProject(t, r, "proj-1", "user-1")
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rr.Code, http.StatusNoContent, rr.Body.String())
	}
}

// ── InternalMembership (#44) ─────────────────────────────────────────────────

func internalMembershipRequest(t *testing.T, r chi.Router, projectID, userID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/internal/projects/"+projectID+"/members/"+userID, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}

func TestInternalMembership_MemberReturns200WithRole(t *testing.T) {
	db := setupProjectsDB(t)
	seedProject(t, db, "proj-1", "user-1")
	h := handler.NewProjects(db, "test-secret", nil)
	r := projectsRouter(h)

	rr := internalMembershipRequest(t, r, "proj-1", "user-1")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	var resp struct {
		ProjectID   string `json:"projectId"`
		UserID      string `json:"userId"`
		Role        string `json:"role"`
		ProjectType string `json:"projectType"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ProjectID != "proj-1" || resp.UserID != "user-1" || resp.Role != "gm" || resp.ProjectType != "campaign" {
		t.Errorf("response = %+v, want proj-1/user-1/gm/campaign", resp)
	}
}

func TestInternalMembership_NonMemberReturns404(t *testing.T) {
	db := setupProjectsDB(t)
	seedProject(t, db, "proj-1", "user-1")
	h := handler.NewProjects(db, "test-secret", nil)
	r := projectsRouter(h)

	rr := internalMembershipRequest(t, r, "proj-1", "user-2")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rr.Code, http.StatusNotFound, rr.Body.String())
	}
}

func TestInternalMembership_NonexistentProjectReturns404(t *testing.T) {
	db := setupProjectsDB(t)
	h := handler.NewProjects(db, "test-secret", nil)
	r := projectsRouter(h)

	rr := internalMembershipRequest(t, r, "no-such-project", "user-1")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rr.Code, http.StatusNotFound, rr.Body.String())
	}
}
