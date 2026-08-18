package client

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLogin_ReturnsSessionCookie(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/auth/login" || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "JWT123", HttpOnly: true})
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	token, err := Login(srv.URL, "dm@example.com", "secret")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if token != "JWT123" {
		t.Errorf("token = %q, want JWT123", token)
	}
}

func TestLogin_BadCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid credentials"}`))
	}))
	defer srv.Close()
	if _, err := Login(srv.URL, "dm@example.com", "wrong"); err == nil {
		t.Error("bad credentials accepted")
	}
}

func TestListProjects_SendsSessionCookie(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("session"); err != nil || c.Value != "TOK" {
			t.Errorf("session cookie = %v, %v", c, err)
		}
		_, _ = w.Write([]byte(`[{"id":"p1","name":"curse-of-strahd"}]`))
	}))
	defer srv.Close()
	c := &Client{Server: srv.URL, Token: "TOK"}
	ps, err := c.ListProjects()
	if err != nil || len(ps) != 1 || ps[0].ID != "p1" {
		t.Errorf("ListProjects = %+v, %v", ps, err)
	}
}

func TestImport_401IsErrUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	c := &Client{Server: srv.URL, Token: "expired"}
	_, err := c.Import("p1", strings.NewReader("x"), ImportOptions{Mode: "dry-run", OnConflict: "fail"})
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("err = %v, want ErrUnauthorized", err)
	}
}

func TestImport_SendsParamsAndReturnsReport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/content/projects/p1/import" {
			t.Errorf("path = %s", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("mode") != "apply" || q.Get("onConflict") != "skip" || q.Get("createFacets") != "true" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"applied":true,"report":[{"path":"a/b","action":"create"}],"facets":{"created":[]},"images":[]}`))
	}))
	defer srv.Close()
	c := &Client{Server: srv.URL, Token: "TOK"}
	resp, err := c.Import("p1", strings.NewReader("TAR"), ImportOptions{Mode: "apply", OnConflict: "skip", CreateFacets: true})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if !resp.Applied || len(resp.Report) != 1 || resp.Report[0].Action != "create" {
		t.Errorf("resp = %+v", resp)
	}
}

func TestImport_422IsValidationError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":"validation failed","entries":[{"path":"a/a","error":"unclosed block"}],"missingFacets":["stat-block"]}`))
	}))
	defer srv.Close()
	c := &Client{Server: srv.URL, Token: "TOK"}
	_, err := c.Import("p1", strings.NewReader("TAR"), ImportOptions{Mode: "dry-run", OnConflict: "fail"})
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want *ValidationError", err)
	}
	if len(ve.Entries) != 1 || len(ve.MissingFacets) != 1 {
		t.Errorf("ve = %+v", ve)
	}
}

func TestListEntries_ParsesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/content/projects/proj-1/entries" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[
			{"id":"e1","slug":"tom","directoryPath":"characters/npcs","bodyHash":"abc123"},
			{"id":"e2","slug":"inn","directoryPath":"locations","bodyHash":""}
		]`))
	}))
	defer srv.Close()
	c := &Client{Server: srv.URL, Token: "TOK"}

	entries, err := c.ListEntries("proj-1")
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].Slug != "tom" || entries[0].DirectoryPath != "characters/npcs" || entries[0].BodyHash != "abc123" {
		t.Errorf("entries[0] = %+v, want slug=tom directoryPath=characters/npcs bodyHash=abc123", entries[0])
	}
	if entries[1].BodyHash != "" {
		t.Errorf("entries[1].BodyHash = %q, want empty (unset remote hash)", entries[1].BodyHash)
	}
}

func TestListEntries_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"not a project member"}`))
	}))
	defer srv.Close()
	c := &Client{Server: srv.URL, Token: "TOK"}

	if _, err := c.ListEntries("proj-1"); err == nil {
		t.Error("expected an error for a 403 response")
	}
}
