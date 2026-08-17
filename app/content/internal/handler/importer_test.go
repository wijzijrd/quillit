package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/quillit/content-svc/internal/authz"
	"github.com/quillit/content-svc/internal/db"
)

func newTestImportHandler(t *testing.T) (*ImportHandler, *fakeBlobStore, *EntriesHandler) {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	blobs := newFakeBlobStore()
	return NewImport(database, "", blobs, authz.AllowAll{}), blobs, NewEntries(database, "", blobs, authz.AllowAll{})
}

// doImport POSTs tarball files to the import endpoint via a chi router so
// {id} resolves, and decodes the response.
func doImport(t *testing.T, h *ImportHandler, projectID, query string, files map[string]string) (*httptest.ResponseRecorder, ImportResponse) {
	t.Helper()
	r := chi.NewRouter()
	r.Post("/content/projects/{id}/import", h.ImportProject)
	req := withTestCaller(httptest.NewRequest(http.MethodPost,
		"/content/projects/"+projectID+"/import"+query, makeTarball(t, files)))
	req.Header.Set("Content-Type", "application/gzip")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	var resp ImportResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	return rec, resp
}

const tomMD = "---\nname: Tom\ntags: [npc]\n---\n\nTom runs the inn at [[locations/inn]].\n"
const innMD = "---\nname: The Inn\n---\n\nRun by [[characters/npcs/tom|Tom]].\n"

func projectFiles() map[string]string {
	return map[string]string{
		"characters/npcs/tom/tom.md": tomMD,
		"locations/inn/inn.md":       innMD,
	}
}

