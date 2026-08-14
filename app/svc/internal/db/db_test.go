package db

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func openMemDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	if _, err := database.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("PRAGMA foreign_keys=ON"); err != nil {
		t.Fatal(err)
	}
	return database
}

func TestOpen_FreshDatabase(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer database.Close()

	if err := checkForeignKeys(database); err != nil {
		t.Errorf("checkForeignKeys after fresh Open(): %v", err)
	}

	var version int
	if err := database.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 8 {
		t.Errorf("expected fresh Open() to migrate to user_version=8, got %d", version)
	}

	for _, table := range []string{"entries", "categories", "annotations", "campaigns", "quick_view_templates"} {
		if tableExists(t, database, table) {
			t.Errorf("expected table %q to not exist after a fresh Open() at v8", table)
		}
	}
}

// TestOpen_UpgradeFromV1 reproduces upgrading a real pre-existing v1 database
// (with data already in categories/category_default_tags) through every
// migration step. This is the regression test for the production bug: before
// the toV2 fix, this would fail with "no such table: main.categories_v1" the
// same way the real deployment did.
func TestOpen_UpgradeFromV1(t *testing.T) {
	database := openMemDB(t)
	defer database.Close()

	if err := toV1(database); err != nil {
		t.Fatalf("toV1: %v", err)
	}

	now := time.Now().Unix()
	if _, err := database.Exec(
		`INSERT INTO categories (id, name, icon, color, sort_order, created_at, updated_at) VALUES ('cat1','Characters','User','#000',0,?,?)`,
		now, now,
	); err != nil {
		t.Fatalf("seed v1 category: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO category_default_tags (id, category_id, label, sort_order) VALUES ('tag1','cat1','NPC',0)`,
	); err != nil {
		t.Fatalf("seed v1 category_default_tags: %v", err)
	}

	if err := migrate(database, latestSchemaVersion); err != nil {
		t.Fatalf("migrate from v1 failed: %v", err)
	}
	if err := checkForeignKeys(database); err != nil {
		t.Errorf("checkForeignKeys after upgrade: %v", err)
	}

	var version int
	if err := database.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 8 {
		t.Errorf("expected user_version=8 after full migration, got %d", version)
	}

	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM user_settings`).Scan(&count); err != nil {
		t.Errorf("user_settings table missing after migration: %v", err)
	}
}

// TestOpen_RepairsBrokenV2 reproduces the exact corrupted state a real
// deployment ended up in after running the (now-fixed) buggy toV2 — built by
// running the real toV1, then replaying the original buggy rename sequence
// by hand (without legacy_alter_table=ON, so SQLite rewrites
// category_default_tags' REFERENCES clause to the soon-to-be-dropped
// categories_v1, exactly as it did in production) — and confirms toV4 repairs it.
func TestOpen_RepairsBrokenV2(t *testing.T) {
	database := openMemDB(t)
	defer database.Close()

	if err := toV1(database); err != nil {
		t.Fatalf("toV1: %v", err)
	}

	now := time.Now().Unix()
	if _, err := database.Exec(
		`INSERT INTO categories (id, name, icon, color, sort_order, created_at, updated_at) VALUES ('cat1','Characters','User','#000',0,?,?)`,
		now, now,
	); err != nil {
		t.Fatalf("seed v1 category: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO category_default_tags (id, category_id, label, sort_order) VALUES ('tag1','cat1','NPC',0)`,
	); err != nil {
		t.Fatalf("seed v1 category_default_tags: %v", err)
	}

	// Replay the original buggy toV2 rename sequence by hand, deliberately
	// WITHOUT legacy_alter_table=ON, so category_default_tags' REFERENCES
	// clause gets silently rewritten to categories_v1, then dangles once
	// categories_v1 is dropped — reproducing the exact production corruption.
	if _, err := database.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`ALTER TABLE categories RENAME TO categories_v1`); err != nil {
		t.Fatalf("simulate buggy rename: %v", err)
	}
	if _, err := database.Exec(`
		CREATE TABLE categories (
			id TEXT PRIMARY KEY, name TEXT NOT NULL, icon TEXT NOT NULL DEFAULT '',
			color TEXT NOT NULL DEFAULT '', sort_order INTEGER NOT NULL DEFAULT 0,
			project_id TEXT NOT NULL DEFAULT 'global', created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
			UNIQUE(name, project_id)
		)
	`); err != nil {
		t.Fatalf("recreate categories: %v", err)
	}
	if _, err := database.Exec(`
		INSERT INTO categories (id, name, icon, color, sort_order, project_id, created_at, updated_at)
		SELECT id, name, icon, color, sort_order, 'global', created_at, updated_at FROM categories_v1
	`); err != nil {
		t.Fatalf("copy categories: %v", err)
	}
	if _, err := database.Exec(`DROP TABLE categories_v1`); err != nil {
		t.Fatalf("drop categories_v1: %v", err)
	}
	if _, err := database.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`PRAGMA user_version = 2`); err != nil {
		t.Fatal(err)
	}

	// Sanity-check the corruption actually reproduced before testing the fix.
	if err := checkForeignKeys(database); err == nil {
		t.Fatal("expected simulated corruption to fail foreign_key_check, but it passed")
	}

	if err := migrate(database, latestSchemaVersion); err != nil {
		t.Fatalf("migrate() did not repair the broken schema: %v", err)
	}
	if err := checkForeignKeys(database); err != nil {
		t.Errorf("checkForeignKeys after repair: %v", err)
	}
}

