package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/quillit/auth-svc/internal/db"
	"github.com/quillit/auth-svc/internal/handler"
)

func TestUsernameAvailable(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer database.Close()

	if _, err := database.Exec(
		`INSERT INTO users (id, email, username, password_hash, role, created_at, updated_at) VALUES (?, ?, ?, ?, 'user', ?, ?)`,
		"u1", "taken@example.com", "takenname", "hash", 0, 0,
	); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	auth := handler.NewAuth(database, "test-secret", nil, "")

	t.Run("taken username", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/auth/users/available?username=takenname", nil)
		rec := httptest.NewRecorder()
		auth.UsernameAvailable(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		var resp struct {
			Available bool `json:"available"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp.Available {
			t.Error("expected available=false for a taken username")
		}
	})

	t.Run("free username", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/auth/users/available?username=freshname", nil)
		rec := httptest.NewRecorder()
		auth.UsernameAvailable(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		var resp struct {
			Available bool `json:"available"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if !resp.Available {
			t.Error("expected available=true for a free username")
		}
	})

	t.Run("missing username param", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/auth/users/available", nil)
		rec := httptest.NewRecorder()
		auth.UsernameAvailable(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})
}

func TestRegister_ConflictField(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer database.Close()

	if _, err := database.Exec(
		`INSERT INTO users (id, email, username, password_hash, role, created_at, updated_at) VALUES (?, ?, ?, ?, 'user', ?, ?)`,
		"u1", "taken@example.com", "takenname", "hash", 0, 0,
	); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	auth := handler.NewAuth(database, "test-secret", nil, "")

	t.Run("email conflict", func(t *testing.T) {
		body := `{"email":"taken@example.com","username":"freshname","password":"longenough1"}`
		req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(body))
		rec := httptest.NewRecorder()
		auth.Register(rec, req)

		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409", rec.Code)
		}
		var resp struct {
			Error string `json:"error"`
			Field string `json:"field"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp.Field != "email" {
			t.Errorf("field = %q, want %q", resp.Field, "email")
		}
	})

	t.Run("username conflict", func(t *testing.T) {
		body := `{"email":"fresh@example.com","username":"takenname","password":"longenough1"}`
		req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(body))
		rec := httptest.NewRecorder()
		auth.Register(rec, req)

		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409", rec.Code)
		}
		var resp struct {
			Error string `json:"error"`
			Field string `json:"field"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp.Field != "username" {
			t.Errorf("field = %q, want %q", resp.Field, "username")
		}
	})
}