func TestImport_DryRunByDefault_MakesNoChanges(t *testing.T) {
	h, blobs, _ := newTestImportHandler(t)
	rec, resp := doImport(t, h, "p1", "", projectFiles())
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if resp.Applied {
		t.Error("dry-run reported applied=true")
	}
	if len(resp.Report) != 2 || resp.Report[0].Action != "create" || resp.Report[1].Action != "create" {
		t.Errorf("report = %+v, want 2 create rows", resp.Report)
	}
	var n int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM entries`).Scan(&n); err != nil || n != 0 {
		t.Errorf("entries rows after dry-run = %d (err %v), want 0", n, err)
	}
	if len(blobs.objects) != 0 {
		t.Errorf("blobs after dry-run = %d, want 0", len(blobs.objects))
	}
}

func TestImport_Apply_CreatesEntriesAndResolvedLinks(t *testing.T) {
	h, blobs, _ := newTestImportHandler(t)
	rec, resp := doImport(t, h, "p1", "?mode=apply", projectFiles())
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if !resp.Applied {
		t.Error("apply reported applied=false")
	}

	// Both entries exist with the right metadata.
	var tomID, innID string
	if err := h.db.QueryRow(`SELECT id FROM entries WHERE project_id='p1' AND directory_path='characters/npcs' AND slug='tom'`).Scan(&tomID); err != nil {
		t.Fatalf("tom row: %v", err)
	}
	if err := h.db.QueryRow(`SELECT id FROM entries WHERE project_id='p1' AND directory_path='locations' AND slug='inn'`).Scan(&innID); err != nil {
		t.Fatalf("inn row: %v", err)
	}

	// AC 4: in-batch wikilinks resolve to real ids, both directions.
	var target string
	var resolved bool
	if err := h.db.QueryRow(`SELECT target_entry_id, resolved FROM entry_links WHERE entry_id=?`, tomID).Scan(&target, &resolved); err != nil {
		t.Fatalf("tom link: %v", err)
	}
	if !resolved || target != innID {
		t.Errorf("tom→inn link = (%s, %v), want (%s, true)", target, resolved, innID)
	}
	if err := h.db.QueryRow(`SELECT target_entry_id, resolved FROM entry_links WHERE entry_id=?`, innID).Scan(&target, &resolved); err != nil {
		t.Fatalf("inn link: %v", err)
	}
	if !resolved || target != tomID {
		t.Errorf("inn→tom link = (%s, %v), want (%s, true)", target, resolved, tomID)
	}

	// Bodies stored byte-identical (render parity rests on this + shared parser).
	body, err := blobs.Get(context.Background(), bodyKey(tomID))
	if err != nil || string(body) != tomMD {
		t.Errorf("tom body = %q (err %v), want source bytes", body, err)
	}
}

func TestImport_ApplyMatchesDryRunReport(t *testing.T) {
	h, _, _ := newTestImportHandler(t)
	_, dry := doImport(t, h, "p1", "", projectFiles())
	_, apply := doImport(t, h, "p1", "?mode=apply", projectFiles())
	if len(dry.Report) != len(apply.Report) {
		t.Fatalf("report lengths differ: dry %d apply %d", len(dry.Report), len(apply.Report))
	}
	for i := range dry.Report {
		if dry.Report[i] != apply.Report[i] {
			t.Errorf("row %d: dry %+v != apply %+v", i, dry.Report[i], apply.Report[i])
		}
	}
}

func TestImport_ParseFailure_Is422ListingAllEntries(t *testing.T) {
	h, _, _ := newTestImportHandler(t)
	files := map[string]string{
		"a/a.md": "---\nname: A\n---\n\n:::secret\nunclosed",
		"b/b.md": "---\nname: B\n---\n\n:::secret\nalso unclosed",
	}
	r := chi.NewRouter()
	r.Post("/content/projects/{id}/import", h.ImportProject)
	req := withTestCaller(httptest.NewRequest(http.MethodPost, "/content/projects/p1/import", makeTarball(t, files)))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Entries []struct{ Path, Error string } `json:"entries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Entries) != 2 {
		t.Errorf("failing entries = %d, want 2 (every failure listed)", len(body.Entries))
	}
	var n int
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM entries`).Scan(&n)
	if n != 0 {
		t.Errorf("entries created on validation failure = %d, want 0", n)
	}
}

func TestImport_UnknownFacet_RejectedByDefault_CreatedWithFlag(t *testing.T) {
	h, _, _ := newTestImportHandler(t)
	files := map[string]string{
		"tom/tom.md": "---\nname: Tom\n---\n\n:::card stat-block\nAC 12\n:::\n",
	}

	r := chi.NewRouter()
	r.Post("/content/projects/{id}/import", h.ImportProject)
	req := withTestCaller(httptest.NewRequest(http.MethodPost, "/content/projects/p1/import?mode=apply", makeTarball(t, files)))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body %s", rec.Code, rec.Body.String())
	}
	var errBody struct {
		MissingFacets []string `json:"missingFacets"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &errBody)
	if len(errBody.MissingFacets) != 1 || errBody.MissingFacets[0] != "stat-block" {
		t.Errorf("missingFacets = %v, want [stat-block]", errBody.MissingFacets)
	}

	// With createFacets=true the same import succeeds and records the facet.
	rec2, resp := doImport(t, h, "p1", "?mode=apply&createFacets=true", files)
	if rec2.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec2.Code, rec2.Body.String())
	}
	if len(resp.Facets.Created) != 1 || resp.Facets.Created[0] != "stat-block" {
		t.Errorf("facets.created = %v, want [stat-block]", resp.Facets.Created)
	}
	var n int
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM project_facets WHERE project_id='p1' AND name='stat-block'`).Scan(&n)
	if n != 1 {
		t.Error("stat-block not added to project_facets")
	}
}

// TestImport_NonKebabFacetRejectedEvenWithCreateFacets guards against a bug
// where the card-block parser accepts any non-space token as a facet name
// (e.g. "Bad_Facet"), but project_facets has a kebab-case CHECK constraint;
// with createFacets=true and INSERT OR IGNORE, the constraint violation was
// silently swallowed and facets.created reported the bad name as created
// even though it never actually landed in project_facets. The bad name must
// be rejected in the 422, regardless of createFacets.
func TestImport_NonKebabFacetRejectedEvenWithCreateFacets(t *testing.T) {
	h, _, _ := newTestImportHandler(t)
	files := map[string]string{
		"tom/tom.md": "---\nname: Tom\n---\n\n:::card Bad_Facet\nAC 12\n:::\n",
	}

	r := chi.NewRouter()
	r.Post("/content/projects/{id}/import", h.ImportProject)
	req := withTestCaller(httptest.NewRequest(http.MethodPost, "/content/projects/p1/import?mode=apply&createFacets=true", makeTarball(t, files)))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Bad_Facet") {
		t.Errorf("422 body doesn't name Bad_Facet: %s", rec.Body.String())
	}
	var n int
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM project_facets WHERE project_id='p1' AND name='Bad_Facet'`).Scan(&n)
	if n != 0 {
		t.Error("invalid facet name was inserted into project_facets")
	}
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM entries`).Scan(&n)
	if n != 0 {
		t.Error("entry created despite invalid facet name")
	}

	// A genuine kebab-case facet in the same createFacets=true flow still
	// works (regression guard against the fix over-rejecting).
	files2 := map[string]string{
		"tom/tom.md": "---\nname: Tom\n---\n\n:::card stat-block\nAC 12\n:::\n",
	}
	rec2, resp2 := doImport(t, h, "p1", "?mode=apply&createFacets=true", files2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec2.Code, rec2.Body.String())
	}
	if len(resp2.Facets.Created) != 1 || resp2.Facets.Created[0] != "stat-block" {
		t.Errorf("facets.created = %v, want [stat-block]", resp2.Facets.Created)
	}
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM project_facets WHERE project_id='p1' AND name='stat-block'`).Scan(&n)
	if n != 1 {
		t.Error("stat-block not added to project_facets")
	}
}

