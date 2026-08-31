package db

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

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

// legacyTables is every table the old toV1..toV7 chain created that toV8
// (issues #34/#35, the content-service cutover) then dropped — none of them
// should exist in the schema goose's baseline migration produces.
var legacyTables = []string{
	"entries", "annotations", "categories", "category_default_tags",
	"project_global_categories", "quick_view_templates", "players",
	"player_notes", "campaigns", "entry_shares", "entry_relations",
	"member_folders", "member_folder_entries", "member_entry_meta",
	"facets", "project_facets", "entry_links",
}

// survivingTables is every table that toV1..toV8's end state actually
// leaves behind — what the goose baseline migration reproduces.
var survivingTables = []string{
	"sessions", "projects", "project_members", "project_invites",
	"user_settings", "game_sessions", "chat_messages",
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

	for _, table := range legacyTables {
		if tableExists(t, database, table) {
			t.Errorf("expected legacy table %q to not exist after a fresh Open()", table)
		}
	}
	for _, table := range survivingTables {
		if !tableExists(t, database, table) {
			t.Errorf("expected table %q to exist after a fresh Open()", table)
		}
	}

	// The system "global" project (historically seeded by toV2 to own
	// admin-managed categories) is still expected by internal/handler/
	// projects.go's listing filter (type != 'global'), so the baseline
	// migration reproduces it.
	var projectType string
	if err := database.QueryRow(`SELECT type FROM projects WHERE id = 'global'`).Scan(&projectType); err != nil {
		t.Fatalf("expected seeded 'global' project to exist: %v", err)
	}
	if projectType != "global" {
		t.Errorf("expected 'global' project type = 'global', got %q", projectType)
	}
}

