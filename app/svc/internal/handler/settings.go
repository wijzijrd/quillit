package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/quillit/svc/internal/middleware"
)

// maxSettingsBytes caps both the request body and the stored settings blob.
const maxSettingsBytes = 4096

type SettingsHandler struct {
	db        *sql.DB
	jwtSecret []byte
}

func NewSettings(db *sql.DB, jwtSecret string) *SettingsHandler {
	return &SettingsHandler{db: db, jwtSecret: []byte(jwtSecret)}
}

func (h *SettingsHandler) callerID(r *http.Request) (string, bool) {
	// Test helper: allow injecting caller ID without JWT.
	if id, ok := r.Context().Value(testCallerKey{}).(string); ok && id != "" {
		return id, true
	}
	mc, err := middleware.ClaimsFromContext(r.Context(), h.jwtSecret)
	if err != nil {
		return "", false
	}
	sub, _ := mc["sub"].(string)
	return sub, sub != ""
}

func (h *SettingsHandler) load(r *http.Request, userID string) (map[string]any, error) {
	var raw string
	err := h.db.QueryRowContext(r.Context(),
		"SELECT settings FROM user_settings WHERE user_id = ?", userID,
	).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	settings := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		// Corrupt blob: treat as empty rather than failing the request.
		return map[string]any{}, nil
	}
	return settings, nil
}

// Get godoc
// @Summary      Get the caller's settings
// @Tags         settings
// @Produce      json
// @Success      200  {object}  map[string]any
// @Router       /api/me/settings [get]
func (h *SettingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	settings, err := h.load(r, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

// Update godoc
// @Summary      Merge settings into the caller's stored settings
// @Description  Shallow merge: each top-level key in the body overwrites the stored key; a null value deletes the key.
// @Tags         settings
// @Accept       json
// @Produce      json
// @Param        body  body      map[string]any  true  "Settings patch (JSON object)"
// @Success      200   {object}  map[string]any
// @Router       /api/me/settings [patch]
func (h *SettingsHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxSettingsBytes)
	var patch map[string]any
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "settings must be a JSON object")
		return
	}

	settings, err := h.load(r, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	for k, v := range patch {
		if v == nil {
			delete(settings, k)
		} else {
			settings[k] = v
		}
	}

	merged, err := json.Marshal(settings)
	if err != nil || len(merged) > maxSettingsBytes {
		writeError(w, http.StatusBadRequest, "settings too large")
		return
	}

	_, err = h.db.ExecContext(r.Context(),
		"INSERT INTO user_settings (user_id, settings, updated_at) VALUES (?, ?, ?) ON CONFLICT(user_id) DO UPDATE SET settings = excluded.settings, updated_at = excluded.updated_at",
		userID, string(merged), nowUnix(),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}
