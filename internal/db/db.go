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
	return db, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id            TEXT    PRIMARY KEY,
			email         TEXT    NOT NULL UNIQUE,
			username      TEXT    NOT NULL UNIQUE,
			password_hash TEXT    NOT NULL,
			role          TEXT    NOT NULL DEFAULT 'gm'
			              CHECK (role IN ('gm', 'player', 'admin')),
			created_at    INTEGER NOT NULL,
			updated_at    INTEGER NOT NULL
		)
	`)
	return err
}
