package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestRenderHandler(t *testing.T) (*EntriesHandler, *RenderHandler) {
	t.Helper()
	entries, blobs := newTestEntriesHandler(t)
	return entries, NewRender(entries.db, blobs)
}

func renderEntry(t *testing.T, h *RenderHandler, id, query string) *httptest.ResponseRecorder {
	t.Helper()
	url := "/content/entries/" + id + "/render"
	if query != "" {
		url += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req = withChiParam(req, "id", id)
	w := httptest.NewRecorder()
	h.Render(w, req)
	return w
}

func TestRender_PlayerViewExcludesSecrets(t *testing.T) {
	entries, h := newTestRenderHandler(t)
	body := "Public intro.\n\n:::secret\nTom is secretly a spy.\n:::\n"
	e := createEntry(t, entries, "proj-1", createEntryRequest{Slug: "tom", Body: body})

	w := renderEntry(t, h, e.ID, "view=player")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "secretly a spy") {
		t.Errorf("player view leaked secret content: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Public intro") {
		t.Errorf("player view dropped non-secret content: %s", w.Body.String())
	}
}

func TestRender_NoHTMLWrapper(t *testing.T) {
	entries, h := newTestRenderHandler(t)
	e := createEntry(t, entries, "proj-1", createEntryRequest{Slug: "tom", Body: "Just prose."})

	w := renderEntry(t, h, e.ID, "")
	body := w.Body.String()
	if strings.Contains(strings.ToLower(body), "<html") || strings.Contains(strings.ToLower(body), "<body") {
		t.Errorf("expected a bare fragment, got wrapper tags: %s", body)
	}
}

func TestRender_UnknownFacetFailsLoud(t *testing.T) {
	entries, h := newTestRenderHandler(t)
	e := createEntry(t, entries, "proj-1", createEntryRequest{Slug: "tom", Body: "Prose only, no cards."})

	w := renderEntry(t, h, e.ID, "card=role")
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusUnprocessableEntity, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["facet"] != "role" {
		t.Errorf("facet = %v, want %q", resp["facet"], "role")
	}
	if vocab, ok := resp["vocabulary"].([]any); !ok || len(vocab) == 0 {
		t.Errorf("vocabulary = %v, want non-empty", resp["vocabulary"])
	}
}

func TestRender_ViewAndCardMutuallyExclusive(t *testing.T) {
	entries, h := newTestRenderHandler(t)
	e := createEntry(t, entries, "proj-1", createEntryRequest{Slug: "tom", Body: "Prose."})

	w := renderEntry(t, h, e.ID, "view=player&card=motivation")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestRender_CardViewExpandsLinkedEntryViaSQLiteResolver(t *testing.T) {
	entries, h := newTestRenderHandler(t)
	createEntry(t, entries, "proj-1", createEntryRequest{
		Slug: "mary", DirectoryPath: "characters/npcs",
		Body: ":::card motivation\nMary wants revenge on the duke.\n:::\n",
	})
	tom := createEntry(t, entries, "proj-1", createEntryRequest{
		Slug: "tom", DirectoryPath: "characters/npcs",
		Body: ":::card motivation\nTom talks about [[characters/npcs/mary|Mary]] often.\n:::\n",
	})

	w := renderEntry(t, h, tom.ID, "card=motivation")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "wants revenge on the duke") {
		t.Errorf("expected depth-1 card expansion of Mary's motivation card, got: %s", body)
	}
	if !strings.Contains(body, "card-block--expanded") {
		t.Errorf("expected expanded card to carry the expansion class, got: %s", body)
	}
}

func TestRender_NonExpandedLinkIsInAppNavReference(t *testing.T) {
	entries, h := newTestRenderHandler(t)
	createEntry(t, entries, "proj-1", createEntryRequest{Slug: "mary", DirectoryPath: "characters/npcs", Body: "Mary."})
	tom := createEntry(t, entries, "proj-1", createEntryRequest{
		Slug: "tom", DirectoryPath: "characters/npcs",
		Body: "Tom talks about [[characters/npcs/mary|Mary]] often.\n",
	})

	// DM (default) view: the link isn't inside a card, so it's never
	// depth-1-expanded — it must go through webLinkRenderer.
	w := renderEntry(t, h, tom.ID, "")
	body := w.Body.String()
	if !strings.Contains(body, `href="/entries/characters/npcs/mary"`) {
		t.Errorf("expected an in-app nav href, got: %s", body)
	}
	if !strings.Contains(body, `data-quillit-link="characters/npcs/mary"`) {
		t.Errorf("expected a data-quillit-link attribute, got: %s", body)
	}
	if strings.Contains(body, "quillit render") {
		t.Errorf("expected no CLI clipboard-link presentation, got: %s", body)
	}
}
