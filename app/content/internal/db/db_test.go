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
	if version != 1 {
		t.Errorf("user_version = %d, want 1", version)
	}

	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM schema_meta`).Scan(&count); err != nil {
		t.Fatalf("count schema_meta: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 schema_meta row, got %d", count)
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
