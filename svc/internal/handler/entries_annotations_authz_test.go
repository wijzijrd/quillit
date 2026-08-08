package handler_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/quillit/svc/internal/handler"
)

// setupAuthzTestDB builds a minimal projects/project_members/entries/
// annotations schema with two projects, one member each, and one entry
// per project — enough to prove entries/annotations reads are actually
// scoped by project membership (#30), not the full production schema.
func setupAuthzTestDB(t *testing.T) *sql.DB {
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
	);
	CREATE TABLE entries (
		id TEXT PRIMARY KEY, title TEXT NOT NULL DEFAULT '', category TEXT NOT NULL DEFAULT '',
		body TEXT NOT NULL DEFAULT '', body_key TEXT,
		visibility TEXT NOT NULL DEFAULT 'private',
		campaign_ids TEXT NOT NULL DEFAULT '[]',
		linked_entries TEXT NOT NULL DEFAULT '[]',
		tags TEXT NOT NULL DEFAULT '[]',
		quick_view_data TEXT NOT NULL DEFAULT '{}',
		created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
		owner_user_id TEXT
	);
	CREATE TABLE annotations (
		id TEXT PRIMARY KEY, entry_id TEXT NOT NULL REFERENCES entries(id),
		text TEXT NOT NULL DEFAULT '', visibility TEXT NOT NULL DEFAULT 'gm',
		shared_with TEXT NOT NULL DEFAULT '[]',
		author_user_id TEXT,
		created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL
	);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}

	now := time.Now().Unix()
	db.Exec(`INSERT INTO projects VALUES ('proj-a','Project A','campaign','user1',?)`, now)
	db.Exec(`INSERT INTO projects VALUES ('proj-b','Project B','campaign','user2',?)`, now)
	db.Exec(`INSERT INTO project_members VALUES ('m1','proj-a','user1','gm',?,'u1')`, now)
	db.Exec(`INSERT INTO project_members VALUES ('m2','proj-b','user2','gm',?,'u2')`, now)

	// entry-a belongs only to proj-a; entry-b belongs only to proj-b.
	db.Exec(`INSERT INTO entries (id,title,category,body,visibility,campaign_ids,created_at,updated_at) VALUES
		('entry-a','Entry A','Lore','body-a','private','["proj-a"]',?,?)`, now, now)
	db.Exec(`INSERT INTO entries (id,title,category,body,visibility,campaign_ids,created_at,updated_at) VALUES
		('entry-b','Entry B','Lore','body-b','private','["proj-b"]',?,?)`, now, now)

	db.Exec(`INSERT INTO annotations (id,entry_id,text,visibility,shared_with,created_at,updated_at) VALUES
		('anno-a','entry-a','GM secret for A','gm','[]',?,?)`, now, now)

	return db
}

func entriesRouter(db *sql.DB) http.Handler {
	h := handler.NewEntriesForTest(db)
	r := chi.NewRouter()
	r.Get("/api/entries", h.List)
	r.Get("/api/entries/{id}", h.Get)
	return r
}

func annotationsRouter(db *sql.DB) http.Handler {
	h := handler.NewAnnotationsForTest(db)
	r := chi.NewRouter()
	r.Get("/api/annotations", h.List)
	return r
}

func authzRequest(t *testing.T, router http.Handler, method, path, userID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if userID != "" {
		req = req.WithContext(handler.WithTestCallerID(req.Context(), userID))
	}
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

func TestEntriesList_FiltersToMemberProjects(t *testing.T) {
	db := setupAuthzTestDB(t)
	rr := authzRequest(t, entriesRouter(db), "GET", "/api/entries", "user1")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var entries []handler.Entry
	if err := json.NewDecoder(rr.Body).Decode(&entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry (only entry-a, user1's own project), got %d: %+v", len(entries), entries)
	}
	if entries[0].ID != "entry-a" {
		t.Errorf("expected entry-a, got %s", entries[0].ID)
	}
}

func TestEntriesGet_DeniesNonMember(t *testing.T) {
	db := setupAuthzTestDB(t)
	// user1 is not a member of proj-b, which owns entry-b.
	rr := authzRequest(t, entriesRouter(db), "GET", "/api/entries/entry-b", "user1")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestEntriesGet_AllowsMember(t *testing.T) {
	db := setupAuthzTestDB(t)
	rr := authzRequest(t, entriesRouter(db), "GET", "/api/entries/entry-a", "user1")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var e handler.Entry
	if err := json.NewDecoder(rr.Body).Decode(&e); err != nil {
		t.Fatal(err)
	}
	if e.ID != "entry-a" || e.Title != "Entry A" {
		t.Errorf("unexpected entry, response shape may have changed: %+v", e)
	}
}

func TestEntriesList_Unauthenticated(t *testing.T) {
	db := setupAuthzTestDB(t)
	rr := authzRequest(t, entriesRouter(db), "GET", "/api/entries", "")
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestEntriesGet_Unauthenticated(t *testing.T) {
	db := setupAuthzTestDB(t)
	rr := authzRequest(t, entriesRouter(db), "GET", "/api/entries/entry-a", "")
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAnnotationsList_DeniesNonMemberEntry(t *testing.T) {
	db := setupAuthzTestDB(t)
	// entry-a's annotation exists (visibility=gm) but user2 isn't a
	// member of proj-a — the exact hole #30 closes.
	rr := authzRequest(t, annotationsRouter(db), "GET", "/api/annotations?entryId=entry-a", "user2")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAnnotationsList_AllowsMemberEntry(t *testing.T) {
	db := setupAuthzTestDB(t)
	rr := authzRequest(t, annotationsRouter(db), "GET", "/api/annotations?entryId=entry-a", "user1")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var annos []handler.Annotation
	if err := json.NewDecoder(rr.Body).Decode(&annos); err != nil {
		t.Fatal(err)
	}
	if len(annos) != 1 || annos[0].Text != "GM secret for A" {
		t.Fatalf("unexpected annotations, response shape may have changed: %+v", annos)
	}
}

// TestAnnotationsList_NoEntryIdFiltersToAccessible covers the bulk
// (no entryId) path too — the issue's own acceptance criteria only spell
// out the entryId-filtered case, but leaving bare `/api/annotations`
// unfiltered would make the fix trivially bypassable by just omitting
// the query param, so it gets the same membership filter.
func TestAnnotationsList_NoEntryIdFiltersToAccessible(t *testing.T) {
	db := setupAuthzTestDB(t)
	now := time.Now().Unix()
	db.Exec(`INSERT INTO annotations (id,entry_id,text,visibility,shared_with,created_at,updated_at) VALUES
		('anno-b','entry-b','GM secret for B','gm','[]',?,?)`, now, now)

	rr := authzRequest(t, annotationsRouter(db), "GET", "/api/annotations", "user1")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var annos []handler.Annotation
	if err := json.NewDecoder(rr.Body).Decode(&annos); err != nil {
		t.Fatal(err)
	}
	if len(annos) != 1 || annos[0].EntryID != "entry-a" {
		t.Fatalf("expected only entry-a's annotation (bulk list must not leak entry-b's), got %+v", annos)
	}
}