func hasColumn(t *testing.T, database *sql.DB, table, column string) bool {
	t.Helper()
	rows, err := database.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt interface{}
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		if strings.EqualFold(name, column) {
			return true
		}
	}
	return false
}

func tableExists(t *testing.T, database *sql.DB, table string) bool {
	t.Helper()
	var name string
	err := database.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name = ?`, table).Scan(&name)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		t.Fatal(err)
	}
	return true
}

func upToV6(t *testing.T, database *sql.DB) {
	t.Helper()
	for _, step := range []func(*sql.DB) error{toV1, toV2, toV3, toV4, toV5, toV6} {
		if err := step(database); err != nil {
			t.Fatalf("migrate up to v6: %v", err)
		}
	}
}

func upToV7(t *testing.T, database *sql.DB) {
	t.Helper()
	for _, step := range []func(*sql.DB) error{toV1, toV2, toV3, toV4, toV5, toV6, toV7} {
		if err := step(database); err != nil {
			t.Fatalf("migrate up to v7: %v", err)
		}
	}
}

// mustExec runs a write query and fails the test immediately if it errors.
func mustExec(t *testing.T, database *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := database.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func TestToV7_AddsEntryColumns(t *testing.T) {
	database := openMemDB(t)
	defer database.Close()
	upToV6(t, database)

	if err := toV7(database); err != nil {
		t.Fatalf("toV7: %v", err)
	}

	for _, col := range []string{"slug", "directory_path", "project_id"} {
		if !hasColumn(t, database, "entries", col) {
			t.Errorf("expected entries.%s to exist after toV7", col)
		}
	}
}

func TestToV7_CreatesFacetAndLinkTables(t *testing.T) {
	database := openMemDB(t)
	defer database.Close()
	upToV6(t, database)

	if err := toV7(database); err != nil {
		t.Fatalf("toV7: %v", err)
	}

	for _, table := range []string{"facets", "project_facets", "entry_links"} {
		if !tableExists(t, database, table) {
			t.Errorf("expected table %s to exist after toV7", table)
		}
	}
}

func TestToV7_SeedsDefaultFacets(t *testing.T) {
	database := openMemDB(t)
	defer database.Close()
	upToV6(t, database)

	if err := toV7(database); err != nil {
		t.Fatalf("toV7: %v", err)
	}

	for _, want := range []string{"motivation", "description", "history"} {
		var count int
		if err := database.QueryRow(`SELECT COUNT(*) FROM facets WHERE name = ?`, want).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Errorf("expected default facet %q to be seeded, got count=%d", want, count)
		}
	}
}

func TestToV7_SeedsFacetsFromQuickViewTemplates(t *testing.T) {
	database := openMemDB(t)
	defer database.Close()
	upToV6(t, database)

	if _, err := database.Exec(
		`INSERT INTO quick_view_templates (category, fields) VALUES ('Ancient Ruins', '[]')`,
	); err != nil {
		t.Fatalf("seed quick_view_templates: %v", err)
	}

	if err := toV7(database); err != nil {
		t.Fatalf("toV7: %v", err)
	}

	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM facets WHERE name = ?`, "ancient-ruins").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected quick_view_templates category to be seeded as kebab-case facet 'ancient-ruins', got count=%d", count)
	}
}

func TestToV7_RejectsNonKebabCaseFacetName(t *testing.T) {
	database := openMemDB(t)
	defer database.Close()
	upToV6(t, database)

	if err := toV7(database); err != nil {
		t.Fatalf("toV7: %v", err)
	}

	if _, err := database.Exec(`INSERT INTO facets (name) VALUES (?)`, "Not Kebab Case"); err == nil {
		t.Error("expected inserting a non-kebab-case facet name to fail the CHECK constraint, but it succeeded")
	}
}

