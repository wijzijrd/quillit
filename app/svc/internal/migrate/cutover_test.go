package migrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestContentClient_CreateEntry_Success(t *testing.T) {
	var gotPath string
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := &ContentClient{BaseURL: srv.URL}
	err := c.CreateEntry(context.Background(), "proj-1", "mary", "characters/npcs", "# Mary\n")
	if err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}
	if gotPath != "/content/projects/proj-1/entries" {
		t.Errorf("path = %q, want /content/projects/proj-1/entries", gotPath)
	}
	if gotBody["slug"] != "mary" || gotBody["directoryPath"] != "characters/npcs" || gotBody["body"] != "# Mary\n" {
		t.Errorf("unexpected request body: %+v", gotBody)
	}
}

func TestContentClient_CreateEntry_EscapesProjectIDInPath(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := &ContentClient{BaseURL: srv.URL}
	err := c.CreateEntry(context.Background(), "weird/id x", "mary", "", "body")
	if err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}
	if want := "/content/projects/weird%2Fid%20x/entries"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
}

func TestContentClient_CreateEntry_ConflictIsAlreadyExists(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))
	defer srv.Close()

	c := &ContentClient{BaseURL: srv.URL}
	err := c.CreateEntry(context.Background(), "proj-1", "mary", "", "body")
	if !errors.Is(err, ErrAlreadyExists) {
		t.Errorf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestContentClient_CreateEntry_OtherErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}))
	defer srv.Close()

	c := &ContentClient{BaseURL: srv.URL}
	err := c.CreateEntry(context.Background(), "proj-1", "mary", "", "body")
	if err == nil || errors.Is(err, ErrAlreadyExists) {
		t.Errorf("expected a plain error for 422, got %v", err)
	}
}

type fakeImporter struct {
	calls []struct{ projectID, slug, dir, body string }
	err   error // returned for every call when set
}

func (f *fakeImporter) CreateEntry(_ context.Context, projectID, slug, dir, body string) error {
	f.calls = append(f.calls, struct{ projectID, slug, dir, body string }{projectID, slug, dir, body})
	return f.err
}

func seedSingleCleanEntry(t *testing.T, database *sql.DB) {
	t.Helper()
	insertEntry(t, database, "e1", "Mary", "Characters", "<p>An innkeeper.</p>", "", "[\"proj1\"]", "[]", "{}")
}

func seedCutoverFixture(t *testing.T, database *sql.DB) {
	t.Helper()
	// One clean entry
	insertEntry(t, database, "e1", "Mary", "Characters", "<p>An innkeeper.</p>", "", "[\"proj1\"]", "[]", "{}")

	// One entry with flags (image without resolvable src)
	insertEntry(t, database, "e2", "Treasure Map", "Lore", "<p>Map at <img src=\"/missing.png\"/></p>", "", "[\"proj1\"]", "[]", "{}")

	// One entry that fails parser validation (malformed card block)
	insertEntry(t, database, "e3", "Bad Entry", "Characters", "<p>hi</p>", "", "[]", "[]", "{}")
	if _, err := database.Exec(`UPDATE entries SET body = '<p>' || ':::card nonexistent-facet' || char(10) || 'x' || char(10) || ':::' || '</p>' WHERE id = 'e3'`); err != nil {
		t.Fatalf("seed malformed body: %v", err)
	}
}

func TestCutover_ImportsCleanAndFlaggedSkipsFailed(t *testing.T) {
	database := openTestDB(t)
	seedCutoverFixture(t, database)

	f := &fakeImporter{}
	results, err := Cutover(context.Background(), database, nil, f)
	if err != nil {
		t.Fatalf("Cutover: %v", err)
	}

	var imported, skipped int
	for _, r := range results {
		switch r.Status {
		case "imported":
			imported++
		case "skipped-failed":
			skipped++
		}
	}
	if imported != 2 {
		t.Errorf("expected 2 imported (clean+flagged), got %d", imported)
	}
	if skipped != 1 {
		t.Errorf("expected 1 skipped-failed, got %d", skipped)
	}
	if len(f.calls) != 2 {
		t.Errorf("expected importer called exactly twice, got %d", len(f.calls))
	}
}

func TestCutover_NoProjectEntrySkippedDistinctly(t *testing.T) {
	database := openTestDB(t)
	// campaign_ids "[]" -> no project under the legacy model (personal/
	// session-note entry); body is clean so it wouldn't fail conversion.
	insertEntry(t, database, "e1", "Mary", "Characters", "<p>An innkeeper.</p>", "", "[]", "[]", "{}")

	f := &fakeImporter{}
	results, err := Cutover(context.Background(), database, nil, f)
	if err != nil {
		t.Fatalf("Cutover: %v", err)
	}
	if len(results) != 1 || results[0].Status != "skipped-no-project" {
		t.Errorf("expected a no-project entry to be skipped distinctly, got %+v", results)
	}
	if len(f.calls) != 0 {
		t.Errorf("expected importer not called for a no-project entry, got %d calls", len(f.calls))
	}
}

func TestCutover_ConflictCountsAsImported(t *testing.T) {
	database := openTestDB(t)
	seedSingleCleanEntry(t, database)

	f := &fakeImporter{err: ErrAlreadyExists}
	results, err := Cutover(context.Background(), database, nil, f)
	if err != nil {
		t.Fatalf("Cutover: %v", err)
	}
	if len(results) != 1 || results[0].Status != "imported" {
		t.Errorf("expected a 409 to be reported as imported, got %+v", results)
	}
}
