package rpc_test

import (
	"database/sql"
	"testing"
	"time"

	"connectrpc.com/connect"
	_ "modernc.org/sqlite"

	v1 "github.com/quillit/gen/quillit/svc/v1"

	"github.com/quillit/svc/internal/rpc"
)

// setupMembershipDB mirrors app/svc/internal/handler/projects_test.go's
// setupProjectsDB — same minimal schema, duplicated here since rpc_test
// can't import handler's unexported test helpers.
func setupMembershipDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		t.Fatal(err)
	}
	schema := `
	CREATE TABLE projects (
		id TEXT PRIMARY KEY, name TEXT NOT NULL, type TEXT NOT NULL DEFAULT 'campaign',
		created_by TEXT NOT NULL, created_at INTEGER NOT NULL
	);
	CREATE TABLE project_members (
		id TEXT PRIMARY KEY, project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
		user_id TEXT NOT NULL, role TEXT NOT NULL, joined_at INTEGER NOT NULL,
		username TEXT NOT NULL DEFAULT '',
		UNIQUE(project_id, user_id)
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	return db
}

func seedProject(t *testing.T, db *sql.DB, id, createdBy string) {
	t.Helper()
	now := time.Now().Unix()
	if _, err := db.Exec(`INSERT INTO projects (id, name, type, created_by, created_at) VALUES (?, ?, 'campaign', ?, ?)`,
		id, "Test Project", createdBy, now); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO project_members (id, project_id, user_id, role, joined_at) VALUES (?, ?, ?, 'gm', ?)`,
		id+"-m-"+createdBy, id, createdBy, now); err != nil {
		t.Fatalf("seed member: %v", err)
	}
}

func TestCheckMembership_MemberReturnsIsMemberTrueWithRole(t *testing.T) {
	db := setupMembershipDB(t)
	seedProject(t, db, "proj-1", "user-1")
	srv := rpc.NewSvcInternalServer(db)

	resp, err := srv.CheckMembership(t.Context(), connect.NewRequest(&v1.CheckMembershipRequest{ProjectId: "proj-1", UserId: "user-1"}))
	if err != nil {
		t.Fatalf("CheckMembership: %v", err)
	}
	if !resp.Msg.GetIsMember() {
		t.Error("IsMember = false, want true")
	}
	if resp.Msg.GetRole() != "gm" || resp.Msg.GetProjectType() != "campaign" {
		t.Errorf("response = %+v, want role=gm/projectType=campaign", resp.Msg)
	}
}

func TestCheckMembership_NonMemberReturnsCodeNotFound(t *testing.T) {
	db := setupMembershipDB(t)
	seedProject(t, db, "proj-1", "user-1")
	srv := rpc.NewSvcInternalServer(db)

	_, err := srv.CheckMembership(t.Context(), connect.NewRequest(&v1.CheckMembershipRequest{ProjectId: "proj-1", UserId: "user-2"}))
	if err == nil {
		t.Fatal("expected an error for a non-member, got nil")
	}
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("code = %v, want CodeNotFound", connect.CodeOf(err))
	}
}

func TestCheckMembership_NonexistentProjectReturnsCodeNotFound(t *testing.T) {
	db := setupMembershipDB(t)
	srv := rpc.NewSvcInternalServer(db)

	_, err := srv.CheckMembership(t.Context(), connect.NewRequest(&v1.CheckMembershipRequest{ProjectId: "no-such-project", UserId: "user-1"}))
	if err == nil {
		t.Fatal("expected an error for a nonexistent project, got nil")
	}
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("code = %v, want CodeNotFound", connect.CodeOf(err))
	}
}
