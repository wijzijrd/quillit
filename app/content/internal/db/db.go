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

// Open opens (creating if necessary) the SQLite database at path, enables
// WAL mode and foreign key enforcement, and runs any pending migrations.
func Open(path string) (*sql.DB, error) {
	database, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	database.SetMaxOpenConns(1)
	if _, err = database.Exec("PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;"); err != nil {
		return nil, fmt.Errorf("pragma: %w", err)
	}
	if err = migrate(database); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return database, nil
}

// migrate applies pending goose migrations from the embedded migrations
// directory (see internal/db/migrations/00001_baseline.sql).
//
// Before running goose, it guards against the old hand-rolled
// PRAGMA-user_version migration chain (toV1..toV5, deleted — see git
// history) ever running partway. The baseline migration is written entirely
// as idempotent CREATE ... IF NOT EXISTS / INSERT OR IGNORE statements,
// which only produces a correct schema against (a) an empty database
// (user_version 0 — goose tracks its own progress via a goose_db_version
// table and never touches user_version) or (b) a database already at the
// old system's v5 end state. A database caught mid-chain (user_version
// 1-4) would silently end up with a stale schema instead — e.g.
// entries.orphaned_at/body_hash (added by toV4/toV5 via ALTER TABLE, not
// CREATE) would never get added, because CREATE TABLE IF NOT EXISTS entries
// no-ops against the table's older shape. Unlike auth's equivalent guard,
// this one IS load-bearing here, not merely defensive: toV4/toV5 added
// orphaned_at/body_hash to entries (a table toV2 already created) via
// ALTER TABLE, so a database frozen at user_version 1-4 has entries without
// those columns, and the baseline's CREATE TABLE IF NOT EXISTS entries
// would silently skip adding them — a real schema-corruption mode this
// guard exists specifically to prevent.
func migrate(database *sql.DB) error {
	var userVersion int
	if err := database.QueryRow(`PRAGMA user_version`).Scan(&userVersion); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}
	if userVersion > 0 && userVersion < 5 {
		return fmt.Errorf("database is at legacy schema v%d; the goose baseline only "+
			"applies to an empty database or one already at the old system's v5 end state",
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
