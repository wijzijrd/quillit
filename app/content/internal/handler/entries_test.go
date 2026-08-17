package handler

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/quillit/content-svc/internal/authz"
	"github.com/quillit/content-svc/internal/db"
)

// testUserID is the caller identity every shared test-request builder in
// this package injects by default (via withTestCaller). Handler tests use
// authz.AllowAll as their default Checker (see newTestEntriesHandler etc.),
// so any non-empty caller id would do — what matters for most tests is
// just "some authenticated caller," with the specific rejection paths
// (unauthenticated / non-member) exercised by their own dedicated tests
// using an explicit authz.Static checker instead.
const testUserID = "test-user"

// withTestCaller injects testUserID into req's context — the auth every
// content handler now requires (#44's WithTestCallerID, defined in
// helpers.go). Folded into withChiParam/withChiParams below since these
// tests call handler methods directly, bypassing any router or middleware
// that would otherwise supply it.
func withTestCaller(req *http.Request) *http.Request {
	return req.WithContext(WithTestCallerID(req.Context(), testUserID))
}

var errObjectNotFound = errors.New("object not found")

// fakeBlobStore is an in-memory BlobStore for tests — no real MinIO needed.
type fakeBlobStore struct {
	mu      sync.Mutex
	objects map[string][]byte
	// failAfter, when > 0, makes Put start failing once it has been called
	// more than failAfter times — lets tests force a mid-batch blob failure
	// (e.g. to exercise the import undo path) without a real backend.
	failAfter int
	putCalls  int
}

func newFakeBlobStore() *fakeBlobStore {
	return &fakeBlobStore{objects: map[string][]byte{}}
}

func (f *fakeBlobStore) Put(_ context.Context, key, _ string, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.putCalls++
	if f.failAfter > 0 && f.putCalls > f.failAfter {
		return errors.New("simulated blob store failure")
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	f.objects[key] = cp
	return nil
}

func (f *fakeBlobStore) Get(_ context.Context, key string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.objects[key]
	if !ok {
		return nil, errObjectNotFound
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

func newTestEntriesHandler(t *testing.T) (*EntriesHandler, *fakeBlobStore) {
	t.Helper()
	return newTestEntriesHandlerWithChecker(t, authz.AllowAll{})
}

func newTestEntriesHandlerWithChecker(t *testing.T, checker authz.Checker) (*EntriesHandler, *fakeBlobStore) {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	blobs := newFakeBlobStore()
	return NewEntries(database, "", blobs, checker), blobs
}

// withChiParam builds a request with a chi URL param set, mimicking what
// chi's router does before invoking a handler — needed since these tests
// call handler methods directly rather than through a mounted router. It
// also injects a default test caller identity (withTestCaller) — every
// handler now requires authentication (#44), and virtually every direct
// handler-method test in this package sets exactly one chi param
// immediately before invoking the handler, so this is the natural place
// for that default. Tests that need a specific caller identity or that
// exercise rejection (unauthenticated/non-member) build their own request
// context explicitly instead of using this helper.
func withChiParam(req *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return withTestCaller(req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx)))
}

func storedBodyHash(t *testing.T, h *EntriesHandler, id string) string {
	t.Helper()
	var hash sql.NullString
	if err := h.db.QueryRow(`SELECT body_hash FROM entries WHERE id = ?`, id).Scan(&hash); err != nil {
		t.Fatalf("query body_hash: %v", err)
	}
	return hash.String
}

func createEntry(t *testing.T, h *EntriesHandler, projectID string, reqBody createEntryRequest) Entry {
	t.Helper()
	raw, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/content/projects/"+projectID+"/entries", strings.NewReader(string(raw)))
	req = withChiParam(req, "id", projectID)
	w := httptest.NewRecorder()
	h.Create(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("Create() status = %d, body = %s", w.Code, w.Body.String())
	}
	var e Entry
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
		t.Fatalf("decode created entry: %v", err)
	}
	return e
}

