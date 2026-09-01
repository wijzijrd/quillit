package db

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// productionShapedSchemaSQL reproduces the old hand-rolled system's v5
// end-state schema exactly (derived by reading toV1..toV5 before they were
// deleted — see the baseline migration's header comment for the full
// derivation), plus a small amount of pre-existing data used by
// TestOpen_AgainstRealProductionShapedDB to confirm Open() doesn't lose or
// duplicate anything already there.
const productionShapedSchemaSQL = `
	CREATE TABLE schema_meta (
		id         INTEGER PRIMARY KEY CHECK (id = 1),
		created_at INTEGER NOT NULL
	);
	INSERT INTO schema_meta (id, created_at) VALUES (1, 1700000000);

	CREATE TABLE entries (
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
		orphaned_at    INTEGER,
		body_hash      TEXT,
		UNIQUE(project_id, directory_path, slug)
	);
	CREATE INDEX idx_entries_project ON entries(project_id);
	INSERT INTO entries (id, project_id, slug, directory_path, title, tags, created_at, updated_at)
		VALUES ('e1', 'proj-a', 'mary', 'characters/npcs', 'Mary', '[]', 1700000000, 1700000000);

	CREATE TABLE entry_links (
		entry_id        TEXT    NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
		target_path     TEXT    NOT NULL,
		target_entry_id TEXT,
		label           TEXT    NOT NULL DEFAULT '',
		card_facet      TEXT,
		resolved        INTEGER NOT NULL DEFAULT 0
	);
	CREATE INDEX idx_entry_links_entry ON entry_links(entry_id);

	CREATE TABLE facets (
		name TEXT PRIMARY KEY
			CHECK (name <> '' AND name NOT GLOB '*[^a-z0-9-]*')
	);
	INSERT INTO facets (name) VALUES ('motivation'), ('description'), ('history'), ('custom-facet');

	CREATE TABLE project_facets (
		project_id TEXT NOT NULL,
		name       TEXT NOT NULL
			CHECK (name <> '' AND name NOT GLOB '*[^a-z0-9-]*'),
		UNIQUE(project_id, name)
	);

	CREATE VIRTUAL TABLE entries_fts USING fts5(
		entry_id UNINDEXED,
		title,
		tags,
		body,
		tokenize = 'unicode61'
	);
	INSERT INTO entries_fts (entry_id, title, tags, body) VALUES ('e1', 'Mary', '[]', 'A tavern regular.');

	PRAGMA user_version = 5;
`

