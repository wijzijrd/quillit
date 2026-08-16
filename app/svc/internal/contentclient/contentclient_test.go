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

func TestClient_NotifyProjectDeleted_Success(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL}
	if err := c.NotifyProjectDeleted(context.Background(), "proj-1"); err != nil {
		t.Fatalf("NotifyProjectDeleted: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/content/internal/projects/proj-1/deleted" {
		t.Errorf("path = %q", gotPath)
	}
}

func TestClient_NotifyProjectDeleted_EscapesProjectIDInPath(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL}
	if err := c.NotifyProjectDeleted(context.Background(), "weird/id x"); err != nil {
		t.Fatalf("NotifyProjectDeleted: %v", err)
	}
	if want := "/content/internal/projects/weird%2Fid%20x/deleted"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
}

func TestClient_NotifyProjectDeleted_NonOKStatusIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL}
	if err := c.NotifyProjectDeleted(context.Background(), "proj-1"); err == nil {
		t.Error("expected an error for a 500 response, got nil")
	}
}

func TestClient_NotifyProjectDeleted_UnreachableServerIsError(t *testing.T) {
	c := &Client{BaseURL: "http://127.0.0.1:1"} // nothing listens here
	if err := c.NotifyProjectDeleted(context.Background(), "proj-1"); err == nil {
		t.Error("expected an error for an unreachable content service, got nil")
	}
}
