package db

import "testing"

func TestOpen_FreshDatabase(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer database.Close()

	var version int
	if err := database.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != 3 {
		t.Errorf("user_version = %d, want 3", version)
	}

	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM schema_meta`).Scan(&count); err != nil {
		t.Fatalf("count schema_meta: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 schema_meta row, got %d", count)
	}
}

func TestOpen_CreatesSearchIndex(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer database.Close()

	if _, err := database.Exec(`INSERT INTO entries_fts (entry_id, title, tags, body) VALUES (?, ?, ?, ?)`,
		"e1", "Tom the Innkeeper", "npc", "Runs the tavern."); err != nil {
		t.Fatalf("insert into entries_fts failed: %v", err)
	}

	var entryID string
	if err := database.QueryRow(`SELECT entry_id FROM entries_fts WHERE entries_fts MATCH 'tavern'`).Scan(&entryID); err != nil {
		t.Fatalf("FTS5 MATCH query failed: %v", err)
	}
	if entryID != "e1" {
		t.Errorf("entry_id = %q, want %q", entryID, "e1")
	}
}

func TestOpen_SeedsGlobalFacets(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer database.Close()

	for _, name := range []string{"motivation", "description", "history"} {
		var count int
		if err := database.QueryRow(`SELECT COUNT(*) FROM facets WHERE name = ?`, name).Scan(&count); err != nil {
			t.Fatalf("count facets(%s): %v", name, err)
		}
		if count != 1 {
			t.Errorf("facet %q: expected 1 row, got %d", name, count)
		}
	}
}

func TestOpen_EntriesUniqueSlugPerDirectory(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer database.Close()

	insert := `INSERT INTO entries (id, project_id, slug, directory_path, created_at, updated_at) VALUES (?, ?, ?, ?, 0, 0)`
	if _, err := database.Exec(insert, "e1", "proj-a", "mary", "characters/npcs"); err != nil {
		t.Fatalf("first insert failed: %v", err)
	}
	if _, err := database.Exec(insert, "e2", "proj-a", "mary", "characters/npcs"); err == nil {
		t.Error("expected UNIQUE constraint violation for duplicate (project_id, directory_path, slug), got nil")
	}
	// Same slug, different project — allowed.
	if _, err := database.Exec(insert, "e3", "proj-b", "mary", "characters/npcs"); err != nil {
		t.Errorf("insert into different project should succeed: %v", err)
	}
}

func TestOpen_IsIdempotent(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("first Open() failed: %v", err)
	}
	defer database.Close()

	// Running migrate() again against the same handle must not error or
	// duplicate the schema_meta row.
	if err := migrate(database); err != nil {
		t.Fatalf("second migrate() failed: %v", err)
	}
	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM schema_meta`).Scan(&count); err != nil {
		t.Fatalf("count schema_meta: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 schema_meta row after re-migrate, got %d", count)
	}
}
