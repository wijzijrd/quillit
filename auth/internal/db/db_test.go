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
