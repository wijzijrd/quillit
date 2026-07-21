package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
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
	if err = seedCategories(database); err != nil {
		return nil, fmt.Errorf("seed categories: %w", err)
	}
	if err = migrateNPCToCharacters(database); err != nil {
		return nil, fmt.Errorf("migrate npc: %w", err)
	}
	return database, nil
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
	if version < 3 {
		if err := toV3(db); err != nil {
			return fmt.Errorf("schema v3: %w", err)
		}
	}
	if version < 4 {
		if err := toV4(db); err != nil {
			return fmt.Errorf("schema v4: %w", err)
		}
	}
	if version < 5 {
		if err := toV5(db); err != nil {
			return fmt.Errorf("schema v5: %w", err)
		}
	}
	if version < 6 {
		if err := toV6(db); err != nil {
			return fmt.Errorf("schema v6: %w", err)
		}
	}
	if version < 7 {
		if err := toV7(db); err != nil {
			return fmt.Errorf("schema v7: %w", err)
		}
	}
	return nil
}

// toV1 creates the original schema (sessions, campaigns, players, entries, etc.)
// plus the new project-system tables introduced in phase 2.
// For existing installs the original tables already exist (CREATE TABLE IF NOT EXISTS
// is a no-op); only the new tables and added columns are applied.
func toV1(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// ── Original tables (idempotent) ──────────────────────────────────────────

	if _, err := tx.Exec(`
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
	`); err != nil {
		return err
	}

	// ── New project-system tables ─────────────────────────────────────────────

	if _, err := tx.Exec(`
		-- Typed project container (replaces campaigns over time).
		-- type: 'campaign', 'book', ... determines role labels in the UI.
		CREATE TABLE IF NOT EXISTS projects (
			id         TEXT    PRIMARY KEY,
			name       TEXT    NOT NULL,
			type       TEXT    NOT NULL DEFAULT 'campaign',
			created_by TEXT    NOT NULL,
			created_at INTEGER NOT NULL
		);

		-- Per-project membership. Role name depends on project type:
		-- campaign → 'gm' / 'player'; book → 'author' / 'collaborator'.
		CREATE TABLE IF NOT EXISTS project_members (
			id         TEXT    PRIMARY KEY,
			project_id TEXT    NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			user_id    TEXT    NOT NULL,
			role       TEXT    NOT NULL,
			joined_at  INTEGER NOT NULL,
			UNIQUE(project_id, user_id)
		);

		-- Explicit per-entry access grants (replaces blunt visibility flag).
		CREATE TABLE IF NOT EXISTS entry_shares (
			id        TEXT    PRIMARY KEY,
			entry_id  TEXT    NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
			user_id   TEXT    NOT NULL,
			shared_by TEXT    NOT NULL,
			shared_at INTEGER NOT NULL,
			UNIQUE(entry_id, user_id)
		);

		-- GM-generated invite links for joining a project.
		CREATE TABLE IF NOT EXISTS project_invites (
			id         TEXT    PRIMARY KEY,
			token      TEXT    NOT NULL UNIQUE,
			project_id TEXT    NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			role       TEXT    NOT NULL,
			created_by TEXT    NOT NULL,
			expires_at INTEGER NOT NULL,
			used_at    INTEGER,
			used_by    TEXT
		);

		-- Member-created folders for organising shared notes.
		CREATE TABLE IF NOT EXISTS member_folders (
			id         TEXT    PRIMARY KEY,
			user_id    TEXT    NOT NULL,
			project_id TEXT    REFERENCES projects(id) ON DELETE SET NULL,
			name       TEXT    NOT NULL,
			color      TEXT    NOT NULL DEFAULT '',
			sort_order INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL
		);

		-- Junction: which entries belong to which member folder.
		CREATE TABLE IF NOT EXISTS member_folder_entries (
			folder_id TEXT NOT NULL REFERENCES member_folders(id) ON DELETE CASCADE,
			entry_id  TEXT NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
			PRIMARY KEY (folder_id, entry_id)
		);

		-- Per-member personal metadata for any entry they can see.
		CREATE TABLE IF NOT EXISTS member_entry_meta (
			user_id    TEXT    NOT NULL,
			entry_id   TEXT    NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
			pinned     INTEGER NOT NULL DEFAULT 0,
			sort_order INTEGER,
			tags       TEXT    NOT NULL DEFAULT '[]',
			PRIMARY KEY (user_id, entry_id)
		);
	`); err != nil {
		return err
	}

	// ── Additive columns on existing tables ───────────────────────────────────
	// SQLite supports ADD COLUMN but not IF NOT EXISTS, so we check first.

	if err := addColumnIfMissing(tx, "entries", "owner_user_id", "TEXT"); err != nil {
		return err
	}
	if err := addColumnIfMissing(tx, "entries", "project_ids", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}
	if err := addColumnIfMissing(tx, "annotations", "author_user_id", "TEXT"); err != nil {
		return err
	}

	if _, err := tx.Exec(`PRAGMA user_version = 1`); err != nil {
		return err
	}
	return tx.Commit()
}

