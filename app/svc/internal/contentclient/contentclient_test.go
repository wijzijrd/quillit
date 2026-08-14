package contentclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_Get_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/content/entries/e1" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "e1", "projectId": "proj-1", "title": "Mary", "body": "# Mary\n",
		})
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL}
	e, err := c.Get(context.Background(), "e1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if e.ID != "e1" || e.ProjectID != "proj-1" || e.Title != "Mary" || e.Body != "# Mary\n" {
		t.Errorf("unexpected entry: %+v", e)
	}
}

func TestClient_Get_EscapesEntryIDInPath(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "weird/id x", "projectId": "proj-1", "title": "Mary", "body": "# Mary\n",
		})
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL}
	_, err := c.Get(context.Background(), "weird/id x")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if want := "/content/entries/weird%2Fid%20x"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
}

func TestClient_Get_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL}
	_, err := c.Get(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
