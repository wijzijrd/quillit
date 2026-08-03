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

// TestDeleteUser_CascadesPasswordResetTokens pins that auth's DeleteUser
// handler (a plain `DELETE FROM users WHERE id = ?`, no manual cleanup of
// related rows) does not fail with a foreign-key violation when a user has
// an outstanding password_reset_tokens row — the ON DELETE CASCADE on that
// table's user_id reference must delete the token row along with the user.
func TestDeleteUser_CascadesPasswordResetTokens(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer database.Close()

	if _, err := database.Exec(
		`INSERT INTO users (id, email, username, password_hash, role, created_at, updated_at) VALUES (?, ?, ?, ?, 'user', ?, ?)`,
		"u1", "u1@example.com", "u1", "hash", 0, 0,
	); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO password_reset_tokens (id, user_id, token_hash, expires_at, used, created_at) VALUES (?, ?, ?, ?, 0, ?)`,
		"t1", "u1", "hash1", 9999999999, 0,
	); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	if _, err := database.Exec(`DELETE FROM users WHERE id = ?`, "u1"); err != nil {
		t.Fatalf("delete user with outstanding reset token should not fail (expected ON DELETE CASCADE): %v", err)
	}

	var tokenCount int
	if err := database.QueryRow(`SELECT COUNT(*) FROM password_reset_tokens WHERE id = ?`, "t1").Scan(&tokenCount); err != nil {
		t.Fatalf("count tokens: %v", err)
	}
	if tokenCount != 0 {
		t.Errorf("expected the user's reset token to be cascade-deleted, but %d row(s) remain", tokenCount)
	}
}
