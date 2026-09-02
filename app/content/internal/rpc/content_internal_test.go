package rpc_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/go-chi/chi/v5"

	v1 "github.com/quillit/gen/quillit/content/v1"

	"github.com/quillit/content-svc/internal/authz"
	"github.com/quillit/content-svc/internal/db"
	"github.com/quillit/content-svc/internal/handler"
	"github.com/quillit/content-svc/internal/rpc"
)

// fakeBlobStore is a minimal in-memory handler.BlobStore, just enough for
// EntriesHandler.Create (used to seed fixtures below) and
// ContentInternalServer.GetEntry to round-trip an entry's body.
type fakeBlobStore struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func newFakeBlobStore() *fakeBlobStore { return &fakeBlobStore{objects: map[string][]byte{}} }

func (f *fakeBlobStore) Put(_ context.Context, key, _ string, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[key] = append([]byte(nil), data...)
	return nil
}

func (f *fakeBlobStore) Get(_ context.Context, key string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.objects[key]
	if !ok {
		return nil, errors.New("object not found")
	}
	return data, nil
}

func (f *fakeBlobStore) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objects, key)
	return nil
}

func (f *fakeBlobStore) DeletePrefix(_ context.Context, prefix string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for k := range f.objects {
		if strings.HasPrefix(k, prefix) {
			delete(f.objects, k)
		}
	}
	return nil
}

// newFixture builds a ContentInternalServer and an EntriesHandler sharing
// one in-memory db, so tests can seed entries the same way the HTTP layer
// would (via EntriesHandler.Create) and then exercise the RPC surface
// against that same data — proving GetEntry/NotifyProjectDeleted are backed
// by the real sqlc logic, not a reimplementation.
func newFixture(t *testing.T) (*rpc.ContentInternalServer, *handler.EntriesHandler) {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	blobs := newFakeBlobStore()
	entries := handler.NewEntries(database, "", blobs, authz.AllowAll{})
	return rpc.NewContentInternalServer(database, blobs), entries
}

func withChiParam(req *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return req.WithContext(
		handler.WithTestCallerID(context.WithValue(req.Context(), chi.RouteCtxKey, rctx), "test-user"),
	)
}

type createEntryRequest struct {
	Slug          string `json:"slug"`
	DirectoryPath string `json:"directoryPath"`
	Body          string `json:"body"`
}

func createEntry(t *testing.T, h *handler.EntriesHandler, projectID string, reqBody createEntryRequest) handler.Entry {
	t.Helper()
	raw, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/content/projects/"+projectID+"/entries", strings.NewReader(string(raw)))
	req = withChiParam(req, "id", projectID)
	w := httptest.NewRecorder()
	h.Create(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("Create() status = %d, body = %s", w.Code, w.Body.String())
	}
	var e handler.Entry
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
		t.Fatalf("decode created entry: %v", err)
	}
	return e
}

func TestGetEntry_Found(t *testing.T) {
	srv, entries := newFixture(t)
	e := createEntry(t, entries, "proj-1", createEntryRequest{Slug: "mary", Body: "Mary the merchant."})

	resp, err := srv.GetEntry(t.Context(), connect.NewRequest(&v1.GetEntryRequest{EntryId: e.ID}))
	if err != nil {
		t.Fatalf("GetEntry: %v", err)
	}
	if resp.Msg.GetId() != e.ID || resp.Msg.GetProjectId() != "proj-1" || resp.Msg.GetTitle() != "mary" || resp.Msg.GetBody() != "Mary the merchant." {
		t.Errorf("unexpected response: %+v", resp.Msg)
	}
}

func TestGetEntry_MissingReturnsCodeNotFound(t *testing.T) {
	srv, _ := newFixture(t)

	_, err := srv.GetEntry(t.Context(), connect.NewRequest(&v1.GetEntryRequest{EntryId: "no-such-entry"}))
	if err == nil {
		t.Fatal("expected an error for a missing entry, got nil")
	}
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("code = %v, want CodeNotFound", connect.CodeOf(err))
	}
}

