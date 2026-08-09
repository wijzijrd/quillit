package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec("PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;"); err != nil {
		return nil, fmt.Errorf("pragma: %w", err)
	}
	if err = migrate(db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if err := checkForeignKeys(db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

// checkForeignKeys fails fast with a clear error if a migration left the schema
// with a dangling foreign key reference, instead of surfacing later as a
// confusing runtime error from whatever query happens to hit it first.
func checkForeignKeys(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("foreign_key_check: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		return fmt.Errorf("post-migration integrity check failed: schema has a dangling foreign key (see PRAGMA foreign_key_check)")
	}
	return rows.Err()
}

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

// toV1 establishes the initial schema.
// Roles are account-level: 'user' (default) or 'admin'.
// Project-level roles (gm/player, author/collaborator) live in quillit-svc.
// The active flag allows admins to disable accounts without deleting them.
func toV1(db *sql.DB) error {
	// Check whether the users table already exists with the old schema.
	var tableCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='users'`,
	).Scan(&tableCount); err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if tableCount > 0 {
		// Existing install: recreate with new role constraint + active column.
		// FK enforcement is OFF during DDL to allow DROP/RENAME safely.
		if _, err := tx.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
			return err
		}
		if _, err := tx.Exec(`
			CREATE TABLE users_new (
				id            TEXT    PRIMARY KEY,
				email         TEXT    NOT NULL UNIQUE,
				username      TEXT    NOT NULL UNIQUE,
				password_hash TEXT    NOT NULL,
				role          TEXT    NOT NULL DEFAULT 'user'
				              CHECK (role IN ('user', 'admin')),
				active        INTEGER NOT NULL DEFAULT 1,
				created_at    INTEGER NOT NULL,
				updated_at    INTEGER NOT NULL
			)
		`); err != nil {
			return err
		}
		if _, err := tx.Exec(`
			INSERT INTO users_new
				(id, email, username, password_hash, role, active, created_at, updated_at)
			SELECT
				id, email, username, password_hash,
				CASE WHEN role = 'admin' THEN 'admin' ELSE 'user' END,
				1, created_at, updated_at
			FROM users
		`); err != nil {
			return err
		}
		if _, err := tx.Exec(`DROP TABLE users`); err != nil {
			return err
		}
		if _, err := tx.Exec(`ALTER TABLE users_new RENAME TO users`); err != nil {
			return err
		}
		if _, err := tx.Exec(`PRAGMA foreign_keys = ON`); err != nil {
			return err
		}
	} else {
		// Fresh install.
		if _, err := tx.Exec(`
			CREATE TABLE IF NOT EXISTS users (
				id            TEXT    PRIMARY KEY,
				email         TEXT    NOT NULL UNIQUE,
				username      TEXT    NOT NULL UNIQUE,
				password_hash TEXT    NOT NULL,
				role          TEXT    NOT NULL DEFAULT 'user'
				              CHECK (role IN ('user', 'admin')),
				active        INTEGER NOT NULL DEFAULT 1,
				created_at    INTEGER NOT NULL,
				updated_at    INTEGER NOT NULL
			)
		`); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(`PRAGMA user_version = 1`); err != nil {
		return err
	}
	return tx.Commit()
}

// toV2 adds password reset tokens: single-use, short-lived, keyed to a user.
func toV2(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS password_reset_tokens (
			id         TEXT    PRIMARY KEY,
			user_id    TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			token_hash TEXT    NOT NULL UNIQUE,
			expires_at INTEGER NOT NULL,
			used       INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL
		)
	`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_user ON password_reset_tokens(user_id)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`PRAGMA user_version = 2`); err != nil {
		return err
	}
	return tx.Commit()
}