func toV2(db *sql.DB) error {
	// FK constraints can't be toggled inside a transaction; disable outside.
	// legacy_alter_table=ON prevents SQLite from rewriting OTHER tables'
	// REFERENCES clauses when we rename `categories` below — without this,
	// category_default_tags.category_id silently ends up pointing at the
	// soon-to-be-dropped categories_v1, corrupting the schema.
	if _, err := db.Exec("PRAGMA foreign_keys=OFF"); err != nil {
		return err
	}
	if _, err := db.Exec("PRAGMA legacy_alter_table=ON"); err != nil {
		return err
	}
	defer db.Exec("PRAGMA legacy_alter_table=OFF") //nolint:errcheck
	defer db.Exec("PRAGMA foreign_keys=ON")         //nolint:errcheck

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().Unix()

	// Seed the system "global" project that owns admin-managed categories.
	if _, err := tx.Exec(`
		INSERT OR IGNORE INTO projects (id, name, type, created_by, created_at)
		VALUES ('global', 'Global Categories', 'global', 'system', ?)
	`, now); err != nil {
		return fmt.Errorf("seed global project: %w", err)
	}

	// Recreate categories with project_id column and (name, project_id) uniqueness.
	// The old schema had UNIQUE(name); SQLite requires table recreation to change constraints.
	// IMPORTANT: Split each DDL statement into separate Exec calls — Go's database/sql
	// with SQLite does not reliably execute multi-statement strings.
	if _, err := tx.Exec(`ALTER TABLE categories RENAME TO categories_v1`); err != nil {
		return fmt.Errorf("rename categories: %w", err)
	}

	if _, err := tx.Exec(`
		CREATE TABLE categories (
			id         TEXT    PRIMARY KEY,
			name       TEXT    NOT NULL,
			icon       TEXT    NOT NULL DEFAULT '',
			color      TEXT    NOT NULL DEFAULT '',
			sort_order INTEGER NOT NULL DEFAULT 0,
			project_id TEXT    NOT NULL DEFAULT 'global'
			               REFERENCES projects(id) ON DELETE CASCADE,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			UNIQUE(name, project_id)
		)
	`); err != nil {
		return fmt.Errorf("create categories: %w", err)
	}

	if _, err := tx.Exec(`
		INSERT INTO categories (id, name, icon, color, sort_order, project_id, created_at, updated_at)
		SELECT id, name, icon, color, sort_order, 'global', created_at, updated_at
		FROM categories_v1
	`); err != nil {
		return fmt.Errorf("copy categories: %w", err)
	}

	if _, err := tx.Exec(`DROP TABLE categories_v1`); err != nil {
		return fmt.Errorf("drop categories_v1: %w", err)
	}

	// Junction table: which global categories a project has opted into.
	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS project_global_categories (
			project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			category_id TEXT NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
			PRIMARY KEY (project_id, category_id)
		)
	`); err != nil {
		return fmt.Errorf("create project_global_categories: %w", err)
	}

	if err := addColumnIfMissing(tx, "project_members", "username", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("add username column: %w", err)
	}

	if _, err := tx.Exec(`PRAGMA user_version = 2`); err != nil {
		return err
	}
	return tx.Commit()
}

func toV3(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// body_key stores the MinIO object key for entries whose body lives in blob storage.
	// Null means the body is stored inline in the body column (legacy).
	if err := addColumnIfMissing(tx, "entries", "body_key", "TEXT"); err != nil {
		return fmt.Errorf("add body_key column: %w", err)
	}

	if _, err := tx.Exec(`PRAGMA user_version = 3`); err != nil {
		return err
	}
	return tx.Commit()
}

func toV4(db *sql.DB) error {
	if _, err := db.Exec("PRAGMA foreign_keys=OFF"); err != nil {
		return err
	}
	defer db.Exec("PRAGMA foreign_keys=ON") //nolint:errcheck

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Repairs installs that already ran the buggy toV2, where this table's
	// stored schema ended up referencing the dropped categories_v1 table.
	if _, err := tx.Exec(`ALTER TABLE category_default_tags RENAME TO category_default_tags_v3`); err != nil {
		return fmt.Errorf("rename category_default_tags: %w", err)
	}
	if _, err := tx.Exec(`
		CREATE TABLE category_default_tags (
			id          TEXT    PRIMARY KEY,
			category_id TEXT    NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
			label       TEXT    NOT NULL,
			sort_order  INTEGER NOT NULL DEFAULT 0
		)
	`); err != nil {
		return fmt.Errorf("recreate category_default_tags: %w", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO category_default_tags (id, category_id, label, sort_order)
		SELECT id, category_id, label, sort_order FROM category_default_tags_v3
	`); err != nil {
		return fmt.Errorf("copy category_default_tags: %w", err)
	}
	if _, err := tx.Exec(`DROP TABLE category_default_tags_v3`); err != nil {
		return fmt.Errorf("drop category_default_tags_v3: %w", err)
	}
	if _, err := tx.Exec(`PRAGMA user_version = 4`); err != nil {
		return err
	}
	return tx.Commit()
}

