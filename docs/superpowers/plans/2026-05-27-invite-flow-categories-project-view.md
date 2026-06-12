# Invite Flow Fix, Project-Scoped Categories & Project View Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix invite context loss through auth redirects, scope category features to project routes only, and add a ProjectView with account-search member inviting.

**Architecture:** Backend gets a v2 migration (global project seed + category project_id + junction table) and new project-category endpoints. Frontend adds `/projects/:projectId` and `/projects/:projectId/notes` routes — the route param is the project-context signal for all category visibility logic. Invite fix threads a `?redirect=` param through the auth flow so tokens survive login/register redirects.

**Tech Stack:** Go (chi router, SQLite), Vue 3 (Composition API, Pinia), Vue Router 4, ofetch, lucide-vue-next

---

## File Map

**Modified (backend):**
- `quillit-svc/internal/db/db.go` — v2 migration: global project seed, recreate categories with project_id, add project_global_categories table
- `quillit-svc/internal/handler/categories.go` — update List/Create to filter/set project_id='global'
- `quillit-svc/internal/handler/projects.go` — add project category handlers + filter global from List
- `quillit-svc/main.go` — register new category routes on projects

**Created (backend):**
- `quillit-svc/internal/handler/project_categories_test.go` — httptest integration tests for project category endpoints

**Modified (frontend):**
- `quillit-ui/src/router/index.js` — add project routes, preserve redirect on auth bounce
- `quillit-ui/src/stores/useCategoriesStore.js` — add projectCategories + initForProject()
- `quillit-ui/src/views/LoginView.vue` — navigate to ?redirect after login
- `quillit-ui/src/views/SetupView.vue` — auto-join if already logged in
- `quillit-ui/src/views/QuillitView.vue` — flat list outside project, grouped inside
- `quillit-ui/src/views/DashboardView.vue` — remove category tab strip
- `quillit-ui/src/components/SideNav.vue` — hide category section outside project
- `quillit-ui/src/components/EntryEditor.vue` — hide category select outside project

**Created (frontend):**
- `quillit-ui/src/views/ProjectView.vue` — project settings, members, account-search invite

---

## Task 1: DB migration v2

**Files:**
- Modify: `quillit-svc/internal/db/db.go`

- [ ] **Step 1: Add `toV2` function**

In `db.go`, after the `toV1` function and before `addColumnIfMissing`, add:

```go
func toV2(db *sql.DB) error {
	// FK constraints can't be toggled inside a transaction; disable outside.
	if _, err := db.Exec("PRAGMA foreign_keys=OFF"); err != nil {
		return err
	}
	defer db.Exec("PRAGMA foreign_keys=ON") //nolint:errcheck

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().Unix()

	// Seed the system "global" project that owns admin-managed categories.
	if _, err := tx.Exec(`
		INSERT OR IGNORE INTO projects (id, name, type, created_by, created_at)
		VALUES ('global', 'Global Categories', 'global', 'system', ?)
	`, now); err != nil {
		return fmt.Errorf("seed global project: %w", err)
	}

	// Recreate categories with project_id column and (name, project_id) uniqueness.
	// The old schema had UNIQUE(name); SQLite requires table recreation to change constraints.
	if _, err := tx.Exec(`
		ALTER TABLE categories RENAME TO categories_v1;

		CREATE TABLE categories (
			id         TEXT    PRIMARY KEY,
			name       TEXT    NOT NULL,
			icon       TEXT    NOT NULL DEFAULT '',
			color      TEXT    NOT NULL DEFAULT '',
			sort_order INTEGER NOT NULL DEFAULT 0,
			project_id TEXT    NOT NULL DEFAULT 'global'
			               REFERENCES projects(id) ON DELETE CASCADE,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			UNIQUE(name, project_id)
		);

		INSERT INTO categories (id, name, icon, color, sort_order, project_id, created_at, updated_at)
		SELECT id, name, icon, color, sort_order, 'global', created_at, updated_at
		FROM categories_v1;

		DROP TABLE categories_v1;
	`); err != nil {
		return fmt.Errorf("recreate categories: %w", err)
	}

	// Junction table: which global categories a project has opted into.
	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS project_global_categories (
			project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			category_id TEXT NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
			PRIMARY KEY (project_id, category_id)
		);
	`); err != nil {
		return fmt.Errorf("create project_global_categories: %w", err)
	}

	if _, err := tx.Exec(`PRAGMA user_version = 2`); err != nil {
		return err
	}
	return tx.Commit()
}
```

- [ ] **Step 2: Add `addColumnIfMissing` call for `project_members.username`**

In `toV2`, before the `PRAGMA user_version = 2` line, add this (it stores the display name when a member is added via search):

```go
	if err := addColumnIfMissing(tx, "project_members", "username", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("add username column: %w", err)
	}
