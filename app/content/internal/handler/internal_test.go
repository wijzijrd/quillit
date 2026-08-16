package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/quillit/content-svc/internal/db"
)

func newTestInternalHandler(t *testing.T) (*InternalHandler, *EntriesHandler) {
	t.Helper()
	entries, _ := newTestEntriesHandler(t)
	return NewInternal(entries.db), entries
}

func notifyProjectDeleted(t *testing.T, h *InternalHandler, projectID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/content/internal/projects/"+projectID+"/deleted", nil)
	req = withChiParam(req, "id", projectID)
	w := httptest.NewRecorder()
	h.ProjectDeleted(w, req)
	return w
}

// TestProjectDeleted_OrphansRatherThanDeletes is the #44 acceptance test:
// create a project + entries via content, "delete the project" (simulate
// svc's notification), and verify the entries end up orphaned — not
// destroyed. This is the content-side half of the full create→delete flow;
// see app/svc/internal/handler/projects_test.go for the svc-side half
// (verifying Delete actually places this call) — content and svc are
// separate Go modules with no cross-module test imports, so the full flow
// is proven by these two halves together rather than one combined test.
func TestProjectDeleted_OrphansRatherThanDeletes(t *testing.T) {
	internal, entries := newTestInternalHandler(t)
	e1 := createEntry(t, entries, "proj-1", createEntryRequest{Slug: "mary", Body: "Mary the merchant."})
	e2 := createEntry(t, entries, "proj-1", createEntryRequest{Slug: "tom", Body: "Tom the innkeeper."})
	// A second, unrelated project must not be touched.
	other := createEntry(t, entries, "proj-2", createEntryRequest{Slug: "other", Body: "Untouched."})

	w := notifyProjectDeleted(t, internal, "proj-1")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp projectDeletedResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.EntriesOrphaned != 2 {
		t.Errorf("EntriesOrphaned = %d, want 2", resp.EntriesOrphaned)
	}

	for _, id := range []string{e1.ID, e2.ID} {
		var v sql.NullInt64
		if err := entries.db.QueryRow(`SELECT orphaned_at FROM entries WHERE id = ?`, id).Scan(&v); err != nil {
			t.Fatalf("select orphaned_at for %s: %v", id, err)
		}
		if !v.Valid {
			t.Errorf("entry %s: orphaned_at is NULL, want set", id)
		}
	}

	// The entry row itself, its body blob, and its metadata must all
	// survive — orphan, don't destroy.
	var count int
	if err := entries.db.QueryRow(`SELECT COUNT(*) FROM entries WHERE project_id = ?`, "proj-1").Scan(&count); err != nil {
		t.Fatalf("count entries: %v", err)
	}
	if count != 2 {
		t.Errorf("entries remaining for proj-1 = %d, want 2 (orphaned, not deleted)", count)
	}

	// The unrelated project's entry is untouched.
	got, err := entries.fetchResolved(t.Context(), other.ID)
	if err != nil {
		t.Fatalf("fetch proj-2 entry: %v", err)
	}
	if got.OrphanedAt != nil {
		t.Errorf("proj-2's entry was orphaned by proj-1's deletion notification, want untouched")
	}

	// A fresh Get (with the checker back to the default allow-all
	// membership) still returns the orphaned entry's data — it's flagged,
	// not gone.
	e1Resolved, err := entries.fetchResolved(t.Context(), e1.ID)
	if err != nil {
		t.Fatalf("fetch orphaned entry: %v", err)
	}
	if e1Resolved.Body != "Mary the merchant." {
		t.Errorf("orphaned entry body = %q, want preserved", e1Resolved.Body)
	}
	if e1Resolved.OrphanedAt == nil {
		t.Error("expected EntryMeta.OrphanedAt to be set for an orphaned entry")
	}
}

func TestProjectDeleted_IdempotentSecondCallOrphansNothingNew(t *testing.T) {
	internal, entries := newTestInternalHandler(t)
	createEntry(t, entries, "proj-1", createEntryRequest{Slug: "mary", Body: "Mary."})

	w1 := notifyProjectDeleted(t, internal, "proj-1")
	var resp1 projectDeletedResponse
	_ = json.Unmarshal(w1.Body.Bytes(), &resp1)
	if resp1.EntriesOrphaned != 1 {
		t.Fatalf("first call EntriesOrphaned = %d, want 1", resp1.EntriesOrphaned)
	}

	w2 := notifyProjectDeleted(t, internal, "proj-1")
	if w2.Code != http.StatusOK {
		t.Fatalf("second call status = %d, body = %s", w2.Code, w2.Body.String())
	}
	var resp2 projectDeletedResponse
	_ = json.Unmarshal(w2.Body.Bytes(), &resp2)
	if resp2.EntriesOrphaned != 0 {
		t.Errorf("second call EntriesOrphaned = %d, want 0 (already orphaned)", resp2.EntriesOrphaned)
	}
}

func TestProjectDeleted_UnknownProjectOrphansNothing(t *testing.T) {
	internal, _ := newTestInternalHandler(t)
	w := notifyProjectDeleted(t, internal, "no-such-project")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp projectDeletedResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.EntriesOrphaned != 0 {
		t.Errorf("EntriesOrphaned = %d, want 0", resp.EntriesOrphaned)
	}
}

// TestProjectDeleted_DoesNotRequireProjectMembership confirms the internal
// webhook route is reachable without going through requireCaller/
// requireProjectMember the way every other content endpoint now must
// (#44) — it's svc reporting a system event, not proxying a user request,
// so there's no caller identity to check. This test builds its own
// InternalHandler directly against a fresh db (no EntriesHandler/Checker
// involved at all) specifically to prove ProjectDeleted's own db-level
// orphaning doesn't depend on one.
func TestProjectDeleted_DoesNotRequireProjectMembership(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if _, err := database.Exec(
		`INSERT INTO entries (id, project_id, slug, directory_path, created_at, updated_at) VALUES (?, ?, ?, '', 0, 0)`,
		newID(), "proj-1", "sample",
	); err != nil {
		t.Fatalf("insert entry: %v", err)
	}

	internal := NewInternal(database)
	req := withChiParam(httptest.NewRequest(http.MethodPost, "/content/internal/projects/proj-1/deleted", nil), "id", "proj-1")
	rec := httptest.NewRecorder()
	internal.ProjectDeleted(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp projectDeletedResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.EntriesOrphaned != 1 {
		t.Errorf("EntriesOrphaned = %d, want 1", resp.EntriesOrphaned)
	}
}