func TestToV7_Idempotent(t *testing.T) {
	database := openMemDB(t)
	defer database.Close()
	upToV6(t, database)

	if err := toV7(database); err != nil {
		t.Fatalf("first toV7: %v", err)
	}
	if err := toV7(database); err != nil {
		t.Fatalf("second toV7 (idempotency): %v", err)
	}

	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM facets WHERE name = 'motivation'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected re-running toV7 not to duplicate seeded facets, got count=%d", count)
	}
}

func TestOpen_FreshDatabase_MigratesToV8(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer database.Close()

	var version int
	if err := database.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 8 {
		t.Errorf("expected fresh Open() to migrate to user_version=8, got %d", version)
	}
	if err := checkForeignKeys(database); err != nil {
		t.Errorf("checkForeignKeys after fresh Open(): %v", err)
	}
}

func TestToV8_DropsLegacyEntryDomainTables(t *testing.T) {
	database := openMemDB(t)
	defer database.Close()
	upToV7(t, database)

	now := time.Now().Unix()
	// Minimal cross-linked fixture so DROP has something real to remove and
	// chat_messages has a live entry_id to prove the FK rework doesn't lose data.
	mustExec(t, database, `INSERT INTO projects VALUES ('proj-1','Test','campaign','user1',?)`, now)
	mustExec(t, database, `INSERT INTO entries (id,title,category,body,created_at,updated_at) VALUES ('e1','Mary','Lore','body',?,?)`, now, now)
	mustExec(t, database, `INSERT INTO annotations (id,entry_id,text,created_at,updated_at) VALUES ('a1','e1','secret',?,?)`, now, now)
	mustExec(t, database, `INSERT INTO game_sessions (id,project_id,status,started_by,started_at) VALUES ('gs1','proj-1','running','user1',?)`, now)
	mustExec(t, database, `INSERT INTO chat_messages (id,session_id,project_id,sender_id,type,body,entry_id,card_title,card_body,created_at) VALUES ('m1','gs1','proj-1','user1','note_card','','e1','Mary','snapshot',?)`, now)

	if err := toV8(database); err != nil {
		t.Fatalf("toV8: %v", err)
	}

	dropped := []string{
		"entries", "annotations", "quick_view_templates", "categories",
		"category_default_tags", "project_global_categories", "entry_relations",
		"member_folders", "member_folder_entries", "member_entry_meta",
		"entry_shares", "campaigns", "players", "player_notes",
		"facets", "project_facets", "entry_links",
	}
	for _, table := range dropped {
		if tableExists(t, database, table) {
			t.Errorf("expected table %q to be dropped by toV8", table)
		}
	}

	if !tableExists(t, database, "chat_messages") {
		t.Fatal("chat_messages must survive toV8")
	}
	var cardTitle, entryID string
	if err := database.QueryRow(`SELECT card_title, entry_id FROM chat_messages WHERE id = 'm1'`).Scan(&cardTitle, &entryID); err != nil {
		t.Fatalf("chat_messages row lost during toV8: %v", err)
	}
	if cardTitle != "Mary" || entryID != "e1" {
		t.Errorf("chat_messages data corrupted: card_title=%q entry_id=%q", cardTitle, entryID)
	}

	if err := checkForeignKeys(database); err != nil {
		t.Errorf("checkForeignKeys after toV8: %v", err)
	}

	var version int
	if err := database.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 8 {
		t.Errorf("expected user_version=8, got %d", version)
	}
}

func TestToV8_Idempotent(t *testing.T) {
	database := openMemDB(t)
	defer database.Close()
	upToV7(t, database)

	if err := toV8(database); err != nil {
		t.Fatalf("first toV8: %v", err)
	}
	if err := toV8(database); err != nil {
		t.Fatalf("second toV8 (idempotency): %v", err)
	}
}

func TestOpenLegacy_MigratesToV7Only(t *testing.T) {
	database, err := OpenLegacy(":memory:")
	if err != nil {
		t.Fatalf("OpenLegacy() failed: %v", err)
	}
	defer database.Close()

	var version int
	if err := database.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 7 {
		t.Errorf("expected OpenLegacy() to stop at user_version=7, got %d", version)
	}

	for _, table := range []string{"entries", "annotations", "categories"} {
		if !tableExists(t, database, table) {
			t.Errorf("expected table %q to still exist after OpenLegacy() (must not reach v8)", table)
		}
	}

	if err := checkForeignKeys(database); err != nil {
		t.Errorf("checkForeignKeys after OpenLegacy(): %v", err)
	}
}

func TestOpen_StillReachesV8(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer database.Close()

	var version int
	if err := database.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 8 {
		t.Errorf("expected Open() to still reach user_version=8, got %d — this test guards against the OpenLegacy refactor accidentally capping Open() too", version)
	}
}
