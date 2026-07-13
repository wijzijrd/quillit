package db

import (
	"database/sql"
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

	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM categories`).Scan(&count); err != nil {
		t.Fatalf("count categories: %v", err)
	}
	if count != 6 {
		t.Errorf("expected 6 seeded categories, got %d", count)
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

	if err := migrate(database); err != nil {
		t.Fatalf("migrate from v1 failed: %v", err)
	}
	if err := checkForeignKeys(database); err != nil {
		t.Errorf("checkForeignKeys after upgrade: %v", err)
	}

	var version int
	if err := database.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 5 {
		t.Errorf("expected user_version=5 after full migration, got %d", version)
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

	if err := migrate(database); err != nil {
		t.Fatalf("migrate() did not repair the broken schema: %v", err)
	}
	if err := checkForeignKeys(database); err != nil {
		t.Errorf("checkForeignKeys after repair: %v", err)
	}

	if _, err := database.Exec(
		`INSERT INTO category_default_tags (id, category_id, label, sort_order) VALUES ('tag2','cat1','PC',1)`,
	); err != nil {
		t.Errorf("insert into repaired category_default_tags failed: %v", err)
	}
}