```

- [ ] **Step 3: Update `migrate()` to call `toV2`**

Change `migrate` to:

```go
func migrate(db *sql.DB) error {
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if version < 1 {
		if err := toV1(db); err != nil {
			return fmt.Errorf("schema v1: %w", err)
		}
	}
	if version < 2 {
		if err := toV2(db); err != nil {
			return fmt.Errorf("schema v2: %w", err)
		}
	}
	return nil
}
```

- [ ] **Step 4: Verify build**

```bash
cd /Users/tauriqbehardien/repositories/quillit/quillit-svc && go build ./...
```

Expected: no output (clean build).

- [ ] **Step 5: Commit**

```bash
cd /Users/tauriqbehardien/repositories/quillit
git add quillit-svc/internal/db/db.go
git commit -m "feat(db): v2 migration — global project seed, category project_id, project_global_categories"
```

---

## Task 2: Update existing categories handler for project_id

**Files:**
- Modify: `quillit-svc/internal/handler/categories.go`

The existing `List` and `Create` handlers need to know about `project_id='global'`.

- [ ] **Step 1: Add `ProjectID` and `Source` fields to `Category` struct**

In `categories.go`, update the `Category` struct:

```go
type Category struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Icon        string               `json:"icon"`
	Color       string               `json:"color"`
	SortOrder   int                  `json:"sortOrder"`
	ProjectID   string               `json:"projectId,omitempty"`
	Source      string               `json:"source,omitempty"` // "own" or "global" — set by project category endpoint
	DefaultTags []CategoryDefaultTag `json:"defaultTags"`
	CreatedAt   int64                `json:"createdAt"`
	UpdatedAt   int64                `json:"updatedAt"`
}
```

- [ ] **Step 2: Filter `List` to global categories only**

The admin `GET /api/categories` should only return global categories. Update the query in `List`:

```go
func (h *CategoriesHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.QueryContext(r.Context(),
		`SELECT id, name, icon, color, sort_order, created_at, updated_at
		 FROM categories WHERE project_id = 'global' ORDER BY sort_order`)
```

The rest of `List` (tag loading loop) is unchanged.

- [ ] **Step 3: Set `project_id='global'` in `Create`**

Update the `Create` INSERT:

```go
	if _, err := h.db.ExecContext(r.Context(),
		`INSERT INTO categories (id, name, icon, color, sort_order, project_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 'global', ?, ?)`,
		id, req.Name, req.Icon, req.Color, req.SortOrder, now, now,
	); err != nil {
```

- [ ] **Step 4: Verify build**

```bash
cd /Users/tauriqbehardien/repositories/quillit/quillit-svc && go build ./...
```

Expected: clean build.

- [ ] **Step 5: Commit**

```bash
git add quillit-svc/internal/handler/categories.go
git commit -m "feat(categories): scope admin list/create to project_id='global'"
```

---

## Task 3: Add project category handlers

**Files:**
- Modify: `quillit-svc/internal/handler/projects.go`
- Create: `quillit-svc/internal/handler/project_categories_test.go`

Add four methods to `ProjectsHandler`: `ListProjectCategories`, `CreateProjectCategory`, `OptInGlobalCategory`, `RemoveProjectCategory`.

- [ ] **Step 1: Write the failing tests**

Create `quillit-svc/internal/handler/project_categories_test.go`:

```go
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
	if _, err := db.Exec(`PRAGMA foreign_keys=ON;`); err != nil {
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
	// Seed global project + a test project + a GM member
	db.Exec(`INSERT INTO projects VALUES ('global','Global','global','system',?)`, now)
	db.Exec(`INSERT INTO projects VALUES ('proj1','TestCampaign','campaign','user1',?)`, now)
	db.Exec(`INSERT INTO project_members VALUES ('m1','proj1','user1','gm',?,'gmuser')`, now)
	// Seed a global category
	db.Exec(`INSERT INTO categories VALUES ('gcat1','Lore','BookMarked','#aaa',0,'global',?,?)`, now, now)
	return db
}

// makeRequest builds a chi router with the projects handler and fires a request.
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
	// Inject caller identity via context (test helper sets userID directly)
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
	// Opt in gcat1
	rr := makeRequest(t, db, "POST", "/projects/proj1/categories/global/gcat1", nil, "user1")
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	// List should now include it with source=global
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
	// Opt in first
	makeRequest(t, db, "POST", "/projects/proj1/categories/global/gcat1", nil, "user1")
	// Remove (opt-out)
	rr := makeRequest(t, db, "DELETE", fmt.Sprintf("/projects/proj1/categories/gcat1"), nil, "user1")
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rr.Code, rr.Body.String())
	}
	// List should be empty again
	rr2 := makeRequest(t, db, "GET", "/projects/proj1/categories", nil, "user1")
	var cats []handler.Category
	json.NewDecoder(rr2.Body).Decode(&cats)
	if len(cats) != 0 {
		t.Fatalf("expected 0 categories after opt-out, got %d", len(cats))
	}
}
```

- [ ] **Step 2: Add test helpers to projects.go**

At the bottom of `projects.go`, add the two helpers the tests need:

```go
// NewProjectsForTest creates a handler that uses test context for caller ID.
// Only available for test builds; the real callerID() reads JWT from context.
func NewProjectsForTest(db *sql.DB) *ProjectsHandler {
	return &ProjectsHandler{db: db, jwtSecret: nil}
}

type testCallerKey struct{}

// WithTestCallerID injects a caller ID into a request context for tests.
func WithTestCallerID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, testCallerKey{}, id)
}
```

Also update `callerID` to check the test key first:

```go
func (h *ProjectsHandler) callerID(r *http.Request) (string, bool) {
	// Test helper: allow injecting caller ID without JWT.
	if id, ok := r.Context().Value(testCallerKey{}).(string); ok && id != "" {
		return id, true
	}
	mc, err := middleware.ClaimsFromContext(r.Context(), h.jwtSecret)
	if err != nil {
		return "", false
	}
	sub, _ := mc["sub"].(string)
	return sub, sub != ""
}
```

Add `"context"` to the imports if not already present.

- [ ] **Step 3: Run tests to confirm they fail**

```bash
cd /Users/tauriqbehardien/repositories/quillit/quillit-svc && go test ./internal/handler/... -run TestListProject -v
```

Expected: compilation error — `ListProjectCategories` not defined yet.

- [ ] **Step 4: Implement the four handler methods**

Add to `projects.go`, below the `Join` handler and above the private helpers:

```go
// ── Project Categories ────────────────────────────────────────────────────────

// ListProjectCategories returns the project's own categories plus global ones it has opted into.
func (h *ProjectsHandler) ListProjectCategories(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectId")
	callerID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if _, _, err := h.memberRole(r, projectID, callerID); err != nil {
		writeError(w, http.StatusForbidden, "not a project member")
		return
	}

	rows, err := h.db.QueryContext(r.Context(), `
		SELECT id, name, icon, color, sort_order, project_id, created_at, updated_at, 'own' AS source
		FROM categories WHERE project_id = ?
		UNION ALL
		SELECT c.id, c.name, c.icon, c.color, c.sort_order, c.project_id, c.created_at, c.updated_at, 'global' AS source
		FROM categories c
		JOIN project_global_categories pgc ON pgc.category_id = c.id AND pgc.project_id = ?
		WHERE c.project_id = 'global'
		ORDER BY sort_order
	`, projectID, projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()

	cats := []Category{}
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.Name, &c.Icon, &c.Color, &c.SortOrder, &c.ProjectID, &c.CreatedAt, &c.UpdatedAt, &c.Source); err != nil {
			writeError(w, http.StatusInternalServerError, "scan error")
			return
		}
		c.DefaultTags = h.projectCategoryDefaultTags(r, c.ID)
		cats = append(cats, c)
	}
	writeJSON(w, http.StatusOK, cats)
}