// TestOpen_IdempotentOnAlreadyMigratedDB confirms the baseline migration has
// zero schema effect when run a second time against a database that already
// ran it — the exact scenario the real production database will hit when
// this goose-based system first runs against it (it's already at the old
// system's v8 end state). Uses a real file (not :memory:) so the second
// Open() reopens genuinely persisted state rather than a fresh in-memory db.
func TestOpen_IdempotentOnAlreadyMigratedDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quillit.db")

	first, err := Open(path)
	if err != nil {
		t.Fatalf("first Open() failed: %v", err)
	}
	var firstCount int
	if err := first.QueryRow(`SELECT COUNT(*) FROM goose_db_version`).Scan(&firstCount); err != nil {
		t.Fatalf("query goose_db_version after first Open(): %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := Open(path)
	if err != nil {
		t.Fatalf("second Open() (against an already-migrated db) failed: %v", err)
	}
	defer second.Close()

	if err := checkForeignKeys(second); err != nil {
		t.Errorf("checkForeignKeys after second Open(): %v", err)
	}

	var secondCount int
	if err := second.QueryRow(`SELECT COUNT(*) FROM goose_db_version`).Scan(&secondCount); err != nil {
		t.Fatalf("query goose_db_version after second Open(): %v", err)
	}
	if secondCount != firstCount {
		t.Errorf("expected re-running the migration against an already-migrated db to record no new goose version rows, got %d rows before vs %d after", firstCount, secondCount)
	}

	for _, table := range legacyTables {
		if tableExists(t, second, table) {
			t.Errorf("expected legacy table %q to still not exist after second Open()", table)
		}
	}
	for _, table := range survivingTables {
		if !tableExists(t, second, table) {
			t.Errorf("expected table %q to still exist after second Open()", table)
		}
	}

	var projectCount int
	if err := second.QueryRow(`SELECT COUNT(*) FROM projects WHERE id = 'global'`).Scan(&projectCount); err != nil {
		t.Fatal(err)
	}
	if projectCount != 1 {
		t.Errorf("expected re-running the migration not to duplicate the seeded 'global' project, got count=%d", projectCount)
	}
}

// TestOpen_AgainstRealProductionShapedDB is the actual production scenario
// this migration must handle safely: a database that already reached the
// old hand-rolled system's v8 end state (built here with raw SQL matching
// that end state exactly, with no goose_db_version table at all — this is
// what the real home-server database looks like today, since it has never
// run goose). The baseline migration must apply against it with zero schema
// effect and without touching existing data.
func TestOpen_AgainstRealProductionShapedDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quillit.db")
	pre, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pre.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		t.Fatal(err)
	}

	now := time.Now().Unix()
	for _, stmt := range []string{
		`CREATE TABLE sessions (
			id TEXT PRIMARY KEY, jwt TEXT NOT NULL,
			expires_at INTEGER NOT NULL, created_at INTEGER NOT NULL
		)`,
		`CREATE TABLE projects (
			id TEXT PRIMARY KEY, name TEXT NOT NULL, type TEXT NOT NULL DEFAULT 'campaign',
			created_by TEXT NOT NULL, created_at INTEGER NOT NULL
		)`,
		`CREATE TABLE project_members (
			id TEXT PRIMARY KEY, project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			user_id TEXT NOT NULL, role TEXT NOT NULL, joined_at INTEGER NOT NULL,
			username TEXT NOT NULL DEFAULT '', UNIQUE(project_id, user_id)
		)`,
		`CREATE TABLE project_invites (
			id TEXT PRIMARY KEY, token TEXT NOT NULL UNIQUE,
			project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			role TEXT NOT NULL, created_by TEXT NOT NULL, expires_at INTEGER NOT NULL,
			used_at INTEGER, used_by TEXT
		)`,
		`CREATE TABLE user_settings (
			user_id TEXT PRIMARY KEY, settings TEXT NOT NULL DEFAULT '{}', updated_at INTEGER NOT NULL
		)`,
		`CREATE TABLE game_sessions (
			id TEXT PRIMARY KEY, project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			status TEXT NOT NULL DEFAULT 'running', started_by TEXT NOT NULL,
			started_at INTEGER NOT NULL, stopped_by TEXT, stopped_at INTEGER
		)`,
		`CREATE INDEX idx_game_sessions_project ON game_sessions(project_id, status)`,
		`CREATE UNIQUE INDEX idx_game_sessions_one_active ON game_sessions(project_id) WHERE status = 'running'`,
		`CREATE TABLE chat_messages (
			id TEXT PRIMARY KEY, session_id TEXT NOT NULL REFERENCES game_sessions(id) ON DELETE CASCADE,
			project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			sender_id TEXT NOT NULL, type TEXT NOT NULL DEFAULT 'text', body TEXT NOT NULL DEFAULT '',
			entry_id TEXT, card_title TEXT NOT NULL DEFAULT '', card_body TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL
		)`,
		`CREATE INDEX idx_chat_messages_session ON chat_messages(session_id, created_at)`,
	} {
		if _, err := pre.Exec(stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
	if _, err := pre.Exec(
		`INSERT INTO projects (id, name, type, created_by, created_at) VALUES ('global', 'Global Categories', 'global', 'system', ?)`,
		now,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pre.Exec(
		`INSERT INTO projects (id, name, type, created_by, created_at) VALUES ('proj-1', 'Real Campaign', 'campaign', 'user1', ?)`,
		now,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pre.Exec(`PRAGMA user_version = 8`); err != nil {
		t.Fatal(err)
	}
	if err := pre.Close(); err != nil {
		t.Fatal(err)
	}

	database, err := Open(path)
	if err != nil {
		t.Fatalf("Open() against a real-production-shaped pre-goose db failed: %v", err)
	}
	defer database.Close()

	if err := checkForeignKeys(database); err != nil {
		t.Errorf("checkForeignKeys: %v", err)
	}

	var projectCount int
	if err := database.QueryRow(`SELECT COUNT(*) FROM projects`).Scan(&projectCount); err != nil {
		t.Fatal(err)
	}
	if projectCount != 2 {
		t.Errorf("expected the migration to leave the 2 pre-existing projects untouched (no duplicate 'global' insert), got count=%d", projectCount)
	}

	var name string
	if err := database.QueryRow(`SELECT name FROM projects WHERE id = 'proj-1'`).Scan(&name); err != nil {
		t.Fatalf("pre-existing project data lost: %v", err)
	}
	if name != "Real Campaign" {
		t.Errorf("pre-existing project data corrupted: got name=%q", name)
	}

	for _, table := range legacyTables {
		if tableExists(t, database, table) {
			t.Errorf("expected the migration not to create legacy table %q against a real-production-shaped db", table)
		}
	}
}

// TestOpen_RejectsLegacyPreV8Database guards the precondition the baseline
// migration depends on but can't enforce with SQL alone: it's written
// entirely as IF NOT EXISTS / OR IGNORE statements, which only produce a
// correct schema against an empty database or one already at the old
// hand-rolled system's v8 end state. A database still at that old system's
// user_version 1-7 (never cut over to v8) must be rejected with a clear
// error instead of silently getting a wrong schema. The schema built here is
// intentionally minimal — the guard only inspects PRAGMA user_version, not
// the tables present — to isolate exactly what's under test.
func TestOpen_RejectsLegacyPreV8Database(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quillit.db")
	pre, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	// A minimal legacy-shaped table is enough: the guard fires on
	// PRAGMA user_version alone, not on inspecting this table's columns.
	if _, err := pre.Exec(`CREATE TABLE project_members (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, err := pre.Exec(`PRAGMA user_version = 7`); err != nil {
		t.Fatal(err)
	}
	if err := pre.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = Open(path)
	if err == nil {
		t.Fatal("expected Open() to reject a database at legacy user_version 7, got nil error")
	}
	if !strings.Contains(err.Error(), "legacy schema v7") {
		t.Errorf("expected error to mention the legacy schema version, got: %v", err)
	}
}

// TestOpen_ChatMessagesHasNoEntriesForeignKey guards the toV8 rework that's
// carried into the baseline: chat_messages.entry_id must not be a foreign
// key (the entries table it used to reference doesn't exist in this schema
// at all), while session_id/project_id must still reference their tables.
func TestOpen_ChatMessagesHasNoEntriesForeignKey(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer database.Close()

	rows, err := database.Query(`PRAGMA foreign_key_list(chat_messages)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	referenced := map[string]bool{}
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}
		var table string
		for i, c := range cols {
			if c == "table" {
				if s, ok := vals[i].(string); ok {
					table = s
				}
			}
		}
		if table != "" {
			referenced[table] = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	if referenced["entries"] {
		t.Error("expected chat_messages to have no foreign key referencing entries (that table doesn't exist post-cutover)")
	}
	if !referenced["game_sessions"] {
		t.Error("expected chat_messages.session_id to reference game_sessions")
	}
	if !referenced["projects"] {
		t.Error("expected chat_messages.project_id to reference projects")
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

func TestOpen_ProjectMembersHasUsernameColumn(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer database.Close()

	if !hasColumn(t, database, "project_members", "username") {
		t.Error("expected project_members.username to exist")
	}
}