func TestNotifyProjectDeleted_OrphansRatherThanDeletes(t *testing.T) {
	srv, entries := newFixture(t)
	e1 := createEntry(t, entries, "proj-1", createEntryRequest{Slug: "mary", Body: "Mary the merchant."})
	e2 := createEntry(t, entries, "proj-1", createEntryRequest{Slug: "tom", Body: "Tom the innkeeper."})
	other := createEntry(t, entries, "proj-2", createEntryRequest{Slug: "other", Body: "Untouched."})

	resp, err := srv.NotifyProjectDeleted(t.Context(), connect.NewRequest(&v1.NotifyProjectDeletedRequest{ProjectId: "proj-1"}))
	if err != nil {
		t.Fatalf("NotifyProjectDeleted: %v", err)
	}
	if resp.Msg.GetEntriesOrphaned() != 2 {
		t.Errorf("EntriesOrphaned = %d, want 2", resp.Msg.GetEntriesOrphaned())
	}
	if resp.Msg.GetProjectId() != "proj-1" {
		t.Errorf("ProjectId = %q, want proj-1", resp.Msg.GetProjectId())
	}

	// GetEntryResponse doesn't carry orphaned_at, so this just confirms the
	// entries themselves still resolve (orphan, not delete) after the call.
	for _, id := range []string{e1.ID, e2.ID} {
		if _, err := srv.GetEntry(t.Context(), connect.NewRequest(&v1.GetEntryRequest{EntryId: id})); err != nil {
			t.Fatalf("GetEntry(%s): %v", id, err)
		}
	}

	otherResp, err := srv.GetEntry(t.Context(), connect.NewRequest(&v1.GetEntryRequest{EntryId: other.ID}))
	if err != nil {
		t.Fatalf("GetEntry(other): %v", err)
	}
	if otherResp.Msg.GetBody() != "Untouched." {
		t.Errorf("unrelated project's entry body changed: %q", otherResp.Msg.GetBody())
	}
}

func TestNotifyProjectDeleted_IdempotentSecondCallOrphansNothingNew(t *testing.T) {
	srv, entries := newFixture(t)
	createEntry(t, entries, "proj-1", createEntryRequest{Slug: "mary", Body: "Mary."})

	resp1, err := srv.NotifyProjectDeleted(t.Context(), connect.NewRequest(&v1.NotifyProjectDeletedRequest{ProjectId: "proj-1"}))
	if err != nil {
		t.Fatalf("first NotifyProjectDeleted: %v", err)
	}
	if resp1.Msg.GetEntriesOrphaned() != 1 {
		t.Fatalf("first call EntriesOrphaned = %d, want 1", resp1.Msg.GetEntriesOrphaned())
	}

	resp2, err := srv.NotifyProjectDeleted(t.Context(), connect.NewRequest(&v1.NotifyProjectDeletedRequest{ProjectId: "proj-1"}))
	if err != nil {
		t.Fatalf("second NotifyProjectDeleted: %v", err)
	}
	if resp2.Msg.GetEntriesOrphaned() != 0 {
		t.Errorf("second call EntriesOrphaned = %d, want 0 (already orphaned)", resp2.Msg.GetEntriesOrphaned())
	}
}

func TestNotifyProjectDeleted_UnknownProjectOrphansNothing(t *testing.T) {
	srv, _ := newFixture(t)

	resp, err := srv.NotifyProjectDeleted(t.Context(), connect.NewRequest(&v1.NotifyProjectDeletedRequest{ProjectId: "no-such-project"}))
	if err != nil {
		t.Fatalf("NotifyProjectDeleted: %v", err)
	}
	if resp.Msg.GetEntriesOrphaned() != 0 {
		t.Errorf("EntriesOrphaned = %d, want 0", resp.Msg.GetEntriesOrphaned())
	}
}

func TestNotifyProjectDeleted_DoesNotRequireProjectMembership(t *testing.T) {
	// Mirrors the old HTTP test's intent: this RPC is svc reporting a
	// system event, not proxying a user request, so it must work with no
	// caller identity/membership check involved at all — a fresh db with
	// no EntriesHandler/checker in the picture proves that.
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if _, err := database.Exec(
		`INSERT INTO entries (id, project_id, slug, directory_path, created_at, updated_at) VALUES (?, ?, ?, '', 0, 0)`,
		"e1", "proj-1", "sample",
	); err != nil {
		t.Fatalf("insert entry: %v", err)
	}

	srv := rpc.NewContentInternalServer(database, nil)
	resp, err := srv.NotifyProjectDeleted(context.Background(), connect.NewRequest(&v1.NotifyProjectDeletedRequest{ProjectId: "proj-1"}))
	if err != nil {
		t.Fatalf("NotifyProjectDeleted: %v", err)
	}
	if resp.Msg.GetEntriesOrphaned() != 1 {
		t.Errorf("EntriesOrphaned = %d, want 1", resp.Msg.GetEntriesOrphaned())
	}
}