// CreateProjectCategory creates a category owned by this project.
func (h *ProjectsHandler) CreateProjectCategory(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectId")
	callerID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	projectType, myRole, err := h.memberRole(r, projectID, callerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if !isEditorRole(projectType, myRole) {
		writeError(w, http.StatusForbidden, "editor access required")
		return
	}

	var body struct {
		Name      string `json:"name"`
		Icon      string `json:"icon"`
		Color     string `json:"color"`
		SortOrder int    `json:"sortOrder"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}

	now := nowUnix()
	id := newID()
	if _, err := h.db.ExecContext(r.Context(),
		`INSERT INTO categories (id, name, icon, color, sort_order, project_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, body.Name, body.Icon, body.Color, body.SortOrder, projectID, now, now,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "insert failed")
		return
	}
	writeJSON(w, http.StatusCreated, Category{
		ID: id, Name: body.Name, Icon: body.Icon, Color: body.Color,
		SortOrder: body.SortOrder, ProjectID: projectID, Source: "own",
		DefaultTags: []CategoryDefaultTag{}, CreatedAt: now, UpdatedAt: now,
	})
}

// OptInGlobalCategory adds a global category to this project's category set.
func (h *ProjectsHandler) OptInGlobalCategory(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectId")
	catID := chi.URLParam(r, "catId")
	callerID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	projectType, myRole, err := h.memberRole(r, projectID, callerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if !isEditorRole(projectType, myRole) {
		writeError(w, http.StatusForbidden, "editor access required")
		return
	}

	if _, err := h.db.ExecContext(r.Context(),
		`INSERT OR IGNORE INTO project_global_categories (project_id, category_id) VALUES (?, ?)`,
		projectID, catID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	var c Category
	if err := h.db.QueryRowContext(r.Context(),
		`SELECT id, name, icon, color, sort_order, project_id, created_at, updated_at
		 FROM categories WHERE id = ? AND project_id = 'global'`, catID,
	).Scan(&c.ID, &c.Name, &c.Icon, &c.Color, &c.SortOrder, &c.ProjectID, &c.CreatedAt, &c.UpdatedAt); err != nil {
		writeError(w, http.StatusNotFound, "global category not found")
		return
	}
	c.Source = "global"
	c.DefaultTags = h.projectCategoryDefaultTags(r, c.ID)
	writeJSON(w, http.StatusCreated, c)
}

// RemoveProjectCategory removes an own category or opts out of a global one.
func (h *ProjectsHandler) RemoveProjectCategory(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectId")
	catID := chi.URLParam(r, "catId")
	callerID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	projectType, myRole, err := h.memberRole(r, projectID, callerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if !isEditorRole(projectType, myRole) {
		writeError(w, http.StatusForbidden, "editor access required")
		return
	}

	// Determine if it's an own category or a global opt-in.
	var storedProjectID string
	if err := h.db.QueryRowContext(r.Context(),
		`SELECT project_id FROM categories WHERE id = ?`, catID,
	).Scan(&storedProjectID); errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "category not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	if storedProjectID == projectID {
		// Own category — delete it entirely.
		h.db.ExecContext(r.Context(), `DELETE FROM categories WHERE id = ?`, catID) //nolint:errcheck
	} else {
		// Global category — remove from junction table only.
		h.db.ExecContext(r.Context(), //nolint:errcheck
			`DELETE FROM project_global_categories WHERE project_id = ? AND category_id = ?`,
			projectID, catID,
		)
	}
	w.WriteHeader(http.StatusNoContent)
}

// projectCategoryDefaultTags fetches default tags for a category (reused helper).
func (h *ProjectsHandler) projectCategoryDefaultTags(r *http.Request, catID string) []CategoryDefaultTag {
	rows, err := h.db.QueryContext(r.Context(),
		`SELECT id, category_id, label, sort_order FROM category_default_tags WHERE category_id = ? ORDER BY sort_order`, catID)
	if err != nil {
		return []CategoryDefaultTag{}
	}
	defer rows.Close()
	tags := []CategoryDefaultTag{}
	for rows.Next() {
		var t CategoryDefaultTag
		_ = rows.Scan(&t.ID, &t.CategoryID, &t.Label, &t.SortOrder)
		tags = append(tags, t)
	}
	return tags
}
```

Note: `CategoryDefaultTag` and `Category` are defined in `categories.go` in the same `handler` package — they're accessible here without import.

- [ ] **Step 5: Run tests**

```bash
cd /Users/tauriqbehardien/repositories/quillit/quillit-svc && go test ./internal/handler/... -v
```

Expected: all four tests pass.

- [ ] **Step 6: Commit**

```bash
git add quillit-svc/internal/handler/projects.go quillit-svc/internal/handler/project_categories_test.go
git commit -m "feat(projects): add project category handlers (list, create, opt-in global, remove)"
```

---

## Task 4: Register project category routes + filter global project from list

**Files:**
- Modify: `quillit-svc/main.go`
- Modify: `quillit-svc/internal/handler/projects.go`

- [ ] **Step 1: Update `projects.List` to exclude the global project**

In `projects.go`, update the query in `List`:

```go
	rows, err := h.db.QueryContext(r.Context(), `
		SELECT p.id, p.name, p.type, p.created_by, p.created_at,
		       pm.role,
		       (SELECT COUNT(*) FROM project_members WHERE project_id = p.id) AS member_count
		FROM projects p
		JOIN project_members pm ON pm.project_id = p.id AND pm.user_id = ?
		WHERE p.type != 'global'
		ORDER BY p.created_at DESC
	`, callerID)
```

- [ ] **Step 2: Register the four new routes in `main.go`**

After the existing `r.Delete("/api/projects/{id}/invite/{token}", ...)` line, add:

```go
		r.Get("/api/projects/{projectId}/categories", projects.ListProjectCategories)
		r.Post("/api/projects/{projectId}/categories", projects.CreateProjectCategory)
		r.Post("/api/projects/{projectId}/categories/global/{catId}", projects.OptInGlobalCategory)
		r.Delete("/api/projects/{projectId}/categories/{catId}", projects.RemoveProjectCategory)
```

Note: uses `{projectId}` (not `{id}`) to match the chi param name in the handler.

- [ ] **Step 3: Build and verify**

```bash
cd /Users/tauriqbehardien/repositories/quillit/quillit-svc && go build ./...
```

Expected: clean build.

- [ ] **Step 4: Commit**

```bash
git add quillit-svc/main.go quillit-svc/internal/handler/projects.go
git commit -m "feat(api): register project category routes; exclude global project from list"
```

---

## Task 5: Update `useCategoriesStore` with project category support

**Files:**
- Modify: `quillit-ui/src/stores/useCategoriesStore.js`

- [ ] **Step 1: Replace the entire file content**

```js
import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api } from '../api/client.js'

export const useCategoriesStore = defineStore('categories', () => {
  // Global admin-managed categories (used by AdminCategoriesView only)
  const categories = ref([])
  const loaded = ref(false)

  // Categories for the currently active project (own + opted-in globals)
  const projectCategories = ref([])

  // ── Global (admin) methods ────────────────────────────────────────────────

  async function init() {
    if (loaded.value) return
    categories.value = await api('/categories')
    loaded.value = true
  }

  async function createCategory(data) {
    const cat = await api('/categories', { method: 'POST', body: data })
    categories.value.push(cat)
    return cat
  }

  async function updateCategory(id, data) {
    const updated = await api(`/categories/${id}`, { method: 'PATCH', body: data })
    const idx = categories.value.findIndex(c => c.id === id)
    if (idx !== -1) categories.value[idx] = updated
    return updated
  }

  async function deleteCategory(id) {
    await api(`/categories/${id}`, { method: 'DELETE' })
    categories.value = categories.value.filter(c => c.id !== id)
  }

  async function addDefaultTag(categoryId, label) {
    const tag = await api(`/categories/${categoryId}/tags`, { method: 'POST', body: { label } })
    const cat = categories.value.find(c => c.id === categoryId)
    if (cat) cat.defaultTags.push(tag)
    return tag
  }

  async function removeDefaultTag(categoryId, tagId) {
    await api(`/categories/${categoryId}/tags/${tagId}`, { method: 'DELETE' })
    const cat = categories.value.find(c => c.id === categoryId)
    if (cat) cat.defaultTags = cat.defaultTags.filter(t => t.id !== tagId)
  }

  function defaultTagsFor(categoryName) {
    const cat = categories.value.find(c => c.name === categoryName)
    return cat ? cat.defaultTags.map(t => t.label) : []
  }

  function categoryFor(name) {
    return categories.value.find(c => c.name === name) ?? null
  }

  // ── Project-scoped methods ────────────────────────────────────────────────

  async function initForProject(projectId) {
    projectCategories.value = await api(`/projects/${projectId}/categories`)
  }

  async function createProjectCategory(projectId, data) {
    const cat = await api(`/projects/${projectId}/categories`, { method: 'POST', body: data })
    projectCategories.value.push(cat)
    return cat
  }

  async function addGlobalToProject(projectId, catId) {
    const cat = await api(`/projects/${projectId}/categories/global/${catId}`, { method: 'POST' })
    projectCategories.value.push(cat)
    return cat
  }

  async function removeFromProject(projectId, catId) {
    await api(`/projects/${projectId}/categories/${catId}`, { method: 'DELETE' })
    projectCategories.value = projectCategories.value.filter(c => c.id !== catId)
  }

  function projectCategoryFor(name) {
    return projectCategories.value.find(c => c.name === name) ?? null
  }

  return {
    categories, projectCategories, loaded,
    init, createCategory, updateCategory, deleteCategory,
    addDefaultTag, removeDefaultTag, defaultTagsFor, categoryFor,
    initForProject, createProjectCategory, addGlobalToProject, removeFromProject, projectCategoryFor,
  }
})
```

- [ ] **Step 2: Commit**

```bash
git add quillit-ui/src/stores/useCategoriesStore.js
git commit -m "feat(store): add initForProject and project category methods to useCategoriesStore"
```

---

## Task 6: Add project routes and fix auth redirect

**Files:**
- Modify: `quillit-ui/src/router/index.js`

- [ ] **Step 1: Replace the file content**

```js
import { createRouter, createWebHistory } from 'vue-router'
import DashboardView from '../views/DashboardView.vue'
import QuillitView from '../views/QuillitView.vue'
import { useAuthStore } from '../stores/useAuthStore.js'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', component: DashboardView },
    { path: '/notes', component: QuillitView },
    { path: '/notes/:id', component: QuillitView },
    { path: '/quillit', redirect: '/notes' },
    { path: '/quillit/:id', redirect: to => `/notes/${to.params.id}` },
    // Project-scoped routes — route.params.projectId signals project context
    { path: '/projects/:projectId', component: () => import('../views/ProjectView.vue') },
    { path: '/projects/:projectId/notes', component: QuillitView },
    { path: '/campaigns', component: () => import('../views/CampaignsView.vue') },
    { path: '/players', redirect: '/campaigns' },
    { path: '/member', component: () => import('../views/MemberView.vue') },
    { path: '/share/:token', component: () => import('../views/ShareView.vue'), meta: { public: true } },
    { path: '/tag/:tag', component: () => import('../views/TagView.vue') },
    { path: '/profile', component: () => import('../views/ProfileView.vue') },
    { path: '/login', component: () => import('../views/LoginView.vue'), meta: { public: true } },
    { path: '/register', component: () => import('../views/SetupView.vue'), meta: { public: true } },
    { path: '/admin', component: () => import('../views/AdminView.vue'), meta: { adminOnly: true } },
    { path: '/admin/categories', redirect: '/admin' },
  ],
})