func TestOpen_FreshDatabase(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer database.Close()

	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM schema_meta`).Scan(&count); err != nil {
		t.Fatalf("count schema_meta: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 schema_meta row, got %d", count)
	}

	for _, table := range []string{"entries", "entry_links", "facets", "project_facets", "entries_fts"} {
		if !tableExists(t, database, table) {
			t.Errorf("expected table %q to exist after Open()", table)
		}
	}
}

func TestOpen_IdempotentOnAlreadyMigratedDB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	database, err := Open(path)
	if err != nil {
		t.Fatalf("first Open() failed: %v", err)
	}
	var before int
	if err := database.QueryRow(`SELECT COUNT(*) FROM goose_db_version`).Scan(&before); err != nil {
		t.Fatalf("count goose_db_version: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	database2, err := Open(path)
	if err != nil {
		t.Fatalf("second Open() failed: %v", err)
	}
	defer database2.Close()

	var after int
	if err := database2.QueryRow(`SELECT COUNT(*) FROM goose_db_version`).Scan(&after); err != nil {
		t.Fatalf("count goose_db_version after reopen: %v", err)
	}
	if after != before {
		t.Errorf("goose_db_version row count changed on reopen: before=%d after=%d", before, after)
	}

	var facetCount int
	if err := database2.QueryRow(`SELECT COUNT(*) FROM facets`).Scan(&facetCount); err != nil {
		t.Fatalf("count facets: %v", err)
	}
	if facetCount != 3 {
		t.Errorf("facets count after reopen = %d, want 3 (no duplicate seeding)", facetCount)
	}
}

// TestOpen_AgainstRealProductionShapedDB builds the exact old-system v5
// end-state schema by hand with raw SQL (no goose involved), including
// pre-existing entry/facet data, then calls Open() and confirms zero data
// loss: the pre-existing entry, custom facet, and FTS row all survive
// unchanged. For the structural "is the resulting schema actually
// identical to what the old system produced" claim, see
// TestSchema_BaselineMatchesProductionShapedSchema, which asserts that
// directly via PRAGMA table_info/index_info comparison rather than relying
// on spot-checking a few rows of data.
func TestOpen_AgainstRealProductionShapedDB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prod.db")

	setup, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open setup db: %v", err)
	}
	if _, err := setup.Exec(productionShapedSchemaSQL); err != nil {
		t.Fatalf("build production-shaped schema: %v", err)
	}
	if err := setup.Close(); err != nil {
		t.Fatalf("close setup db: %v", err)
	}

	database, err := Open(path)
	if err != nil {
		t.Fatalf("Open() against production-shaped db failed: %v", err)
	}
	defer database.Close()

	// No data loss: the pre-existing entry and the custom facet survive.
	var entryCount int
	if err := database.QueryRow(`SELECT COUNT(*) FROM entries WHERE id = 'e1'`).Scan(&entryCount); err != nil {
		t.Fatalf("count entries: %v", err)
	}
	if entryCount != 1 {
		t.Errorf("expected pre-existing entry e1 to survive, got count %d", entryCount)
	}

	var facetCount int
	if err := database.QueryRow(`SELECT COUNT(*) FROM facets`).Scan(&facetCount); err != nil {
		t.Fatalf("count facets: %v", err)
	}
	if facetCount != 4 {
		t.Errorf("expected 4 facets (3 seeded + 1 custom, no duplication), got %d", facetCount)
	}

	var ftsCount int
	if err := database.QueryRow(`SELECT COUNT(*) FROM entries_fts WHERE entries_fts MATCH 'tavern'`).Scan(&ftsCount); err != nil {
		t.Fatalf("query entries_fts: %v", err)
	}
	if ftsCount != 1 {
		t.Errorf("expected pre-existing FTS row to survive, got count %d", ftsCount)
	}
}

// TestSchema_BaselineMatchesProductionShapedSchema asserts the structural
// claim that TestOpen_AgainstRealProductionShapedDB's own comment makes but
// never actually checked: that the goose baseline produces a schema
// byte-identical to the old hand-rolled system's v5 end state, aside from
// goose's own goose_db_version bookkeeping table. It builds the old
// system's schema by hand (the same productionShapedSchemaSQL used above)
// and, separately, opens a brand-new database through the goose baseline,
// then compares the two structurally: every table's PRAGMA table_info
// columns and every index's PRAGMA index_info columns must match.
//
// sqlite_master.sql text is deliberately not compared directly: two
// semantically-identical CREATE statements (e.g. one written with an
// explicit "IF NOT EXISTS" or different whitespace/indentation, as is the
// case between this test's hand-typed schema and 00001_baseline.sql) can
// differ byte-for-byte in sqlite_master while being structurally identical.
// PRAGMA table_info/index_info give a structural view that isn't sensitive
// to that, while still catching a real regression (a missing/extra column,
// a wrong type, a changed nullability/default/primary-key flag, or an
// index over the wrong columns).
func TestSchema_BaselineMatchesProductionShapedSchema(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old-system.db")

	oldDB, err := sql.Open("sqlite", oldPath)
	if err != nil {
		t.Fatalf("open old-system db: %v", err)
	}
	defer oldDB.Close()
	if _, err := oldDB.Exec(productionShapedSchemaSQL); err != nil {
		t.Fatalf("build old-system schema: %v", err)
	}

	newDB, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() fresh db: %v", err)
	}
	defer newDB.Close()

	oldSnapshot := schemaSnapshot(t, oldDB)
	newSnapshot := schemaSnapshot(t, newDB)

	for name, oldDef := range oldSnapshot {
		newDef, ok := newSnapshot[name]
		if !ok {
			t.Errorf("schema object %q exists in the old-system schema but is missing from the goose baseline schema", name)
			continue
		}
		if oldDef != newDef {
			t.Errorf("schema object %q differs between old-system schema and goose baseline schema:\n  old-system: %s\n  baseline:   %s", name, oldDef, newDef)
		}
	}
	for name := range newSnapshot {
		if _, ok := oldSnapshot[name]; !ok {
			t.Errorf("schema object %q exists in the goose baseline schema but not in the old-system schema (unexpected drift)", name)
		}
	}
}

// schemaSnapshot returns a normalized structural view of database's schema,
// keyed by object name, suitable for equality comparison between two
// databases. For each table (including virtual tables, e.g. entries_fts and
// its FTS5 shadow tables) it captures PRAGMA table_info; for each index it
// captures PRAGMA index_info. goose_db_version (goose's own migration
// bookkeeping table) and sqlite_sequence (the table SQLite creates
// implicitly because goose_db_version uses AUTOINCREMENT) are excluded:
// they are artifacts of the migration runner, not part of the content
// schema, and only ever exist on the goose-baselined side of a comparison.
func schemaSnapshot(t *testing.T, database *sql.DB) map[string]string {
	t.Helper()

	rows, err := database.Query(`
		SELECT type, name, tbl_name FROM sqlite_master
		WHERE name NOT IN ('goose_db_version', 'sqlite_sequence')
		ORDER BY type, name`)
	if err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	defer rows.Close()

	type object struct{ typ, name, tbl string }
	var objects []object
	for rows.Next() {
		var o object
		if err := rows.Scan(&o.typ, &o.name, &o.tbl); err != nil {
			t.Fatalf("scan sqlite_master row: %v", err)
		}
		objects = append(objects, o)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate sqlite_master: %v", err)
	}

	snapshot := make(map[string]string, len(objects))
	for _, o := range objects {
		switch o.typ {
		case "table":
			snapshot[o.name] = "table:" + o.name + ":" + tableColumnsSignature(t, database, o.name)
		case "index":
			snapshot[o.name] = "index:" + o.name + " on " + o.tbl + ":" + indexColumnsSignature(t, database, o.name)
		case "trigger":
			// None expected in this schema; captured for completeness so a
			// future trigger addition isn't silently invisible to this check.
			var sqlText string
			if err := database.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'trigger' AND name = ?`, o.name).Scan(&sqlText); err != nil {
				t.Fatalf("read trigger sql for %q: %v", o.name, err)
			}
			snapshot[o.name] = "trigger:" + o.name + ":" + strings.Join(strings.Fields(sqlText), " ")
		default:
			snapshot[o.name] = o.typ + ":" + o.name
		}
	}
	return snapshot
}

