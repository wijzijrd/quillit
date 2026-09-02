package db

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func tableExists(t *testing.T, database *sql.DB, table string) bool {
	t.Helper()
	var name string
	err := database.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name = ?`, table).Scan(&name)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		t.Fatal(err)
	}
	return true
}

func TestOpen_FreshDatabase(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer database.Close()

	if err := checkForeignKeys(database); err != nil {
		t.Errorf("checkForeignKeys after fresh Open(): %v", err)
	}

	if !tableExists(t, database, "users") {
		t.Error("expected users table to exist after a fresh Open()")
	}
	if !tableExists(t, database, "password_reset_tokens") {
		t.Error("expected password_reset_tokens table to exist after a fresh Open()")
	}

	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 users on fresh install, got %d", count)
	}
}

// TestOpen_IdempotentOnAlreadyMigratedDB confirms the baseline migration has
// zero schema effect when run a second time against a database that already
// ran it — the exact scenario the real production database will hit when
// this goose-based system first runs against it (it's already at the old
// system's v2 end state). Uses a real file (not :memory:) so the second
// Open() reopens genuinely persisted state rather than a fresh in-memory db.
func TestOpen_IdempotentOnAlreadyMigratedDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quillit-auth.db")

	first, err := Open(path)
	if err != nil {
		t.Fatalf("first Open() failed: %v", err)
	}
	var firstCount int
	if err := first.QueryRow(`SELECT COUNT(*) FROM goose_db_version`).Scan(&firstCount); err != nil {
		t.Fatalf("query goose_db_version after first Open(): %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := Open(path)
	if err != nil {
		t.Fatalf("second Open() (against an already-migrated db) failed: %v", err)
	}
	defer second.Close()

	if err := checkForeignKeys(second); err != nil {
		t.Errorf("checkForeignKeys after second Open(): %v", err)
	}

	var secondCount int
	if err := second.QueryRow(`SELECT COUNT(*) FROM goose_db_version`).Scan(&secondCount); err != nil {
		t.Fatalf("query goose_db_version after second Open(): %v", err)
	}
	if secondCount != firstCount {
		t.Errorf("expected re-running the migration against an already-migrated db to record no new goose version rows, got %d rows before vs %d after", firstCount, secondCount)
	}

	if !tableExists(t, second, "users") {
		t.Error("expected users table to still exist after second Open()")
	}
	if !tableExists(t, second, "password_reset_tokens") {
		t.Error("expected password_reset_tokens table to still exist after second Open()")
	}
}

// TestOpen_AgainstRealProductionShapedDB is the actual production scenario
// this migration must handle safely: a database that already reached the
// old hand-rolled system's v2 end state (built here with raw SQL matching
// that end state exactly, with no goose_db_version table at all — this is
// what a real deployed auth database looks like today, since it has never
// run goose). The baseline migration must apply against it with zero schema
// effect and without touching existing data.
func TestOpen_AgainstRealProductionShapedDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quillit-auth.db")
	pre, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pre.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		t.Fatal(err)
	}

	now := time.Now().Unix()
	for _, stmt := range []string{
		`CREATE TABLE users (
			id TEXT PRIMARY KEY, email TEXT NOT NULL UNIQUE, username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin')),
			active INTEGER NOT NULL DEFAULT 1,
			created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL
		)`,
		`CREATE TABLE password_reset_tokens (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			token_hash TEXT NOT NULL UNIQUE, expires_at INTEGER NOT NULL,
			used INTEGER NOT NULL DEFAULT 0, created_at INTEGER NOT NULL
		)`,
		`CREATE INDEX idx_password_reset_tokens_user ON password_reset_tokens(user_id)`,
	} {
		if _, err := pre.Exec(stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
	if _, err := pre.Exec(
		`INSERT INTO users (id, email, username, password_hash, role, active, created_at, updated_at) VALUES ('u1', 'real@example.com', 'realuser', 'hash', 'user', 1, ?, ?)`,
		now, now,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pre.Exec(`PRAGMA user_version = 2`); err != nil {
		t.Fatal(err)
	}
	if err := pre.Close(); err != nil {
		t.Fatal(err)
	}

	database, err := Open(path)
	if err != nil {
		t.Fatalf("Open() against a real-production-shaped pre-goose db failed: %v", err)
	}
	defer database.Close()

	if err := checkForeignKeys(database); err != nil {
		t.Errorf("checkForeignKeys: %v", err)
	}

	var userCount int
	if err := database.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&userCount); err != nil {
		t.Fatal(err)
	}
	if userCount != 1 {
		t.Errorf("expected the migration to leave the 1 pre-existing user untouched, got count=%d", userCount)
	}

	var email string
	if err := database.QueryRow(`SELECT email FROM users WHERE id = 'u1'`).Scan(&email); err != nil {
		t.Fatalf("pre-existing user data lost: %v", err)
	}
	if email != "real@example.com" {
		t.Errorf("pre-existing user data corrupted: got email=%q", email)
	}
}

// TestOpen_RejectsLegacyPreV2Database guards the precondition the baseline
// migration depends on but can't enforce with SQL alone: a database still at
// the old hand-rolled system's user_version 1 (ran toV1 but never toV2) must
// be rejected with a clear error rather than silently trusted. See
// 00001_baseline.sql's header comment for the (today, accidental) reason
// applying the baseline here would actually be safe anyway — the guard is
// kept regardless, since that's a property of today's specific two-step
// chain, not something the guard should assume holds forever.
func TestOpen_RejectsLegacyPreV2Database(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quillit-auth.db")
	pre, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	// A minimal legacy-shaped table is enough: the guard fires on
	// PRAGMA user_version alone, not on inspecting this table's columns.
	if _, err := pre.Exec(`CREATE TABLE users (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, err := pre.Exec(`PRAGMA user_version = 1`); err != nil {
		t.Fatal(err)
	}
	if err := pre.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = Open(path)
	if err == nil {
		t.Fatal("expected Open() to reject a database at legacy user_version 1, got nil error")
	}
	if !strings.Contains(err.Error(), "legacy schema v1") {
		t.Errorf("expected error to mention the legacy schema version, got: %v", err)
	}
}

func TestOpen_CreatesPasswordResetTokensTable(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer database.Close()

	if !tableExists(t, database, "password_reset_tokens") {
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