router.beforeEach(async (to) => {
  if (to.meta.public) return true

  const auth = useAuthStore()
  await auth.fetchMe()
  if (!auth.isLoggedIn) {
    // Preserve the original destination so LoginView can redirect back after auth.
    return { path: '/login', query: { redirect: to.fullPath } }
  }

  if (to.meta.adminOnly && auth.user?.role !== 'admin') return '/'
})

export default router
```

- [ ] **Step 2: Commit**

```bash
git add quillit-ui/src/router/index.js
git commit -m "feat(router): add project routes; preserve redirect param on auth bounce"
```

---

## Task 7: New ProjectView.vue

**Files:**
- Create: `quillit-ui/src/views/ProjectView.vue`

- [ ] **Step 1: Create the file**

```vue
<template>
  <div class="project-view">
    <header class="pv-header">
      <div class="pv-title-row">
        <div>
          <h1 class="pv-name">{{ project?.name ?? '…' }}</h1>
          <span class="pv-type-badge">{{ typeLabel }}</span>
        </div>
        <RouterLink :to="`/projects/${projectId}/notes`" class="pv-open-btn">
          Open Notes
        </RouterLink>
      </div>
    </header>

    <!-- Members -->
    <section class="pv-section">
      <h2 class="pv-section-title">Members</h2>
      <div class="member-list">
        <div v-for="m in members" :key="m.id" class="member-row">
          <span class="member-identity">{{ m.username || m.userId }}</span>
          <span class="member-role">{{ m.role }}</span>
          <button
            v-if="isEditor && m.userId !== auth.user?.sub"
            class="member-remove"
            @click="removeMember(m.userId)"
          >Remove</button>
        </div>
        <p class="pv-empty" v-if="!members.length">No members yet.</p>
      </div>
    </section>

    <!-- Add member via user search -->
    <section class="pv-section" v-if="isEditor">
      <h2 class="pv-section-title">Add Member</h2>
      <div class="search-wrap">
        <input
          class="search-input"
          v-model="searchQuery"
          placeholder="Search by username or email…"
          @input="onSearch"
          autocomplete="off"
        />
        <div class="search-dropdown" v-if="searchResults.length">
          <button
            v-for="u in searchResults"
            :key="u.id"
            class="search-result"
            @click="addUser(u)"
          >
            <span class="sr-username">{{ u.username }}</span>
            <span class="sr-email">{{ u.email }}</span>
          </button>
        </div>
      </div>
      <p class="pv-note">In a future update, invitations will require the user to accept.</p>
    </section>

    <!-- Token invite link -->
    <section class="pv-section" v-if="isEditor">
      <h2 class="pv-section-title">Invite Link</h2>
      <button class="pv-btn" @click="generateLink">Generate invite link</button>
      <div v-if="inviteLink" class="invite-link-row">
        <input class="invite-link-input" readonly :value="inviteLink" />
        <button class="pv-btn" @click="copyLink">Copy</button>
      </div>
    </section>

    <p class="pv-error" v-if="error">{{ error }}</p>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute, RouterLink } from 'vue-router'