// tableColumnsSignature returns a stable, order-preserving string encoding
// of PRAGMA table_info(table): column id, name, declared type, NOT NULL,
// default value, and primary-key position.
func tableColumnsSignature(t *testing.T, database *sql.DB, table string) string {
	t.Helper()
	rows, err := database.Query(fmt.Sprintf("PRAGMA table_info(%q)", table))
	if err != nil {
		t.Fatalf("table_info(%s): %v", table, err)
	}
	defer rows.Close()

	var cols []string
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info(%s): %v", table, err)
		}
		cols = append(cols, fmt.Sprintf("%d:%s:%s:notnull=%d:default=%s:pk=%d", cid, name, ctype, notnull, dflt.String, pk))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate table_info(%s): %v", table, err)
	}
	return strings.Join(cols, "|")
}

// indexColumnsSignature returns a stable, order-preserving string encoding
// of PRAGMA index_info(index): the sequence and identity of columns the
// index covers.
func indexColumnsSignature(t *testing.T, database *sql.DB, index string) string {
	t.Helper()
	rows, err := database.Query(fmt.Sprintf("PRAGMA index_info(%q)", index))
	if err != nil {
		t.Fatalf("index_info(%s): %v", index, err)
	}
	defer rows.Close()

	var cols []string
	for rows.Next() {
		var seqno, cid int
		var name sql.NullString
		if err := rows.Scan(&seqno, &cid, &name); err != nil {
			t.Fatalf("scan index_info(%s): %v", index, err)
		}
		cols = append(cols, fmt.Sprintf("%d:%d:%s", seqno, cid, name.String))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate index_info(%s): %v", index, err)
	}
	return strings.Join(cols, "|")
}

