package handler_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/quillit/svc/internal/handler"
)

func setupSettingsDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	schema := `
	CREATE TABLE user_settings (
		user_id    TEXT    PRIMARY KEY,
		settings   TEXT    NOT NULL DEFAULT '{}',
		updated_at INTEGER NOT NULL
	);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	return db
}

func settingsRequest(t *testing.T, db *sql.DB, method string, rawBody string, userID string) *httptest.ResponseRecorder {
	t.Helper()
	h := handler.NewSettings(db, "test-secret")

	r := chi.NewRouter()
	r.Get("/me/settings", h.Get)
	r.Patch("/me/settings", h.Update)

	req := httptest.NewRequest(method, "/me/settings", strings.NewReader(rawBody))
	req.Header.Set("Content-Type", "application/json")
	if userID != "" {
		req = req.WithContext(handler.WithTestCallerID(req.Context(), userID))
	}

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}

func TestSettingsGet_NoRowReturnsEmpty(t *testing.T) {
	db := setupSettingsDB(t)
	rr := settingsRequest(t, db, "GET", "", "user1")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if got := strings.TrimSpace(rr.Body.String()); got != "{}" {
		t.Errorf("expected {}, got %s", got)
	}
}

func TestSettingsUpdate_PersistsAndMerges(t *testing.T) {
	db := setupSettingsDB(t)

	rr := settingsRequest(t, db, "PATCH", `{"theme":"dark"}`, "user1")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Merge a second key; theme must survive.
	rr = settingsRequest(t, db, "PATCH", `{"lang":"en"}`, "user1")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	rr = settingsRequest(t, db, "GET", "", "user1")
	var settings map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&settings); err != nil {
		t.Fatal(err)
	}
	if settings["theme"] != "dark" || settings["lang"] != "en" {
		t.Errorf("expected merged settings, got %v", settings)
	}
}

func TestSettingsUpdate_NullDeletesKey(t *testing.T) {
	db := setupSettingsDB(t)
	settingsRequest(t, db, "PATCH", `{"theme":"dark","lang":"en"}`, "user1")

	rr := settingsRequest(t, db, "PATCH", `{"lang":null}`, "user1")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	rr = settingsRequest(t, db, "GET", "", "user1")
	var settings map[string]any
	json.NewDecoder(rr.Body).Decode(&settings)
	if _, exists := settings["lang"]; exists {
		t.Errorf("expected lang deleted, got %v", settings)
	}
	if settings["theme"] != "dark" {
		t.Errorf("expected theme preserved, got %v", settings)
	}
}

func TestSettingsUpdate_NonObjectBodyRejected(t *testing.T) {
	db := setupSettingsDB(t)
	for _, body := range []string{`[1,2]`, `"dark"`, `42`, ``} {
		rr := settingsRequest(t, db, "PATCH", body, "user1")
		if rr.Code != http.StatusBadRequest {
			t.Errorf("body %q: expected 400, got %d", body, rr.Code)
		}
	}
}

func TestSettingsUpdate_OversizedBodyRejected(t *testing.T) {
	db := setupSettingsDB(t)
	big := `{"data":"` + strings.Repeat("x", 5000) + `"}`
	rr := settingsRequest(t, db, "PATCH", big, "user1")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestSettings_UsersIsolated(t *testing.T) {
	db := setupSettingsDB(t)
	settingsRequest(t, db, "PATCH", `{"theme":"dark"}`, "user1")

	rr := settingsRequest(t, db, "GET", "", "user2")
	if got := strings.TrimSpace(rr.Body.String()); got != "{}" {
		t.Errorf("expected empty settings for user2, got %s", got)
	}
}

func TestSettings_Unauthorized(t *testing.T) {
	db := setupSettingsDB(t)
	rr := settingsRequest(t, db, "GET", "", "")
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
	rr = settingsRequest(t, db, "PATCH", `{"theme":"dark"}`, "")
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}