func TestCreate_ValidEntry(t *testing.T) {
	h, blobs := newTestEntriesHandler(t)
	body := "---\nname: Tom the Innkeeper\ntags: [npc]\n---\n\n:::card motivation\nWants his farm back.\n:::\n"

	e := createEntry(t, h, "proj-1", createEntryRequest{Slug: "tom", DirectoryPath: "characters/npcs", Body: body})

	if e.Title != "Tom the Innkeeper" {
		t.Errorf("Title = %q, want %q", e.Title, "Tom the Innkeeper")
	}
	if len(e.Tags) != 1 || e.Tags[0] != "npc" {
		t.Errorf("Tags = %v, want [npc]", e.Tags)
	}
	if e.Body != body {
		t.Errorf("Body mismatch")
	}
	if _, ok := blobs.objects[bodyKey(e.ID)]; !ok {
		t.Error("expected body.md to be stored in blob storage")
	}
}

func TestCreate_UnknownFacetFailsLoud(t *testing.T) {
	h, _ := newTestEntriesHandler(t)
	body := ":::card role\nInnkeeper.\n:::\n"
	raw, _ := json.Marshal(createEntryRequest{Slug: "tom", Body: body})
	req := httptest.NewRequest(http.MethodPost, "/content/projects/proj-1/entries", strings.NewReader(string(raw)))
	req = withChiParam(req, "id", "proj-1")
	w := httptest.NewRecorder()

	h.Create(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusUnprocessableEntity, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if resp["facet"] != "role" {
		t.Errorf("facet = %v, want %q", resp["facet"], "role")
	}
	vocab, ok := resp["vocabulary"].([]any)
	if !ok || len(vocab) == 0 {
		t.Errorf("vocabulary = %v, want non-empty list naming the effective vocabulary", resp["vocabulary"])
	}
}

func TestList_ScopedByProject(t *testing.T) {
	h, _ := newTestEntriesHandler(t)
	createEntry(t, h, "proj-a", createEntryRequest{Slug: "mary", Body: "Mary."})
	createEntry(t, h, "proj-b", createEntryRequest{Slug: "john", Body: "John."})

	req := httptest.NewRequest(http.MethodGet, "/content/projects/proj-a/entries", nil)
	req = withChiParam(req, "id", "proj-a")
	w := httptest.NewRecorder()
	h.List(w, req)

	var entries []EntryMeta
	if err := json.Unmarshal(w.Body.Bytes(), &entries); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(entries) != 1 || entries[0].Slug != "mary" {
		t.Errorf("entries = %+v, want just proj-a's \"mary\"", entries)
	}
}

func TestGet_ResolvesBody(t *testing.T) {
	h, _ := newTestEntriesHandler(t)
	created := createEntry(t, h, "proj-1", createEntryRequest{Slug: "mary", Body: "Mary the merchant."})

	req := httptest.NewRequest(http.MethodGet, "/content/entries/"+created.ID, nil)
	req = withChiParam(req, "id", created.ID)
	w := httptest.NewRecorder()
	h.Get(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var e Entry
	_ = json.Unmarshal(w.Body.Bytes(), &e)
	if e.Body != "Mary the merchant." {
		t.Errorf("Body = %q, want %q", e.Body, "Mary the merchant.")
	}
}

func TestUpdate_RecompilesLinks(t *testing.T) {
	h, _ := newTestEntriesHandler(t)
	mary := createEntry(t, h, "proj-1", createEntryRequest{Slug: "mary", DirectoryPath: "characters/npcs", Body: "Mary."})
	tom := createEntry(t, h, "proj-1", createEntryRequest{Slug: "tom", DirectoryPath: "characters/npcs", Body: "Tom."})

	newBody := "Tom talks about [[characters/npcs/mary|Mary]] often."
	patch := updateEntryRequest{Body: &newBody}
	raw, _ := json.Marshal(patch)
	req := httptest.NewRequest(http.MethodPatch, "/content/entries/"+tom.ID, strings.NewReader(string(raw)))
	req = withChiParam(req, "id", tom.ID)
	w := httptest.NewRecorder()
	h.Update(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var targetID string
	var resolved bool
	err := h.db.QueryRow(`SELECT target_entry_id, resolved FROM entry_links WHERE entry_id = ?`, tom.ID).Scan(&targetID, &resolved)
	if err != nil {
		t.Fatalf("expected an entry_links row for tom after update: %v", err)
	}
	if targetID != mary.ID || !resolved {
		t.Errorf("entry_links row = (target=%s, resolved=%v), want (target=%s, resolved=true)", targetID, resolved, mary.ID)
	}
}

func TestCreate_StoresBodyHash(t *testing.T) {
	h, _ := newTestEntriesHandler(t)
	body := "---\nname: Tom\n---\n\nHello."
	e := createEntry(t, h, "proj-1", createEntryRequest{Slug: "tom", Body: body})

	sum := sha256.Sum256([]byte(body))
	want := hex.EncodeToString(sum[:])
	if got := storedBodyHash(t, h, e.ID); got != want {
		t.Errorf("body_hash = %q, want %q", got, want)
	}
}

func TestUpdate_RecomputesBodyHashOnBodyChange(t *testing.T) {
	h, _ := newTestEntriesHandler(t)
	e := createEntry(t, h, "proj-1", createEntryRequest{Slug: "tom", Body: "original"})
	originalHash := storedBodyHash(t, h, e.ID)

	newBody := "changed"
	req := httptest.NewRequest(http.MethodPatch, "/content/entries/"+e.ID, strings.NewReader(`{"body":"changed"}`))
	req = withChiParam(req, "id", e.ID)
	w := httptest.NewRecorder()
	h.Update(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Update status = %d, body = %s", w.Code, w.Body.String())
	}

	sum := sha256.Sum256([]byte(newBody))
	want := hex.EncodeToString(sum[:])
	got := storedBodyHash(t, h, e.ID)
	if got != want {
		t.Errorf("body_hash = %q, want %q", got, want)
	}
	if got == originalHash {
		t.Error("body_hash unchanged despite body change")
	}
}

func TestUpdate_PreservesBodyHashWhenBodyNotChanged(t *testing.T) {
	h, _ := newTestEntriesHandler(t)
	e := createEntry(t, h, "proj-1", createEntryRequest{Slug: "tom", Body: "original"})
	originalHash := storedBodyHash(t, h, e.ID)

	req := httptest.NewRequest(http.MethodPatch, "/content/entries/"+e.ID, strings.NewReader(`{"title":"New Title"}`))
	req = withChiParam(req, "id", e.ID)
	w := httptest.NewRecorder()
	h.Update(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Update status = %d, body = %s", w.Code, w.Body.String())
	}

	if got := storedBodyHash(t, h, e.ID); got != originalHash {
		t.Errorf("body_hash = %q, want unchanged %q (body wasn't touched)", got, originalHash)
	}
}

func TestList_IncludesBodyHash(t *testing.T) {
	h, _ := newTestEntriesHandler(t)
	body := "---\nname: Tom\n---\n\nHello."
	createEntry(t, h, "proj-1", createEntryRequest{Slug: "tom", Body: body})

	req := httptest.NewRequest(http.MethodGet, "/content/projects/proj-1/entries", nil)
	req = withChiParam(req, "id", "proj-1")
	w := httptest.NewRecorder()
	h.List(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("List status = %d, body = %s", w.Code, w.Body.String())
	}
	var got []EntryMeta
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1", len(got))
	}
	sum := sha256.Sum256([]byte(body))
	want := hex.EncodeToString(sum[:])
	if got[0].BodyHash != want {
		t.Errorf("BodyHash = %q, want %q", got[0].BodyHash, want)
	}
}

func TestUploadImage_StoresAtConventionalPath(t *testing.T) {
	h, blobs := newTestEntriesHandler(t)
	e := createEntry(t, h, "proj-1", createEntryRequest{Slug: "mary", Body: "Mary."})

	var buf strings.Builder
	mw := multipart.NewWriter(&buf)
	// multipart.Writer.CreateFormFile always sets Content-Type:
	// application/octet-stream; use CreatePart directly so the part carries
	// a real image content type, matching what a browser upload sends.
	partHeader := textproto.MIMEHeader{}
	partHeader.Set("Content-Disposition", `form-data; name="image"; filename="portrait.png"`)
	partHeader.Set("Content-Type", "image/png")
	part, err := mw.CreatePart(partHeader)
	if err != nil {
		t.Fatalf("CreatePart: %v", err)
	}
	if _, err := part.Write([]byte("fake-png-bytes")); err != nil {
		t.Fatalf("write image bytes: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/content/entries/"+e.ID+"/images", strings.NewReader(buf.String()))
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req = withChiParam(req, "id", e.ID)
	w := httptest.NewRecorder()
	h.UploadImage(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	wantPrefix := imagesPrefix(e.ID)
	if !strings.HasPrefix(resp["key"], wantPrefix) {
		t.Errorf("key = %q, want prefix %q", resp["key"], wantPrefix)
	}
	if !strings.HasSuffix(resp["key"], ".png") {
		t.Errorf("key = %q, want .png extension", resp["key"])
	}
	if _, ok := blobs.objects[resp["key"]]; !ok {
		t.Error("expected image bytes to be stored in blob storage")
	}
}

func TestGetImage_ReturnsStoredBytesAndContentType(t *testing.T) {
	h, blobs := newTestEntriesHandler(t)
	e := createEntry(t, h, "proj-1", createEntryRequest{Slug: "mary", Body: "Mary."})
	key := imagesPrefix(e.ID) + "portrait.png"
	blobs.objects[key] = []byte("fake-png-bytes")

	req := httptest.NewRequest(http.MethodGet, "/content/entries/"+e.ID+"/images/portrait.png", nil)
	req = withChiParams(req, map[string]string{"id": e.ID, "filename": "portrait.png"})
	w := httptest.NewRecorder()
	h.GetImage(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != "fake-png-bytes" {
		t.Errorf("body = %q, want %q", got, "fake-png-bytes")
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", ct)
	}
}

func TestGetImage_SetsAntiXSSHeaders(t *testing.T) {
	h, blobs := newTestEntriesHandler(t)
	e := createEntry(t, h, "proj-1", createEntryRequest{Slug: "mary", Body: "Mary."})
	key := imagesPrefix(e.ID) + "portrait.png"
	blobs.objects[key] = []byte("fake-png-bytes")

	req := httptest.NewRequest(http.MethodGet, "/content/entries/"+e.ID+"/images/portrait.png", nil)
	req = withChiParams(req, map[string]string{"id": e.ID, "filename": "portrait.png"})
	w := httptest.NewRecorder()
	h.GetImage(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want %q", got, "nosniff")
	}
	wantCSP := "default-src 'none'; sandbox"
	if got := w.Header().Get("Content-Security-Policy"); got != wantCSP {
		t.Errorf("Content-Security-Policy = %q, want %q", got, wantCSP)
	}
}

func TestGetImage_UnknownImageReturns404(t *testing.T) {
	h, _ := newTestEntriesHandler(t)
	e := createEntry(t, h, "proj-1", createEntryRequest{Slug: "mary", Body: "Mary."})

	req := httptest.NewRequest(http.MethodGet, "/content/entries/"+e.ID+"/images/missing.png", nil)
	req = withChiParams(req, map[string]string{"id": e.ID, "filename": "missing.png"})
	w := httptest.NewRecorder()
	h.GetImage(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestGetImage_RejectsPathTraversalFilename(t *testing.T) {
	h, blobs := newTestEntriesHandler(t)
	e := createEntry(t, h, "proj-1", createEntryRequest{Slug: "mary", Body: "Mary."})
	blobs.objects["entries/"+e.ID+"/body.md"] = []byte("secret body")

	req := httptest.NewRequest(http.MethodGet, "/content/entries/"+e.ID+"/images/..", nil)
	req = withChiParams(req, map[string]string{"id": e.ID, "filename": ".."})
	w := httptest.NewRecorder()
	h.GetImage(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestGetImage_RejectsNonProjectMember(t *testing.T) {
	h, blobs := newTestEntriesHandler(t)
	e := createEntry(t, h, "proj-1", createEntryRequest{Slug: "mary", Body: "Mary."})
	blobs.objects[imagesPrefix(e.ID)+"portrait.png"] = []byte("fake-png-bytes")

	denied := NewEntries(h.db, "", blobs, authz.Static{})
	req := httptest.NewRequest(http.MethodGet, "/content/entries/"+e.ID+"/images/portrait.png", nil)
	req = withChiParams(req, map[string]string{"id": e.ID, "filename": "portrait.png"})
	w := httptest.NewRecorder()
	denied.GetImage(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestDelete_RemovesRowAndBlobs(t *testing.T) {
	h, blobs := newTestEntriesHandler(t)
	e := createEntry(t, h, "proj-1", createEntryRequest{Slug: "mary", Body: "Mary."})

	req := httptest.NewRequest(http.MethodDelete, "/content/entries/"+e.ID, nil)
	req = withChiParam(req, "id", e.ID)
	w := httptest.NewRecorder()
	h.Delete(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if _, ok := blobs.objects[bodyKey(e.ID)]; ok {
		t.Error("expected body.md to be removed from blob storage")
	}
	var count int
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM entries WHERE id = ?`, e.ID).Scan(&count)
	if count != 0 {
		t.Errorf("expected entries row to be deleted, count = %d", count)
	}
}

// ── Auth (#44) ───────────────────────────────────────────────────────────
//
// These prove entries.go's endpoints reject unauthenticated and non-member
// requests consistently, not just that member requests succeed (already
// exercised implicitly by every test above, all of which go through
// withChiParam/withTestCaller against an AllowAll checker).

func TestList_RejectsUnauthenticatedRequest(t *testing.T) {
	h, _ := newTestEntriesHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/content/projects/proj-1/entries", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "proj-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx)) // no caller injected
	w := httptest.NewRecorder()
	h.List(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestList_RejectsNonProjectMember(t *testing.T) {
	h, _ := newTestEntriesHandlerWithChecker(t, authz.Static{Members: map[string]map[string]bool{
		testUserID: {"proj-a": true},
	}})
	req := httptest.NewRequest(http.MethodGet, "/content/projects/proj-b/entries", nil)
	req = withChiParam(req, "id", "proj-b") // testUserID is only a member of proj-a
	w := httptest.NewRecorder()
	h.List(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestCreate_RejectsUnauthenticatedRequest(t *testing.T) {
	h, _ := newTestEntriesHandler(t)
	raw, _ := json.Marshal(createEntryRequest{Slug: "tom", Body: "Tom."})
	req := httptest.NewRequest(http.MethodPost, "/content/projects/proj-1/entries", strings.NewReader(string(raw)))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "proj-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.Create(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestGet_RejectsNonProjectMember(t *testing.T) {
	h, _ := newTestEntriesHandler(t)
	e := createEntry(t, h, "proj-1", createEntryRequest{Slug: "mary", Body: "Mary."})

	denied := NewEntries(h.db, "", h.blobs, authz.Static{}) // empty Static: no one is a member of anything
	req := httptest.NewRequest(http.MethodGet, "/content/entries/"+e.ID, nil)
	req = withChiParam(req, "id", e.ID)
	w := httptest.NewRecorder()
	denied.Get(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestUpdate_RejectsNonProjectMember(t *testing.T) {
	h, _ := newTestEntriesHandler(t)
	e := createEntry(t, h, "proj-1", createEntryRequest{Slug: "mary", Body: "Mary."})

	denied := NewEntries(h.db, "", h.blobs, authz.Static{})
	newBody := "Mary, updated."
	raw, _ := json.Marshal(updateEntryRequest{Body: &newBody})
	req := httptest.NewRequest(http.MethodPatch, "/content/entries/"+e.ID, strings.NewReader(string(raw)))
	req = withChiParam(req, "id", e.ID)
	w := httptest.NewRecorder()
	denied.Update(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestDelete_RejectsNonProjectMember(t *testing.T) {
	h, blobs := newTestEntriesHandler(t)
	e := createEntry(t, h, "proj-1", createEntryRequest{Slug: "mary", Body: "Mary."})

	denied := NewEntries(h.db, "", blobs, authz.Static{})
	req := httptest.NewRequest(http.MethodDelete, "/content/entries/"+e.ID, nil)
	req = withChiParam(req, "id", e.ID)
	w := httptest.NewRecorder()
	denied.Delete(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusForbidden, w.Body.String())
	}

	var count int
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM entries WHERE id = ?`, e.ID).Scan(&count)
	if count != 1 {
		t.Error("entry was deleted despite the caller not being a project member")
	}
}

func TestUploadImage_RejectsNonProjectMember(t *testing.T) {
	h, blobs := newTestEntriesHandler(t)
	e := createEntry(t, h, "proj-1", createEntryRequest{Slug: "mary", Body: "Mary."})

	denied := NewEntries(h.db, "", blobs, authz.Static{})
	req := httptest.NewRequest(http.MethodPost, "/content/entries/"+e.ID+"/images", strings.NewReader(""))
	req = withChiParam(req, "id", e.ID)
	w := httptest.NewRecorder()
	denied.UploadImage(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusForbidden, w.Body.String())
	}
}