import { useProjectStore } from '../stores/useProjectStore.js'
import { useMemberStore } from '../stores/useMemberStore.js'
import { useAuthStore } from '../stores/useAuthStore.js'

const route = useRoute()
const projectStore = useProjectStore()
const memberStore = useMemberStore()
const auth = useAuthStore()

const projectId = computed(() => route.params.projectId)
const project = computed(() => projectStore.projects.find(p => p.id === projectId.value) ?? null)
const members = ref([])
const searchQuery = ref('')
const searchResults = ref([])
const inviteLink = ref('')
const error = ref('')
let searchTimer = null

const typeLabel = computed(() => {
  if (!project.value) return ''
  return project.value.type === 'campaign' ? 'Campaign' : 'Book'
})

const isEditor = computed(() => {
  if (!project.value || !auth.user) return false
  const editorRole = project.value.roleLabels?.[0]?.toLowerCase()
  return project.value.myRole === editorRole || auth.user.role === 'admin'
})

onMounted(async () => {
  await Promise.all([projectStore.fetchProjects(), projectStore.fetchTypes()])
  members.value = await projectStore.fetchMembers(projectId.value)
})

function onSearch() {
  clearTimeout(searchTimer)
  searchResults.value = []
  if (searchQuery.value.trim().length < 2) return
  searchTimer = setTimeout(async () => {
    searchResults.value = await memberStore.searchUsers(searchQuery.value.trim())
  }, 280)
}

async function addUser(user) {
  error.value = ''
  const memberRole = project.value?.roleLabels?.[1]?.toLowerCase() ?? 'member'
  try {
    const m = await projectStore.addMember(projectId.value, user.id, memberRole, user.username)
    members.value.push(m)
    searchQuery.value = ''
    searchResults.value = []
  } catch (e) {
    error.value = e?.data?.error ?? 'Could not add member'
  }
}

async function removeMember(userId) {
  error.value = ''
  try {
    await projectStore.removeMember(projectId.value, userId)
    members.value = members.value.filter(m => m.userId !== userId)
  } catch (e) {
    error.value = e?.data?.error ?? 'Could not remove member'
  }
}

async function generateLink() {
  const memberRole = project.value?.roleLabels?.[1]?.toLowerCase() ?? 'member'
  const inv = await projectStore.generateInvite(projectId.value, memberRole)
  inviteLink.value = `${window.location.origin}/register?invite=${inv.token}`
}

function copyLink() {
  navigator.clipboard.writeText(inviteLink.value).catch(() => {})
}
</script>

<style scoped>
.project-view {
  max-width: 680px;
  margin: 0 auto;
  padding: var(--space-2xl) var(--space-xl);
  display: flex;
  flex-direction: column;
  gap: var(--space-2xl);
}
.pv-header { display: flex; flex-direction: column; gap: var(--space-sm); }
.pv-title-row { display: flex; align-items: flex-start; justify-content: space-between; gap: var(--space-lg); }
.pv-name { font-family: var(--font-display); font-size: var(--text-2xl); color: var(--text-primary); font-weight: 400; margin: 0; }
.pv-type-badge {
  display: inline-block; margin-top: var(--space-xs);
  font-size: var(--text-xs); text-transform: uppercase; letter-spacing: 0.08em;
  color: var(--accent); background: color-mix(in srgb, var(--accent) 12%, transparent);
  border: 1px solid var(--accent-dim); border-radius: var(--radius);
  padding: 2px 8px;
}
.pv-open-btn {
  height: var(--h-md); padding: 0 var(--space-md);
  background: var(--accent-dim); border-radius: var(--radius);
  color: var(--accent); text-decoration: none; font-size: var(--text-md);
  display: flex; align-items: center; white-space: nowrap;
  transition: background var(--transition);
}
.pv-open-btn:hover { background: var(--accent); color: var(--bg-deep); }
.pv-section { display: flex; flex-direction: column; gap: var(--space-md); }
.pv-section-title { font-family: var(--font-display); font-size: var(--text-lg); font-weight: 400; color: var(--text-primary); margin: 0; }
.member-list { display: flex; flex-direction: column; gap: var(--space-xs); }
.member-row {
  display: flex; align-items: center; gap: var(--space-sm);
  padding: var(--space-xs) var(--space-sm);
  background: var(--bg-raised); border-radius: var(--radius);
}
.member-identity { flex: 1; font-size: var(--text-md); color: var(--text-primary); }
.member-role { font-size: var(--text-xs); text-transform: capitalize; color: var(--text-muted); }
.member-remove {
  font-size: var(--text-xs); color: var(--danger); background: none; border: none;
  cursor: pointer; padding: 2px 6px; border-radius: var(--radius);
}
.member-remove:hover { background: color-mix(in srgb, var(--danger) 12%, transparent); }
.pv-empty { font-size: var(--text-sm); color: var(--text-faint); margin: 0; }
.search-wrap { position: relative; }
.search-input {
  width: 100%; background: var(--bg-raised); border: 1px solid var(--border-light);
  border-radius: var(--radius); color: var(--text-primary); font-family: var(--font-body);
  font-size: var(--text-md); height: var(--h-md); padding: 0 var(--space-sm); outline: none;
  transition: border-color var(--transition); box-sizing: border-box;
}
.search-input:focus { border-color: var(--accent-dim); }
.search-dropdown {
  position: absolute; top: 100%; left: 0; right: 0; z-index: 10;
  background: var(--bg-surface); border: 1px solid var(--border);
  border-radius: var(--radius); margin-top: 2px; overflow: hidden;
}
.search-result {
  display: flex; align-items: center; justify-content: space-between;
  width: 100%; padding: var(--space-xs) var(--space-sm); gap: var(--space-sm);
  background: none; border: none; cursor: pointer; text-align: left;
  transition: background var(--transition);
}
.search-result:hover { background: var(--bg-hover); }
.sr-username { font-size: var(--text-md); color: var(--text-primary); }
.sr-email { font-size: var(--text-xs); color: var(--text-faint); }
.pv-note { font-size: var(--text-xs); color: var(--text-faint); margin: 0; }
.invite-link-row { display: flex; gap: var(--space-sm); }
.invite-link-input {
  flex: 1; background: var(--bg-raised); border: 1px solid var(--border-light);
  border-radius: var(--radius); color: var(--text-muted); font-family: var(--font-body);
  font-size: var(--text-sm); height: var(--h-md); padding: 0 var(--space-sm); outline: none;
}
.pv-btn {
  height: var(--h-md); padding: 0 var(--space-md); background: var(--bg-raised);
  border: 1px solid var(--border); border-radius: var(--radius);
  color: var(--text-primary); font-family: var(--font-body); font-size: var(--text-md);
  cursor: pointer; transition: background var(--transition); white-space: nowrap;
}
.pv-btn:hover { background: var(--bg-hover); }
.pv-error { font-size: var(--text-sm); color: var(--danger); margin: 0; }
</style>
```

- [ ] **Step 2: Update `useProjectStore.addMember` to accept and pass username**

In `quillit-ui/src/stores/useProjectStore.js`, update:

```js
  async function addMember(projectId, userId, role, username = '') {
    const m = await api(`/projects/${projectId}/members`, { method: 'POST', body: { userId, role, username } })
    if (membersCache.value[projectId]) membersCache.value[projectId].push(m)
    return m
  }