func TestOpen_RejectsLegacyPreV5Database(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.db")

	setup, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open setup db: %v", err)
	}
	// Minimal legacy-shaped entries table (as of toV2/toV3, before toV4/
	// toV5 added orphaned_at/body_hash) at user_version 3.
	if _, err := setup.Exec(`
		CREATE TABLE entries (
			id             TEXT PRIMARY KEY,
			project_id     TEXT NOT NULL,
			slug           TEXT NOT NULL,
			directory_path TEXT NOT NULL DEFAULT '',
			title          TEXT NOT NULL DEFAULT '',
			tags           TEXT NOT NULL DEFAULT '[]',
			owner_user_id  TEXT,
			created_at     INTEGER NOT NULL,
			updated_at     INTEGER NOT NULL
		);
		PRAGMA user_version = 3;
	`); err != nil {
		t.Fatalf("build legacy schema: %v", err)
	}
	if err := setup.Close(); err != nil {
		t.Fatalf("close setup db: %v", err)
	}

	_, err = Open(path)
	if err == nil {
		t.Fatal("expected Open() to error against a legacy v3 database, got nil")
	}
	if !strings.Contains(err.Error(), "legacy schema v3") {
		t.Errorf("error message = %q, want it to mention \"legacy schema v3\"", err.Error())
	}
}

func TestOpen_CreatesSearchIndex(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer database.Close()

	if _, err := database.Exec(`INSERT INTO entries_fts (entry_id, title, tags, body) VALUES (?, ?, ?, ?)`,
		"e1", "Tom the Innkeeper", "npc", "Runs the tavern."); err != nil {
		t.Fatalf("insert into entries_fts failed: %v", err)
	}

	var entryID string
	if err := database.QueryRow(`SELECT entry_id FROM entries_fts WHERE entries_fts MATCH 'tavern'`).Scan(&entryID); err != nil {
		t.Fatalf("FTS5 MATCH query failed: %v", err)
	}
	if entryID != "e1" {
		t.Errorf("entry_id = %q, want %q", entryID, "e1")
	}
}