func toV5(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Per-user app settings (theme, etc.) as a generic JSON object,
	// keyed by the auth-service user ID (JWT sub) — no local users table.
	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS user_settings (
			user_id    TEXT    PRIMARY KEY,
			settings   TEXT    NOT NULL DEFAULT '{}',
			updated_at INTEGER NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("create user_settings: %w", err)
	}

	if _, err := tx.Exec(`PRAGMA user_version = 5`); err != nil {
		return err
	}
	return tx.Commit()
}

// toV6 adds Game Mode: a live session scoped to a project, and the chat
// message history attached to each session.
func toV6(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// A live session scoped to a project. Only one may be 'running' per project at a time.
	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS game_sessions (
			id          TEXT    PRIMARY KEY,
			project_id  TEXT    NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			status      TEXT    NOT NULL DEFAULT 'running', -- 'running' | 'stopped'
			started_by  TEXT    NOT NULL,
			started_at  INTEGER NOT NULL,
			stopped_by  TEXT,
			stopped_at  INTEGER
		)
	`); err != nil {
		return fmt.Errorf("create game_sessions: %w", err)
	}
	if _, err := tx.Exec(`
		CREATE INDEX IF NOT EXISTS idx_game_sessions_project ON game_sessions(project_id, status)
	`); err != nil {
		return fmt.Errorf("create idx_game_sessions_project: %w", err)
	}
	if _, err := tx.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_game_sessions_one_active
			ON game_sessions(project_id) WHERE status = 'running'
	`); err != nil {
		return fmt.Errorf("create idx_game_sessions_one_active: %w", err)
	}

	// Chat history for a game_session. 'note_card' rows snapshot the shared entry
	// at share-time so history stays meaningful even if the source entry changes later.
	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS chat_messages (
			id         TEXT    PRIMARY KEY,
			session_id TEXT    NOT NULL REFERENCES game_sessions(id) ON DELETE CASCADE,
			project_id TEXT    NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			sender_id  TEXT    NOT NULL,
			type       TEXT    NOT NULL DEFAULT 'text', -- 'text' | 'note_card' | 'system'
			body       TEXT    NOT NULL DEFAULT '',
			entry_id   TEXT    REFERENCES entries(id) ON DELETE SET NULL,
			card_title TEXT    NOT NULL DEFAULT '',
			card_body  TEXT    NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("create chat_messages: %w", err)
	}
	if _, err := tx.Exec(`
		CREATE INDEX IF NOT EXISTS idx_chat_messages_session ON chat_messages(session_id, created_at)
	`); err != nil {
		return fmt.Errorf("create idx_chat_messages_session: %w", err)
	}

	if _, err := tx.Exec(`PRAGMA user_version = 6`); err != nil {
		return err
	}
	return tx.Commit()
}

// toV7 indexes entries.owner_user_id, the column the entry read-authorization
// predicate and owner-only write checks filter on most heavily.
func toV7(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		CREATE INDEX IF NOT EXISTS idx_entries_owner ON entries(owner_user_id)
	`); err != nil {
		return fmt.Errorf("create idx_entries_owner: %w", err)
	}

	if _, err := tx.Exec(`PRAGMA user_version = 7`); err != nil {
		return err
	}
	return tx.Commit()
}

// addColumnIfMissing adds a column to a table only when it doesn't already exist.
func addColumnIfMissing(tx *sql.Tx, table, column, definition string) error {
	rows, err := tx.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt interface{}
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			return err
		}
		if strings.EqualFold(name, column) {
			return nil // already exists
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = tx.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition))
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
			`INSERT INTO categories (id, name, icon, color, sort_order, project_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 'global', ?, ?)`,
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