```

- [ ] **Step 3: Update `ProjectsHandler.AddMember` to accept and store username**

In `quillit-svc/internal/handler/projects.go`, update the `AddMember` handler's body struct and INSERT:

```go
	var body struct {
		UserID   string `json:"userId"`
		Role     string `json:"role"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.UserID == "" || body.Role == "" {
		writeError(w, http.StatusBadRequest, "userId and role required")
		return
	}
```

And update the INSERT to include username:

```go
	if _, err := tx.ExecContext(r.Context(),
		"INSERT INTO project_members (id, project_id, user_id, role, joined_at, username) VALUES (?, ?, ?, ?, ?, ?)",
		memberID, id, body.UserID, body.Role, now, body.Username,
	); err != nil {
```

Also update the `ProjectMember` struct:

```go
type ProjectMember struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	UserID    string `json:"userId"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	JoinedAt  int64  `json:"joinedAt"`
}
```

And update `ListMembers` scan to include username:

```go
		if err := rows.Scan(&m.ID, &m.ProjectID, &m.UserID, &m.Role, &m.JoinedAt); err != nil {
```

becomes:

```go
		if err := rows.Scan(&m.ID, &m.ProjectID, &m.UserID, &m.Username, &m.Role, &m.JoinedAt); err != nil {
```

And the ListMembers query:

```go
		"SELECT id, project_id, user_id, username, role, joined_at FROM project_members WHERE project_id = ? ORDER BY joined_at ASC", id,
```

And the `Join` handler which also inserts into project_members — update its INSERT to include `username = ''`:

In `projects.go`, find the JOIN handler's INSERT:
```go
	if _, err := h.db.ExecContext(r.Context(),
		"INSERT INTO project_members (id, project_id, user_id, role, joined_at) VALUES (?, ?, ?, ?, ?)",
		memberID, inv.ProjectID, callerID, inv.Role, now,
	); err != nil {
```

Change to:
```go
	if _, err := h.db.ExecContext(r.Context(),
		"INSERT INTO project_members (id, project_id, user_id, role, joined_at, username) VALUES (?, ?, ?, ?, ?, '')",
		memberID, inv.ProjectID, callerID, inv.Role, now,
	); err != nil {
```

Also update the Create handler's INSERT for the initial GM/author member:
```go
	if _, err := tx.ExecContext(r.Context(),
		"INSERT INTO project_members (id, project_id, user_id, role, joined_at, username) VALUES (?, ?, ?, ?, ?, '')",
		memberID, projectID, callerID, editorRole, now,
	); err != nil {
```

- [ ] **Step 4: Build backend and verify**

```bash
cd /Users/tauriqbehardien/repositories/quillit/quillit-svc && go build ./...
```

Expected: clean build.

- [ ] **Step 5: Commit**

```bash
git add quillit-ui/src/views/ProjectView.vue quillit-ui/src/stores/useProjectStore.js quillit-svc/internal/handler/projects.go
git commit -m "feat: add ProjectView with member list and account-search invite; store username on member add"
```

---

## Task 8: QuillitView — flat list outside project, grouped inside

**Files:**
- Modify: `quillit-ui/src/views/QuillitView.vue`

- [ ] **Step 1: Replace the full file content**

```vue
<template>
  <div class="notes-browser">
    <div class="browser-header">
      <span class="browser-title">{{ inProject ? project?.name ?? 'Notes' : 'Notes' }}</span>
      <button class="new-btn" @click="createNew">+ New</button>
    </div>

    <div class="browser-scroll">
      <div class="browser-empty" v-if="visibleEntries.length === 0 && groupedEntries.length === 0">
        No entries yet.
      </div>

      <!-- Flat list: no project context -->
      <template v-if="!inProject">
        <EntryRow
          v-for="entry in visibleEntries"
          :key="entry.id"
          :entry="entry"
          @view="openView(entry)"
          @edit="openEdit(entry)"
          @links="openView(entry)"
        />
      </template>

      <!-- Grouped by category: inside a project -->
      <template v-else>
        <div
          class="category-group"
          v-for="group in groupedEntries"
          :key="group.category"
        >
          <button class="cat-group-header" @click="toggle(group.category)">
            <component
              :is="catIcon(group.category)"
              :size="14"
              :style="{ color: catColor(group.category) }"
            />
            <span class="cat-group-name" :style="{ color: catColor(group.category) }">
              {{ group.category }}
            </span>
            <span class="cat-group-count">{{ group.entries.length }}</span>
            <ChevronDown
              :size="13"
              class="chevron"
              :class="{ rotated: collapsed[group.category] }"
            />
          </button>
          <div class="cat-group-entries" v-show="!collapsed[group.category]">
            <EntryRow
              v-for="entry in group.entries"
              :key="entry.id"
              :entry="entry"
              @view="openView(entry)"
              @edit="openEdit(entry)"
              @links="openView(entry)"
            />
          </div>
        </div>
      </template>
    </div>
  </div>

  <EntryEditModal
    v-if="editingEntry"
    :entry-id="editingEntry.id"
    @close="editingEntry = null"
  />
  <EntryViewModal
    v-if="viewingEntry"
    :entry-id="viewingEntry.id"
    @close="viewingEntry = null"
    @edit="switchToEdit"
    @navigate="navigateTo"
  />
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { ChevronDown } from 'lucide-vue-next'
import EntryRow from '../components/EntryRow.vue'
import EntryEditModal from '../components/EntryEditModal.vue'
import EntryViewModal from '../components/EntryViewModal.vue'
import { useEntriesStore } from '../stores/useEntriesStore.js'
import { useCategoriesStore } from '../stores/useCategoriesStore.js'
import { useProjectStore } from '../stores/useProjectStore.js'
import { resolveIcon } from '../utils/categoryIcons.js'
// Note: @navigate emits an entry id (string); @edit emits nothing (uses viewingEntry.value internally)

const route = useRoute()
const entries = useEntriesStore()
const cats = useCategoriesStore()
const projectStore = useProjectStore()

const projectId = computed(() => route.params.projectId ?? null)
const inProject = computed(() => !!projectId.value)
const project = computed(() => projectStore.projects.find(p => p.id === projectId.value) ?? null)

onMounted(async () => {
  await entries.init()
  if (inProject.value) {
    await Promise.all([
      cats.initForProject(projectId.value),
      projectStore.fetchProjects(),
    ])
  }
})

const collapsed = ref({})
const editingEntry = ref(null)
const viewingEntry = ref(null)

// Flat sorted list for global notes view
const visibleEntries = computed(() =>
  [...entries.entries].sort((a, b) => a.title.localeCompare(b.title))
)

// Grouped list for project view
const groupedEntries = computed(() => {
  if (!inProject.value) return []
  const order = cats.projectCategories.map(c => c.name)
  const map = {}
  for (const e of [...entries.entries].sort((a, b) => a.title.localeCompare(b.title))) {
    if (!map[e.category]) map[e.category] = []
    map[e.category].push(e)
  }
  const known = order.filter(name => map[name]?.length).map(name => ({ category: name, entries: map[name] }))
  const extra = Object.keys(map).filter(name => !order.includes(name)).map(name => ({ category: name, entries: map[name] }))
  return [...known, ...extra]
})

function toggle(category) {
  collapsed.value[category] = !collapsed.value[category]
}

function catIcon(categoryName) {
  const c = cats.projectCategoryFor(categoryName)
  return c ? resolveIcon(c.icon) : resolveIcon('')
}

function catColor(categoryName) {
  return cats.projectCategoryFor(categoryName)?.color ?? 'var(--text-muted)'
}

async function createNew() {
  if (inProject.value) {
    await cats.initForProject(projectId.value)
    const category = cats.projectCategories[0]?.name ?? 'Characters'
    const entry = await entries.createEntry(category)
    editingEntry.value = entry
  } else {
    const entry = await entries.createEntry('Characters')
    editingEntry.value = entry
  }
}

function openEdit(entry) {
  viewingEntry.value = null
  editingEntry.value = entry
}

function openView(entry) {
  editingEntry.value = null
  viewingEntry.value = entry
}

function switchToEdit() {
  if (!viewingEntry.value) return
  const e = viewingEntry.value
  viewingEntry.value = null
  editingEntry.value = e
}

function navigateTo(id) {
  const e = entries.getById(id)
  if (e) viewingEntry.value = e
}
</script>

<style scoped>
.notes-browser { display: flex; flex-direction: column; height: 100vh; max-width: 800px; margin: 0 auto; padding: 0 24px; }
.browser-header { display: flex; align-items: center; justify-content: space-between; padding: 28px 0 20px; flex-shrink: 0; }
.browser-title { font-family: var(--font-display); font-size: 1.1em; letter-spacing: 0.06em; color: var(--text-muted); }
.new-btn { background: var(--accent-dim); border: none; color: var(--accent); font-size: 0.82em; padding: 5px 12px; border-radius: var(--radius); cursor: pointer; transition: background var(--transition); }
.new-btn:hover { background: var(--accent); color: var(--bg-deep); }
.browser-scroll { flex: 1; overflow-y: auto; }
.browser-empty { color: var(--text-faint); font-size: 0.9em; padding: 32px 0; }
.category-group { margin-bottom: 4px; }
.cat-group-header { display: flex; align-items: center; gap: 8px; width: 100%; padding: 8px 12px; background: none; border: none; cursor: pointer; border-radius: var(--radius); font-family: var(--font-body); font-size: 0.82em; font-weight: 600; letter-spacing: 0.04em; text-transform: uppercase; transition: background var(--transition); }
.cat-group-header:hover { background: var(--bg-hover); }
.cat-group-name { flex: 1; text-align: left; }
.cat-group-count { font-size: 0.75em; font-weight: normal; color: var(--text-faint); background: var(--bg-raised); border-radius: 10px; padding: 1px 7px; }
.chevron { color: var(--text-faint); transition: transform 0.15s ease; flex-shrink: 0; }
.chevron.rotated { transform: rotate(-90deg); }
.cat-group-entries { padding-bottom: 8px; }
</style>
```

Note: If `entries.create` doesn't exist by that exact name, check `useEntriesStore` and use whatever the create method is called.

- [ ] **Step 2: Commit**

```bash
git add quillit-ui/src/views/QuillitView.vue
git commit -m "feat(QuillitView): flat list outside project, grouped by project categories inside"
```

---

## Task 9: Hide category section in SideNav and category select in EntryEditor

**Files:**
- Modify: `quillit-ui/src/components/SideNav.vue`
- Modify: `quillit-ui/src/components/EntryEditor.vue`

- [ ] **Step 1: Update SideNav.vue**

Add `useRoute` import and wrap the category section with a `v-if`:

In the `<script setup>` section, add:
```js
import { computed } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
```

(Replace the existing `import { RouterLink } from 'vue-router'` line)

Add after `const auth = useAuthStore()`:
```js
const route = useRoute()
const inProject = computed(() => !!route.params.projectId)
```

In the template, change the category section opening tag:
```html
    <div class="nav-section nav-section--cats" v-if="inProject">
```

Also update the category button to use `cats.projectCategories` instead of `cats.categories`:
```html
      <button
        v-for="cat in cats.projectCategories"
        :key="cat.id"
```

And update the count reference — the entry count filter now uses the project category name:
```html
        :title="`${cat.name} (${entries.byCategory(cat.name).length})`"
```

(No change needed — `byCategory` already searches by name string.)

Remove the `onMounted(() => cats.init())` call — it loaded global categories that are no longer displayed in the sidebar. Project categories are loaded by `QuillitView` via `cats.initForProject()`. If `cats.init()` is the only thing in `onMounted`, remove the `onMounted` call entirely too.

- [ ] **Step 2: Update EntryEditor.vue**

Add `useRoute` and hide the category `<select>` outside project context.

In `EntryEditor.vue`'s `<script setup>`, add:
```js
import { useRoute } from 'vue-router'
const route = useRoute()
const inProject = computed(() => !!route.params.projectId)
```

(add `computed` to the Vue import if not already there)

In the template, wrap the category select with `v-if="inProject"`:
```html
        <select class="cat-select" v-if="inProject" v-model="localCategory" @change="save">
          <option v-for="c in cats.projectCategories" :key="c.id" :value="c.name">{{ c.icon }} {{ c.name }}</option>
        </select>
```

- [ ] **Step 3: Commit**

```bash
git add quillit-ui/src/components/SideNav.vue quillit-ui/src/components/EntryEditor.vue
git commit -m "feat(ui): hide category features outside project context in SideNav and EntryEditor"
```

---

## Task 10: Remove category tabs from DashboardView

**Files:**
- Modify: `quillit-ui/src/views/DashboardView.vue`

The dashboard is always outside a project context. Remove the category tab strip and the tag panel below it.

- [ ] **Step 1: Remove the cat-tabs block**

Delete these lines from the template (approximately lines 31–68):

```html
    <!-- Category tabs strip -->
    <div class="cat-tabs">
      <button
        v-for="cat in cats.categories"
        ...
      </button>
    </div>

    <!-- Tag chips for the active category -->
    <div class="cat-tag-panel" v-if="activeTab">
      ...
    </div>
```

- [ ] **Step 2: Remove dependent reactive state and computed properties from the script**

Remove these lines from `<script setup>`:

```js
const activeTab = ref(null)
```

```js
  if (!activeTab.value && cats.categories.length > 0) {
    activeTab.value = cats.categories[0].name
  }
```

```js
const tagsForActiveTab = computed(() => { ... })
const activeTabColor = computed(() => cats.categoryFor(activeTab.value)?.color ?? null)
const untaggedCount = computed(() => { ... })
```

- [ ] **Step 3: Remove `cats` entirely from DashboardView's script**

Since the dashboard no longer uses categories, remove these lines:

```js
import { useCategoriesStore } from '../stores/useCategoriesStore.js'
```

```js
const cats = useCategoriesStore()
```

```js
  await cats.init()
```

(inside `onMounted`). Also remove the `hexToAlpha` helper import/usage if it was only used by the category tab color styling — search the file for any remaining `hexToAlpha` calls.

- [ ] **Step 4: Remove cat-tabs and cat-tag-panel CSS from `<style scoped>`**

Delete the `.cat-tabs`, `.cat-tab`, `.cat-tab-name`, `.cat-tab-count`, `.cat-tag-panel`, `.tag-pill`, `.tp-label`, `.tp-count`, `.tag-pill-empty`, `.tag-pill--untagged` style blocks.

- [ ] **Step 5: Commit**

```bash
git add quillit-ui/src/views/DashboardView.vue
git commit -m "feat(dashboard): remove category tabs — categories are project-scoped only"
```

---

## Task 11: Fix invite auth flow

**Files:**
- Modify: `quillit-ui/src/views/LoginView.vue`
- Modify: `quillit-ui/src/views/SetupView.vue`

- [ ] **Step 1: Update LoginView to navigate to ?redirect after login**

Replace the `<script setup>` section of `LoginView.vue`:

```js
import { ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '../stores/useAuthStore.js'

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()
const email = ref('')
const password = ref('')
const error = ref('')
const loading = ref(false)

async function submit() {
  if (!email.value || !password.value) return
  loading.value = true
  error.value = ''
  try {
    await auth.login(email.value, password.value)
    router.push(route.query.redirect || '/')
  } catch {
    error.value = 'Invalid credentials'
  } finally {
    loading.value = false
  }
}
```

- [ ] **Step 2: Update SetupView to auto-join and redirect if already logged in**

Replace the `<script setup>` section of `SetupView.vue`:

```js
import { ref, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '../stores/useAuthStore.js'
import { useProjectStore } from '../stores/useProjectStore.js'

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()
const projects = useProjectStore()

const email = ref('')
const username = ref('')
const password = ref('')
const confirm = ref('')
const error = ref('')
const loading = ref(false)
const inviteToken = ref(null)

onMounted(async () => {
  inviteToken.value = route.query.invite ?? null
  await auth.fetchMe()

  if (auth.isLoggedIn) {
    if (inviteToken.value) {
      // Already logged in — redeem the invite and go straight to the project.
      try {
        const membership = await projects.join(inviteToken.value)
        router.push(`/projects/${membership.projectId}/notes`)
      } catch {
        router.push('/')
      }
    } else {
      router.push('/')
    }
  }
})

async function submit() {
  error.value = ''
  if (!email.value || !username.value || !password.value) {
    error.value = 'All fields are required'
    return
  }
  if (password.value.length < 8) {
    error.value = 'Password must be at least 8 characters'
    return
  }
  if (password.value !== confirm.value) {
    error.value = 'Passwords do not match'
    return
  }
  loading.value = true
  try {
    await auth.register(email.value, username.value, password.value)
    if (inviteToken.value) {
      try {
        const membership = await projects.join(inviteToken.value)
        router.push(`/projects/${membership.projectId}/notes`)
        return
      } catch {
        // Join failed — navigate home; user can join manually
      }
    }
    router.push('/')
  } catch (e) {
    error.value = e?.data?.error ?? 'Registration failed'
  } finally {
    loading.value = false
  }
}
```

- [ ] **Step 3: Verify `projects.join()` returns an object with `projectId`**

Check `useProjectStore.js` — the `join` function returns the raw API response. Verify the backend `Join` handler returns `ProjectMember` which has `projectId`. The existing `Join` handler in `projects.go` returns:

```go
writeJSON(w, http.StatusCreated, ProjectMember{
    ID: memberID, ProjectID: inv.ProjectID, ...
})
```

`ProjectID` serializes to `projectId` in JSON — correct. ✓

- [ ] **Step 4: Commit**

```bash
git add quillit-ui/src/views/LoginView.vue quillit-ui/src/views/SetupView.vue
git commit -m "fix(auth): preserve redirect param through login; auto-join invite for logged-in users"
```

---

## Verification

- [ ] **Invite flow (new user):** Open an incognito window, navigate to a `/register?invite=TOKEN` URL, complete registration → lands at `/projects/:id/notes`
- [ ] **Invite flow (logged-in user):** While logged in, navigate to `/register?invite=TOKEN` → no form shown, auto-joined, redirected to project notes
- [ ] **Invite flow (login path):** Open `/notes` while logged out → redirected to `/login?redirect=%2Fnotes` → after login → lands at `/notes`
- [ ] **Category visibility (global notes):** Navigate to `/notes` → no category section in sidebar, no category select in entry editor, flat alphabetical list
- [ ] **Category visibility (project notes):** Navigate to `/projects/:id/notes` → sidebar shows project categories, entry editor shows category select, notes are grouped by category
- [ ] **Project view:** Navigate to `/projects/:id` → see members list, search for a user, click Add → user appears in list
- [ ] **Invite link:** On `/projects/:id`, generate invite link → copy it → paste in incognito → register → lands on that project
- [ ] **Admin categories unchanged:** Navigate to `/admin` → category management still works (now operates on global categories only)
