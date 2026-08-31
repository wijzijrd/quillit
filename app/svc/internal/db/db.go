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
func migrate(database *sql.DB) error {
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
