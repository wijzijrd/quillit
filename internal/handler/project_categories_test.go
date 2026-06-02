package handler_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/quillit/svc/internal/handler"
)

func setupTestDB(t *testing.T) *sql.DB {
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
		id TEXT PRIMARY KEY, project_id TEXT NOT NULL REFERENCES projects(id),
		user_id TEXT NOT NULL, role TEXT NOT NULL, joined_at INTEGER NOT NULL,
		username TEXT NOT NULL DEFAULT '',
		UNIQUE(project_id, user_id)
	);
	CREATE TABLE categories (
		id TEXT PRIMARY KEY, name TEXT NOT NULL, icon TEXT NOT NULL DEFAULT '',
		color TEXT NOT NULL DEFAULT '', sort_order INTEGER NOT NULL DEFAULT 0,
		project_id TEXT NOT NULL DEFAULT 'global', created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
		UNIQUE(name, project_id)
	);
	CREATE TABLE category_default_tags (
		id TEXT PRIMARY KEY, category_id TEXT NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
		label TEXT NOT NULL, sort_order INTEGER NOT NULL DEFAULT 0
	);
	CREATE TABLE project_global_categories (
		project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
		category_id TEXT NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
		PRIMARY KEY (project_id, category_id)
	);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	db.Exec(`INSERT INTO projects VALUES ('global','Global','global','system',?)`, now)
	db.Exec(`INSERT INTO projects VALUES ('proj1','TestCampaign','campaign','user1',?)`, now)
	db.Exec(`INSERT INTO project_members VALUES ('m1','proj1','user1','gm',?,'gmuser')`, now)
	db.Exec(`INSERT INTO categories VALUES ('gcat1','Lore','BookMarked','#aaa',0,'global',?,?)`, now, now)
	return db
}

func makeRequest(t *testing.T, db *sql.DB, method, path string, body any, userID string) *httptest.ResponseRecorder {
	t.Helper()
	h := handler.NewProjectsForTest(db)

	r := chi.NewRouter()
	r.Get("/projects/{projectId}/categories", h.ListProjectCategories)
	r.Post("/projects/{projectId}/categories", h.CreateProjectCategory)
	r.Post("/projects/{projectId}/categories/global/{catId}", h.OptInGlobalCategory)
	r.Delete("/projects/{projectId}/categories/{catId}", h.RemoveProjectCategory)

	var bodyBytes []byte
	if body != nil {
		bodyBytes, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(handler.WithTestCallerID(req.Context(), userID))

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}

func TestListProjectCategories_ReturnsEmpty(t *testing.T) {
	db := setupTestDB(t)
	rr := makeRequest(t, db, "GET", "/projects/proj1/categories", nil, "user1")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var cats []handler.Category
	json.NewDecoder(rr.Body).Decode(&cats)
	if len(cats) != 0 {
		t.Fatalf("expected 0 categories, got %d", len(cats))
	}
}

func TestOptInGlobalCategory_AndList(t *testing.T) {
	db := setupTestDB(t)
	rr := makeRequest(t, db, "POST", "/projects/proj1/categories/global/gcat1", nil, "user1")
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	rr2 := makeRequest(t, db, "GET", "/projects/proj1/categories", nil, "user1")
	var cats []handler.Category
	json.NewDecoder(rr2.Body).Decode(&cats)
	if len(cats) != 1 {
		t.Fatalf("expected 1 category, got %d", len(cats))
	}
	if cats[0].Source != "global" {
		t.Errorf("expected source=global, got %q", cats[0].Source)
	}
}

func TestCreateProjectCategory_OwnCategory(t *testing.T) {
	db := setupTestDB(t)
	body := map[string]any{"name": "Quests", "icon": "Sword", "color": "#f00", "sortOrder": 0}
	rr := makeRequest(t, db, "POST", "/projects/proj1/categories", body, "user1")
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var cat handler.Category
	json.NewDecoder(rr.Body).Decode(&cat)
	if cat.Name != "Quests" {
		t.Errorf("expected name Quests, got %q", cat.Name)
	}
	if cat.Source != "own" {
		t.Errorf("expected source=own, got %q", cat.Source)
	}
}

func TestRemoveProjectCategory_OptsOutGlobal(t *testing.T) {
	db := setupTestDB(t)
	makeRequest(t, db, "POST", "/projects/proj1/categories/global/gcat1", nil, "user1")
	rr := makeRequest(t, db, "DELETE", fmt.Sprintf("/projects/proj1/categories/gcat1"), nil, "user1")
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rr.Code, rr.Body.String())
	}
	rr2 := makeRequest(t, db, "GET", "/projects/proj1/categories", nil, "user1")
	var cats []handler.Category
	json.NewDecoder(rr2.Body).Decode(&cats)
	if len(cats) != 0 {
		t.Fatalf("expected 0 categories after opt-out, got %d", len(cats))
	}
}
