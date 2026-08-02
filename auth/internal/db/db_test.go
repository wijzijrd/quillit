package db

import "testing"

func TestOpen_FreshDatabase(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer database.Close()

	if err := checkForeignKeys(database); err != nil {
		t.Errorf("checkForeignKeys after fresh Open(): %v", err)
	}

	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 users on fresh install, got %d", count)
	}
}

func TestOpen_CreatesPasswordResetTokensTable(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer database.Close()

	var tableCount int
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='password_reset_tokens'`,
	).Scan(&tableCount); err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if tableCount != 1 {
		t.Errorf("expected password_reset_tokens table to exist")
	}
}