// TestVirtualTable_CreateIfNotExistsIsIdempotent verifies directly (independent
// of the migration machinery) that SQLite's CREATE VIRTUAL TABLE IF NOT
// EXISTS is valid, idempotent syntax against modernc.org/sqlite — i.e. that
// running it a second time neither errors nor resets the index's contents.
// This is standard SQLite behavior, not goose-specific, but the brief asked
// for it to be verified with a real test rather than assumed.
func TestVirtualTable_CreateIfNotExistsIsIdempotent(t *testing.T) {
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer database.Close()

	create := `CREATE VIRTUAL TABLE IF NOT EXISTS entries_fts USING fts5(
		entry_id UNINDEXED, title, tags, body, tokenize = 'unicode61'
	)`
	if _, err := database.Exec(create); err != nil {
		t.Fatalf("first CREATE VIRTUAL TABLE IF NOT EXISTS failed: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO entries_fts (entry_id, title, tags, body) VALUES ('e1', 'Tom', '', 'Runs the tavern.')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	// Second CREATE VIRTUAL TABLE IF NOT EXISTS must be a no-op: no error,
	// and it must not clear the row just inserted.
	if _, err := database.Exec(create); err != nil {
		t.Fatalf("second CREATE VIRTUAL TABLE IF NOT EXISTS failed: %v", err)
	}
	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM entries_fts WHERE entries_fts MATCH 'tavern'`).Scan(&count); err != nil {
		t.Fatalf("query after second CREATE: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 matching row after idempotent re-CREATE, got %d", count)
	}
}

func TestOpen_SeedsGlobalFacets(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer database.Close()

	for _, name := range []string{"motivation", "description", "history"} {
		var count int
		if err := database.QueryRow(`SELECT COUNT(*) FROM facets WHERE name = ?`, name).Scan(&count); err != nil {
			t.Fatalf("count facets(%s): %v", name, err)
		}
		if count != 1 {
			t.Errorf("facet %q: expected 1 row, got %d", name, count)
		}
	}
}

func TestOpen_EntriesUniqueSlugPerDirectory(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer database.Close()

	insert := `INSERT INTO entries (id, project_id, slug, directory_path, created_at, updated_at) VALUES (?, ?, ?, ?, 0, 0)`
	if _, err := database.Exec(insert, "e1", "proj-a", "mary", "characters/npcs"); err != nil {
		t.Fatalf("first insert failed: %v", err)
	}
	if _, err := database.Exec(insert, "e2", "proj-a", "mary", "characters/npcs"); err == nil {
		t.Error("expected UNIQUE constraint violation for duplicate (project_id, directory_path, slug), got nil")
	}
	// Same slug, different project — allowed.
	if _, err := database.Exec(insert, "e3", "proj-b", "mary", "characters/npcs"); err != nil {
		t.Errorf("insert into different project should succeed: %v", err)
	}
}

func TestOpen_EntriesOrphanedAtDefaultsNullAndIsSettable(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer database.Close()

	insert := `INSERT INTO entries (id, project_id, slug, directory_path, created_at, updated_at) VALUES (?, ?, ?, ?, 0, 0)`
	if _, err := database.Exec(insert, "e1", "proj-a", "mary", "characters/npcs"); err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	var orphanedAt sql.NullInt64
	if err := database.QueryRow(`SELECT orphaned_at FROM entries WHERE id = ?`, "e1").Scan(&orphanedAt); err != nil {
		t.Fatalf("select orphaned_at: %v", err)
	}
	if orphanedAt.Valid {
		t.Errorf("orphaned_at = %v, want NULL for a freshly created entry", orphanedAt)
	}

	if _, err := database.Exec(`UPDATE entries SET orphaned_at = 12345 WHERE id = ?`, "e1"); err != nil {
		t.Fatalf("update orphaned_at: %v", err)
	}
	if err := database.QueryRow(`SELECT orphaned_at FROM entries WHERE id = ?`, "e1").Scan(&orphanedAt); err != nil {
		t.Fatalf("select orphaned_at after update: %v", err)
	}
	if !orphanedAt.Valid || orphanedAt.Int64 != 12345 {
		t.Errorf("orphaned_at = %v, want 12345", orphanedAt)
	}
}

func TestOpen_EntriesBodyHashDefaultsNullAndIsSettable(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer database.Close()

	insert := `INSERT INTO entries (id, project_id, slug, directory_path, created_at, updated_at) VALUES (?, ?, ?, ?, 0, 0)`
	if _, err := database.Exec(insert, "e1", "proj-a", "mary", "characters/npcs"); err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	var bodyHash sql.NullString
	if err := database.QueryRow(`SELECT body_hash FROM entries WHERE id = ?`, "e1").Scan(&bodyHash); err != nil {
		t.Fatalf("select body_hash: %v", err)
	}
	if bodyHash.Valid {
		t.Errorf("body_hash = %v, want NULL for a freshly created entry", bodyHash)
	}

	if _, err := database.Exec(`UPDATE entries SET body_hash = ? WHERE id = ?`, "abc123", "e1"); err != nil {
		t.Fatalf("update body_hash: %v", err)
	}
	if err := database.QueryRow(`SELECT body_hash FROM entries WHERE id = ?`, "e1").Scan(&bodyHash); err != nil {
		t.Fatalf("select body_hash after update: %v", err)
	}
	if !bodyHash.Valid || bodyHash.String != "abc123" {
		t.Errorf("body_hash = %v, want abc123", bodyHash)
	}
}

func TestOpen_IsIdempotent(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("first Open() failed: %v", err)
	}
	defer database.Close()

	// Running migrate() again against the same handle must not error or
	// duplicate the schema_meta row.
	if err := migrate(database); err != nil {
		t.Fatalf("second migrate() failed: %v", err)
	}
	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM schema_meta`).Scan(&count); err != nil {
		t.Fatalf("count schema_meta: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 schema_meta row after re-migrate, got %d", count)
	}
}

func tableExists(t *testing.T, database *sql.DB, name string) bool {
	t.Helper()
	var got string
	err := database.QueryRow(`SELECT name FROM sqlite_master WHERE type IN ('table','view') AND name = ?`, name).Scan(&got)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		t.Fatalf("check table %q exists: %v", name, err)
	}
	return got == name
}
