package db

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

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

// migrate runs every pending goose migration in internal/db/migrations
// (embedded via migrationsFS) against db, up to the latest version.
//
// Before doing so it guards against a precondition the baseline migration
// depends on but can't enforce on its own: it's written entirely as
// idempotent CREATE ... IF NOT EXISTS statements, which is correct for a
// genuinely empty database (fresh goose install, PRAGMA user_version stays 0
// — goose tracks its own progress via goose_db_version, not user_version) or
// one already at the old hand-rolled system's v2 end state (PRAGMA
// user_version = 2). A database still at the old system's user_version 1
// (i.e. one that ran toV1 but never toV2) must be rejected instead of
// silently trusted — see 00001_baseline.sql's header comment for why this
// guard is kept even though today's specific two-step chain happens not to
// need it for correctness.
func migrate(db *sql.DB) error {
	var userVersion int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&userVersion); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}
	if userVersion > 0 && userVersion < 2 {
		return fmt.Errorf("database is at legacy schema v%d; the goose baseline only "+
			"applies to an empty database or one already at the old system's v2 end state",
			userVersion)
	}

	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("set dialect: %w", err)
	}
	if err := goose.Up(db, "migrations"); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}
