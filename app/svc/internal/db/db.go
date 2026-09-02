package db

import (
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/hex"
	"fmt"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func newDBID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func Open(path string) (*sql.DB, error) {
	database, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	database.SetMaxOpenConns(1)
	if _, err = database.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("set journal mode: %w", err)
	}
	if _, err = database.Exec("PRAGMA foreign_keys=ON"); err != nil {
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	if err = migrate(database); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if err := checkForeignKeys(database); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return database, nil
}

// migrate runs every pending goose migration in internal/db/migrations
// (embedded via migrationsFS) against database, up to the latest version.
//
// Before doing so it guards against a precondition the baseline migration
// depends on but can't enforce on its own: it's written entirely as
// idempotent CREATE ... IF NOT EXISTS / INSERT OR IGNORE statements, which is
// correct for a genuinely empty database (fresh goose install, PRAGMA
// user_version stays 0 — goose tracks its own progress via goose_db_version,
// not user_version) or one already at the old hand-rolled system's v8 end
// state (PRAGMA user_version = 8). But a database still at the old system's
// user_version 1-7 — i.e. one that was never cut over to v8 — already has
// some of these tables (e.g. project_members without its later toV2
// `username` column) in a shape the IF NOT EXISTS statements will silently
// no-op against, producing a wrong schema instead of an error.
func migrate(database *sql.DB) error {
	var userVersion int
	if err := database.QueryRow(`PRAGMA user_version`).Scan(&userVersion); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}
	if userVersion > 0 && userVersion < 8 {
		return fmt.Errorf("database is at legacy schema v%d; the goose baseline only "+
			"applies to an empty database or one already at the old system's v8 end state",
			userVersion)
	}

	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("set dialect: %w", err)
	}
	if err := goose.Up(database, "migrations"); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
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
