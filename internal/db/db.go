package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

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
	if _, err = database.Exec("PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;"); err != nil {
		return nil, fmt.Errorf("pragma: %w", err)
	}
	if err = migrate(database); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if err = seedCategories(database); err != nil {
		return nil, fmt.Errorf("seed categories: %w", err)
	}
	if err = migrateNPCToCharacters(database); err != nil {
		return nil, fmt.Errorf("migrate npc: %w", err)
	}
	return database, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			id         TEXT    PRIMARY KEY,
			jwt        TEXT    NOT NULL,
			expires_at INTEGER NOT NULL,
			created_at INTEGER NOT NULL
		);

		CREATE TABLE IF NOT EXISTS campaigns (
			id         TEXT    PRIMARY KEY,
			name       TEXT    NOT NULL,
			created_at INTEGER NOT NULL
		);

		CREATE TABLE IF NOT EXISTS players (
			id          TEXT    PRIMARY KEY,
			campaign_id TEXT    NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
			name        TEXT    NOT NULL,
			token       TEXT    NOT NULL UNIQUE,
			created_at  INTEGER NOT NULL
		);

		CREATE TABLE IF NOT EXISTS entries (
			id              TEXT    PRIMARY KEY,
			title           TEXT    NOT NULL DEFAULT 'Untitled Entry',
			category        TEXT    NOT NULL DEFAULT 'Lore',
			body            TEXT    NOT NULL DEFAULT '',
			visibility      TEXT    NOT NULL DEFAULT 'private',
			campaign_ids    TEXT    NOT NULL DEFAULT '[]',
			linked_entries  TEXT    NOT NULL DEFAULT '[]',
			tags            TEXT    NOT NULL DEFAULT '[]',
			quick_view_data TEXT    NOT NULL DEFAULT '{}',
			created_at      INTEGER NOT NULL,
			updated_at      INTEGER NOT NULL
		);

		CREATE TABLE IF NOT EXISTS annotations (
			id          TEXT    PRIMARY KEY,
			entry_id    TEXT    NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
			text        TEXT    NOT NULL DEFAULT '',
			visibility  TEXT    NOT NULL DEFAULT 'gm',
			shared_with TEXT    NOT NULL DEFAULT '[]',
			created_at  INTEGER NOT NULL,
			updated_at  INTEGER NOT NULL
		);

		CREATE TABLE IF NOT EXISTS quick_view_templates (
			category TEXT PRIMARY KEY,
			fields   TEXT NOT NULL DEFAULT '[]'
		);

		CREATE TABLE IF NOT EXISTS player_notes (
			id         TEXT    PRIMARY KEY,
			token      TEXT    NOT NULL,
			title      TEXT    NOT NULL DEFAULT '',
			body       TEXT    NOT NULL DEFAULT '',
			category   TEXT    NOT NULL DEFAULT 'Note',
			visibility TEXT    NOT NULL DEFAULT 'private',
			tags       TEXT    NOT NULL DEFAULT '[]',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);

		CREATE TABLE IF NOT EXISTS categories (
			id         TEXT    PRIMARY KEY,
			name       TEXT    NOT NULL UNIQUE,
			icon       TEXT    NOT NULL DEFAULT '',
			color      TEXT    NOT NULL DEFAULT '',
			sort_order INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);

		CREATE TABLE IF NOT EXISTS category_default_tags (
			id          TEXT    PRIMARY KEY,
			category_id TEXT    NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
			label       TEXT    NOT NULL,
			sort_order  INTEGER NOT NULL DEFAULT 0
		);

		CREATE TABLE IF NOT EXISTS entry_relations (
			id         TEXT    PRIMARY KEY,
			from_id    TEXT    NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
			to_id      TEXT    NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
			label      TEXT    NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			UNIQUE(from_id, to_id, label)
		);
	`)
	return err
}

func seedCategories(db *sql.DB) error {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM categories`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	now := time.Now().Unix()
	type catSeed struct {
		name  string
		icon  string
		color string
		tags  []string
	}
	seeds := []catSeed{
		{"Characters", "User", "#4a7a9b", []string{"NPC", "PC", "Deity", "Monster", "Ally", "Enemy"}},
		{"Location", "MapPin", "#5a8a5a", []string{"City", "Dungeon", "Wilderness", "Landmark", "Region"}},
		{"Faction", "Shield", "#8a5a9b", []string{"Guild", "Government", "Religion", "Criminal", "Military"}},
		{"Event", "CalendarDays", "#9b7a3a", []string{"Battle", "Ceremony", "Discovery", "Betrayal", "Plot Thread"}},
		{"Item", "Package", "#9b5a5a", []string{"Weapon", "Armour", "Artefact", "Consumable", "Key Item"}},
		{"Lore", "BookMarked", "#6a7a9b", []string{"History", "Mythology", "Religion", "Plot Thread", "Rumour"}},
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, s := range seeds {
		catID := newDBID()
		if _, err := tx.Exec(
			`INSERT INTO categories (id, name, icon, color, sort_order, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			catID, s.name, s.icon, s.color, i, now, now,
		); err != nil {
			return err
		}
		for j, label := range s.tags {
			if _, err := tx.Exec(
				`INSERT INTO category_default_tags (id, category_id, label, sort_order) VALUES (?, ?, ?, ?)`,
				newDBID(), catID, label, j,
			); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func migrateNPCToCharacters(db *sql.DB) error {
	_, err := db.Exec(`UPDATE entries SET category = 'Characters' WHERE category = 'NPC'`)
	return err
}