func TestImport_Conflicts(t *testing.T) {
	seed := func(t *testing.T) (*ImportHandler, string) {
		h, _, entries := newTestImportHandler(t)
		// Existing entry at characters/npcs/tom via the normal Create endpoint.
		req := httptest.NewRequest(http.MethodPost, "/content/projects/p1/entries",
			strings.NewReader(`{"slug":"tom","directoryPath":"characters/npcs","body":"---\nname: Old Tom\n---\n\nOld."}`))
		req = withChiParam(req, "id", "p1")
		rec := httptest.NewRecorder()
		entries.Create(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("seed create: %d %s", rec.Code, rec.Body.String())
		}
		var e Entry
		_ = json.Unmarshal(rec.Body.Bytes(), &e)
		return h, e.ID
	}

	t.Run("fail default surfaces conflict and applies nothing", func(t *testing.T) {
		h, _ := seed(t)
		rec, resp := doImport(t, h, "p1", "?mode=apply", projectFiles())
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		if resp.Applied {
			t.Error("applied=true despite conflict under onConflict=fail")
		}
		found := false
		for _, row := range resp.Report {
			if row.Path == "characters/npcs/tom" && row.Action == "conflict" {
				found = true
			}
		}
		if !found {
			t.Errorf("no conflict row in %+v", resp.Report)
		}
		var n int
		_ = h.db.QueryRow(`SELECT COUNT(*) FROM entries`).Scan(&n)
		if n != 1 {
			t.Errorf("entries = %d, want only the pre-existing 1", n)
		}
	})

	t.Run("skip imports the rest", func(t *testing.T) {
		h, existingID := seed(t)
		_, resp := doImport(t, h, "p1", "?mode=apply&onConflict=skip", projectFiles())
		if !resp.Applied {
			t.Error("applied=false")
		}
		var body string
		_ = h.db.QueryRow(`SELECT title FROM entries WHERE id=?`, existingID).Scan(&body)
		if body != "Old Tom" {
			t.Errorf("skipped entry was modified: title=%q", body)
		}
		var n int
		_ = h.db.QueryRow(`SELECT COUNT(*) FROM entries`).Scan(&n)
		if n != 2 { // old tom + new inn
			t.Errorf("entries = %d, want 2", n)
		}
	})

	t.Run("overwrite keeps id, replaces content", func(t *testing.T) {
		h, existingID := seed(t)
		_, resp := doImport(t, h, "p1", "?mode=apply&onConflict=overwrite", projectFiles())
		if !resp.Applied {
			t.Error("applied=false")
		}
		var title string
		_ = h.db.QueryRow(`SELECT title FROM entries WHERE id=?`, existingID).Scan(&title)
		if title != "Tom" {
			t.Errorf("overwritten title = %q, want Tom (from imported frontmatter)", title)
		}
	})

	t.Run("suffix creates tom-2", func(t *testing.T) {
		h, _ := seed(t)
		_, resp := doImport(t, h, "p1", "?mode=apply&onConflict=suffix", projectFiles())
		if !resp.Applied {
			t.Error("applied=false")
		}
		var n int
		_ = h.db.QueryRow(`SELECT COUNT(*) FROM entries WHERE directory_path='characters/npcs' AND slug='tom-2'`).Scan(&n)
		if n != 1 {
			t.Error("tom-2 not created")
		}
	})
}

