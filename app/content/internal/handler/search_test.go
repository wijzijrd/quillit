package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/quillit/content-svc/internal/db"
)

// newTestSearchHandler wires an EntriesHandler (so tests can create/update
// entries through the same write path production traffic uses) and a
// SearchHandler against the same in-memory database — search must see
// exactly what entries.go just wrote, in the same request cycle.
func newTestSearchHandler(t *testing.T) (*EntriesHandler, *SearchHandler) {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	blobs := newFakeBlobStore()
	return NewEntries(database, "", blobs), NewSearch(database)
}

func search(t *testing.T, h *SearchHandler, projectID, q, mode string) (*httptest.ResponseRecorder, []SearchResult) {
	t.Helper()
	url := "/content/projects/" + projectID + "/search?q=" + q
	if mode != "" {
		url += "&mode=" + mode
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req = withChiParam(req, "id", projectID)
	w := httptest.NewRecorder()
	h.Search(w, req)
	var results []SearchResult
	if w.Code == http.StatusOK {
		_ = json.Unmarshal(w.Body.Bytes(), &results)
	}
	return w, results
}

func TestSearch_IndexedSynchronouslyOnCreate(t *testing.T) {
	entries, sh := newTestSearchHandler(t)
	createEntry(t, entries, "proj-1", createEntryRequest{
		Slug: "tom", Body: "---\nname: Tom the Innkeeper\n---\n\nRuns the tavern.",
	})

	w, results := search(t, sh, "proj-1", "Innkeeper", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if len(results) != 1 || results[0].Title != "Tom the Innkeeper" {
		t.Errorf("results = %+v, want a single match for Tom the Innkeeper", results)
	}
}

func TestSearch_IndexedSynchronouslyOnUpdate(t *testing.T) {
	entries, sh := newTestSearchHandler(t)
	e := createEntry(t, entries, "proj-1", createEntryRequest{Slug: "mary", Body: "Mary the merchant."})

	newBody := "Mary the merchant now sells enchanted trinkets."
	patch := updateEntryRequest{Body: &newBody}
	raw, _ := json.Marshal(patch)
	req := httptest.NewRequest(http.MethodPatch, "/content/entries/"+e.ID, strings.NewReader(string(raw)))
	req = withChiParam(req, "id", e.ID)
	w := httptest.NewRecorder()
	entries.Update(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Update() status = %d, body = %s", w.Code, w.Body.String())
	}

	_, results := search(t, sh, "proj-1", "enchanted", "")
	if len(results) != 1 || results[0].ID != e.ID {
		t.Errorf("results = %+v, want the updated entry to be searchable for its new content", results)
	}
}

func TestSearch_MatchesBodyNotJustTitleTags(t *testing.T) {
	entries, sh := newTestSearchHandler(t)
	body := "---\nname: Tom the Innkeeper\ntags: [npc]\n---\n\n" +
		":::secret\nSecretly a retired dragonslayer named Skarnfeld.\n:::\n\n" +
		":::card motivation\nWants his farm back.\n:::\n"
	createEntry(t, entries, "proj-1", createEntryRequest{Slug: "tom", Body: body})

	// "Skarnfeld" appears only inside the secret block — not in title or
	// tags — so a match proves search covers full body content.
	_, results := search(t, sh, "proj-1", "Skarnfeld", "")
	if len(results) != 1 || results[0].Title != "Tom the Innkeeper" {
		t.Errorf("results = %+v, want a match on secret-block content", results)
	}

	// Same for card content.
	_, cardResults := search(t, sh, "proj-1", "farm", "")
	if len(cardResults) != 1 || cardResults[0].Title != "Tom the Innkeeper" {
		t.Errorf("results = %+v, want a match on card-block content", cardResults)
	}

	// The raw ":::secret" fence marker itself must not leak into the index
	// as a searchable token — the entry's prose never contains the bare
	// word "secret" (only "Secretly", a different token), so a match here
	// would mean the directive syntax, not the prose, got indexed.
	_, fenceSyntax := search(t, sh, "proj-1", "secret", "")
	if len(fenceSyntax) != 0 {
		t.Errorf("results = %+v, want no match for the bare directive marker %q", fenceSyntax, "secret")
	}
}

func TestSearch_ScopedByProject(t *testing.T) {
	entries, sh := newTestSearchHandler(t)
	createEntry(t, entries, "proj-a", createEntryRequest{Slug: "mary", Body: "Mary the wandering merchant."})
	createEntry(t, entries, "proj-b", createEntryRequest{Slug: "john", Body: "John the wandering knight."})

	_, results := search(t, sh, "proj-a", "wandering", "")
	if len(results) != 1 || results[0].Slug != "mary" {
		t.Errorf("results = %+v, want only proj-a's entry", results)
	}

	_, resultsB := search(t, sh, "proj-b", "wandering", "")
	if len(resultsB) != 1 || resultsB[0].Slug != "john" {
		t.Errorf("results = %+v, want only proj-b's entry", resultsB)
	}
}

func TestSearchLookup_ReturnsWikilinkFields(t *testing.T) {
	entries, sh := newTestSearchHandler(t)
	createEntry(t, entries, "proj-1", createEntryRequest{
		Slug: "mary", DirectoryPath: "characters/npcs", Body: "---\nname: Mary the Merchant\n---\n\nSells wares.",
	})
	createEntry(t, entries, "proj-1", createEntryRequest{
		Slug: "tom", DirectoryPath: "characters/npcs", Body: "---\nname: Tom the Innkeeper\n---\n\nRuns the tavern.",
	})

	w, results := search(t, sh, "proj-1", "Mar", "lookup")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if len(results) != 1 {
		t.Fatalf("results = %+v, want exactly one prefix match for %q", results, "Mar")
	}
	got := results[0]
	if got.DirectoryPath != "characters/npcs" || got.Slug != "mary" || got.Title != "Mary the Merchant" {
		t.Errorf("result = %+v, want enough fields to build [[characters/npcs/mary|Mary the Merchant]]", got)
	}
}
