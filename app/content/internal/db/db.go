package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

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
	return nil
}

// toV1 establishes a placeholder schema — just enough to prove the
// migration machinery works end-to-end. The real entries/entry_links/
// facets/project_facets tables move in via #37-#40.
func toV1(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS schema_meta (
			id         INTEGER PRIMARY KEY CHECK (id = 1),
			created_at INTEGER NOT NULL
		)
	`); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO schema_meta (id, created_at) VALUES (1, strftime('%s','now'))`); err != nil {
		return err
	}
	if _, err := tx.Exec(`PRAGMA user_version = 1`); err != nil {
		return err
	}
	return tx.Commit()
}

// toV2 adds the entry-domain tables this service owns per
// docs/web-refactor-spec.md §4/§7.2 (#37): entries, entry_links, and the
// facet vocabulary (facets + project_facets) entry writes validate
// against. Body is deliberately not a column here — it lives in MinIO at
// entries/{id}/body.md, with title/tags on this table as denormalized
// copies of the body's frontmatter, kept in sync on every write.
func toV2(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS entries (
			id             TEXT PRIMARY KEY,
			project_id     TEXT NOT NULL,
			slug           TEXT NOT NULL
				CHECK (slug <> '' AND slug NOT GLOB '*[^a-z0-9-]*'),
			directory_path TEXT NOT NULL DEFAULT '',
			title          TEXT NOT NULL DEFAULT '',
			tags           TEXT NOT NULL DEFAULT '[]',
			owner_user_id  TEXT,
			created_at     INTEGER NOT NULL,
			updated_at     INTEGER NOT NULL,
			UNIQUE(project_id, directory_path, slug)
		)
	`); err != nil {
		return fmt.Errorf("create entries: %w", err)
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_entries_project ON entries(project_id)`); err != nil {
		return fmt.Errorf("create entries project index: %w", err)
	}

	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS entry_links (
			entry_id        TEXT    NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
			target_path     TEXT    NOT NULL,
			target_entry_id TEXT,
			label           TEXT    NOT NULL DEFAULT '',
			card_facet      TEXT,
			resolved        INTEGER NOT NULL DEFAULT 0
		)
	`); err != nil {
		return fmt.Errorf("create entry_links: %w", err)
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_entry_links_entry ON entry_links(entry_id)`); err != nil {
		return fmt.Errorf("create entry_links entry index: %w", err)
	}

	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS facets (
			name TEXT PRIMARY KEY
				CHECK (name <> '' AND name NOT GLOB '*[^a-z0-9-]*')
		)
	`); err != nil {
		return fmt.Errorf("create facets: %w", err)
	}

	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS project_facets (
			project_id TEXT NOT NULL,
			name       TEXT NOT NULL
				CHECK (name <> '' AND name NOT GLOB '*[^a-z0-9-]*'),
			UNIQUE(project_id, name)
		)
	`); err != nil {
		return fmt.Errorf("create project_facets: %w", err)
	}

	if err := seedFacets(tx); err != nil {
		return fmt.Errorf("seed facets: %w", err)
	}

	if _, err := tx.Exec(`PRAGMA user_version = 2`); err != nil {
		return err
	}
	return tx.Commit()
}

// toV3 adds entries_fts, the FTS5 search index behind #43's search/wikilink-
// autocomplete endpoint (docs/web-refactor-spec.md §7.2: "A search index
// (FTS5 over title/tags/body) is added in content's DB alongside").
//
// entry_id is UNINDEXED (a lookup key, not searchable text) and deliberately
// the only per-row identifier stored here — project_id, slug, and
// directory_path live only on entries and are joined in at query time
// (internal/handler/search.go), so a rename that doesn't touch the body
// never leaves this index holding a stale path. title/tags/body, by
// contrast, only ever change in step with a body write, so refreshing this
// row exactly when entry_links is recompiled (handler.refreshSearchIndex,
// called alongside recompileLinks) keeps them consistent by construction —
// no triggers: this codebase's existing convention for keeping derived data
// in sync on write is an explicit call in the same transaction as the write
// (see recompileLinks in internal/handler/links.go); grep confirms neither
// content nor svc use SQL triggers anywhere, so search follows suit rather
// than introducing a new pattern for one feature.
func toV3(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS entries_fts USING fts5(
			entry_id UNINDEXED,
			title,
			tags,
			body,
			tokenize = 'unicode61'
		)
	`); err != nil {
		return fmt.Errorf("create entries_fts: %w", err)
	}

	if _, err := tx.Exec(`PRAGMA user_version = 3`); err != nil {
		return err
	}
	return tx.Commit()
}

// seedFacets populates the global facet vocabulary with the CLI defaults
// (docs/cli-spec.md), matching svc's pre-migration seed (svc/internal/db/db.go
// seedFacets) minus the quick_view_templates backfill — content starts fresh,
// with no legacy quick-view data to carry forward.
func seedFacets(tx *sql.Tx) error {
	for _, name := range []string{"motivation", "description", "history"} {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO facets (name) VALUES (?)`, name); err != nil {
			return err
		}
	}
	return nil
}