// TestImport_SuffixAvoidsBatchSiblingCollision guards against a bug where
// sequential suffix assignment could steal a later batch item's genuine
// slug for an earlier conflicting item: with a pre-existing "tom" and a
// batch containing both a conflicting "tom" folder and a distinct, genuine
// "tom-2" folder, naive processing would suffix the conflicting "tom" into
// "tom-2" (since the genuine tom-2 isn't inserted yet when "tom" is
// checked), silently displacing the real tom-2 content to "tom-2-2".
func TestImport_SuffixAvoidsBatchSiblingCollision(t *testing.T) {
	h, _, entries := newTestImportHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/content/projects/p1/entries",
		strings.NewReader(`{"slug":"tom","directoryPath":"characters/npcs","body":"---\nname: Old Tom\n---\n\nOld."}`))
	req = withChiParam(req, "id", "p1")
	rec := httptest.NewRecorder()
	entries.Create(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("seed create: %d %s", rec.Code, rec.Body.String())
	}

	files := map[string]string{
		"characters/npcs/tom/tom.md":     "---\nname: New Tom\n---\n\nNew Tom content.\n",
		"characters/npcs/tom-2/tom-2.md": "---\nname: Genuine Tom Two\n---\n\nGenuine second entry.\n",
	}
	_, resp := doImport(t, h, "p1", "?mode=apply&onConflict=suffix", files)
	if !resp.Applied {
		t.Fatalf("applied=false, report=%+v", resp.Report)
	}

	var genuineTitle string
	if err := h.db.QueryRow(`SELECT title FROM entries WHERE project_id='p1' AND directory_path='characters/npcs' AND slug='tom-2'`).Scan(&genuineTitle); err != nil {
		t.Fatalf("tom-2 row: %v", err)
	}
	if genuineTitle != "Genuine Tom Two" {
		t.Errorf("slug tom-2 title = %q, want %q (genuine batch sibling must not be displaced)", genuineTitle, "Genuine Tom Two")
	}

	var suffixedTitle string
	if err := h.db.QueryRow(`SELECT title FROM entries WHERE project_id='p1' AND directory_path='characters/npcs' AND slug='tom-3'`).Scan(&suffixedTitle); err != nil {
		t.Fatalf("expected the conflicting tom import to land at tom-3, got error: %v", err)
	}
	if suffixedTitle != "New Tom" {
		t.Errorf("slug tom-3 title = %q, want %q", suffixedTitle, "New Tom")
	}

	var n int
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM entries WHERE project_id='p1'`).Scan(&n)
	if n != 3 { // old tom + genuine tom-2 + suffixed tom-3
		t.Errorf("entries = %d, want 3", n)
	}
}

func TestImport_ReresolvesPreexistingDanglingLinks(t *testing.T) {
	h, _, entries := newTestImportHandler(t)
	// Existing entry pointing at not-yet-existing locations/inn.
	req := httptest.NewRequest(http.MethodPost, "/content/projects/p1/entries",
		strings.NewReader(`{"slug":"guide","body":"---\nname: Guide\n---\n\nSee [[locations/inn]].\n"}`))
	req = withChiParam(req, "id", "p1")
	rec := httptest.NewRecorder()
	entries.Create(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("seed: %d %s", rec.Code, rec.Body.String())
	}
	var guide Entry
	_ = json.Unmarshal(rec.Body.Bytes(), &guide)

	_, _ = doImport(t, h, "p1", "?mode=apply", map[string]string{"locations/inn/inn.md": innMD})

	var resolved bool
	var target string
	if err := h.db.QueryRow(`SELECT resolved, target_entry_id FROM entry_links WHERE entry_id=?`, guide.ID).Scan(&resolved, &target); err != nil {
		t.Fatalf("guide link: %v", err)
	}
	if !resolved || target == "" {
		t.Errorf("pre-existing dangling link not re-resolved: (%v, %q)", resolved, target)
	}
}

func TestImport_InvalidSlugRejected(t *testing.T) {
	h, _, _ := newTestImportHandler(t)
	r := chi.NewRouter()
	r.Post("/content/projects/{id}/import", h.ImportProject)
	req := withTestCaller(httptest.NewRequest(http.MethodPost, "/content/projects/p1/import",
		makeTarball(t, map[string]string{"Bad_Name/Bad_Name.md": "---\nname: Bad\n---\nx"})))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", rec.Code)
	}
}

func TestImport_ImagesStoredUnderEntryWithFilename(t *testing.T) {
	h, blobs, _ := newTestImportHandler(t)
	files := map[string]string{
		"tom/tom.md":           "---\nname: Tom\n---\n\n![portrait](tom-portrait.png)\n",
		"tom/tom-portrait.png": "PNGDATA",
		"tom/notes.txt":        "not an image",
	}
	_, resp := doImport(t, h, "p1", "?mode=apply", files)
	var tomID string
	if err := h.db.QueryRow(`SELECT id FROM entries WHERE slug='tom'`).Scan(&tomID); err != nil {
		t.Fatalf("tom row: %v", err)
	}
	if _, err := blobs.Get(context.Background(), "entries/"+tomID+"/images/tom-portrait.png"); err != nil {
		t.Error("image not stored under entries/{id}/images/<filename>")
	}
	// notes.txt is not an allowed image type → reported, not uploaded.
	var png, txt *ImportImageRow
	for i := range resp.Images {
		switch resp.Images[i].Path {
		case "tom/tom-portrait.png":
			png = &resp.Images[i]
		case "tom/notes.txt":
			txt = &resp.Images[i]
		}
	}
	if png == nil || !png.Uploaded {
		t.Errorf("png row = %+v, want uploaded=true", png)
	}
	if txt == nil || txt.Uploaded {
		t.Errorf("txt row = %+v, want uploaded=false", txt)
	}
}

// TestImport_BlobFailureUndo_PreservesPreexistingOverwrittenBlob guards
// against a data-loss bug: on a mid-batch blob Put failure, the undo loop
// used to delete every key in blobWrites, including bodyKey(existingID) for
// an onConflict=overwrite entry — a key that held data before this import
// ran. The DB side rolls back cleanly (deferred tx.Rollback), but blob
// deletion is not transactional: undo must never delete a preexisting key.
// This forces the *second* Put in the batch to fail (tom's overwrite body
// is written first and succeeds) so the failure path actually reaches the
// undo loop with a preexisting key already among blobWrites.
func TestImport_BlobFailureUndo_PreservesPreexistingOverwrittenBlob(t *testing.T) {
	h, blobs, entries := newTestImportHandler(t)

	// Seed an existing "tom" entry via the normal Create endpoint — its body
	// blob predates this import.
	req := httptest.NewRequest(http.MethodPost, "/content/projects/p1/entries",
		strings.NewReader(`{"slug":"tom","directoryPath":"characters/npcs","body":"---\nname: Old Tom\n---\n\nOld."}`))
	req = withChiParam(req, "id", "p1")
	rec := httptest.NewRecorder()
	entries.Create(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("seed create: %d %s", rec.Code, rec.Body.String())
	}
	var e Entry
	_ = json.Unmarshal(rec.Body.Bytes(), &e)
	if _, err := blobs.Get(context.Background(), bodyKey(e.ID)); err != nil {
		t.Fatalf("pre-existing body blob missing before import: %v", err)
	}

	// projectFiles() imports "characters/npcs/tom" (conflicts, overwrite) and
	// "locations/inn" (new). Sorted dir order puts tom's body Put first, so
	// failing after 1 call lets tom's overwrite Put succeed and fails on
	// inn's — forcing the undo loop to run with tom's preexisting key already
	// recorded in blobWrites.
	blobs.failAfter = 1

	rec2, _ := doImport(t, h, "p1", "?mode=apply&onConflict=overwrite", projectFiles())
	if rec2.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (forced blob failure); body %s", rec2.Code, rec2.Body.String())
	}

	// The forced failure must not have deleted the pre-existing overwritten
	// body blob — losing it entirely would be strictly worse than leaving
	// possibly-stale overwritten content in place.
	if _, err := blobs.Get(context.Background(), bodyKey(e.ID)); err != nil {
		t.Errorf("pre-existing body blob deleted by failed-apply undo: %v", err)
	}

	// DB side rolled back cleanly: still just the one pre-existing entry.
	var n int
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM entries`).Scan(&n)
	if n != 1 {
		t.Errorf("entries after failed apply = %d, want 1 (rollback)", n)
	}
}

func TestImport_BadTarballIs400(t *testing.T) {
	h, _, _ := newTestImportHandler(t)
	r := chi.NewRouter()
	r.Post("/content/projects/{id}/import", h.ImportProject)
	req := withTestCaller(httptest.NewRequest(http.MethodPost, "/content/projects/p1/import", strings.NewReader("not a tarball")))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestImport_BadParamsAre400(t *testing.T) {
	h, _, _ := newTestImportHandler(t)
	for _, q := range []string{"?mode=yolo", "?onConflict=merge"} {
		rec, _ := doImport(t, h, "p1", q, projectFiles())
		if rec.Code != http.StatusBadRequest {
			t.Errorf("query %q: status = %d, want 400", q, rec.Code)
		}
	}
}
