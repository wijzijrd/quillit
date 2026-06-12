package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/quillit/svc/internal/middleware"
)

type EntrySharesHandler struct {
	db        *sql.DB
	jwtSecret []byte
	authURL   string
}

func NewEntryShares(db *sql.DB, jwtSecret, authURL string) *EntrySharesHandler {
	return &EntrySharesHandler{db: db, jwtSecret: []byte(jwtSecret), authURL: authURL}
}

func (h *EntrySharesHandler) callerID(r *http.Request) (string, bool) {
	mc, err := middleware.ClaimsFromContext(r.Context(), h.jwtSecret)
	if err != nil {
		return "", false
	}
	sub, _ := mc["sub"].(string)
	return sub, sub != ""
}

// isOwnerOrEditor checks whether the caller owns the entry or holds an editor role
// in any project the entry belongs to. For now we check ownership only.
func (h *EntrySharesHandler) canManageShares(r *http.Request, entryID string) bool {
	callerID, ok := h.callerID(r)
	if !ok {
		return false
	}
	var count int
	_ = h.db.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM entries WHERE id = ? AND owner_user_id = ?`,
		entryID, callerID,
	).Scan(&count)
	return count > 0
}

// EntryShare represents a single share grant.
type EntryShare struct {
	ID       string `json:"id"`
	EntryID  string `json:"entryId"`
	UserID   string `json:"userId"`
	SharedBy string `json:"sharedBy"`
	SharedAt int64  `json:"sharedAt"`
}

// ListShares godoc
// @Summary      List entry shares
// @Description  Returns all users who have been granted access to an entry.
// @Tags         shares
// @Produce      json
// @Param        id  path  string  true  "Entry ID"
// @Success      200  {array}   EntryShare
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /api/entries/{id}/shares [get]
func (h *EntrySharesHandler) ListShares(w http.ResponseWriter, r *http.Request) {
	entryID := chi.URLParam(r, "id")
	if !h.canManageShares(r, entryID) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	rows, err := h.db.QueryContext(r.Context(),
		`SELECT id, entry_id, user_id, shared_by, shared_at FROM entry_shares WHERE entry_id = ? ORDER BY shared_at ASC`,
		entryID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()

	shares := []EntryShare{}
	for rows.Next() {
		var s EntryShare
		if err := rows.Scan(&s.ID, &s.EntryID, &s.UserID, &s.SharedBy, &s.SharedAt); err == nil {
			shares = append(shares, s)
		}
	}
	writeJSON(w, http.StatusOK, shares)
}

// AddShares godoc
// @Summary      Share entry with users
// @Description  Grants access to an entry for one or more user IDs. Caller must be the entry owner.
// @Tags         shares
// @Accept       json
// @Produce      json
// @Param        id    path  string  true  "Entry ID"
// @Param        body  body  object  true  "{ userIds: [\"...\"] }"
// @Success      200  {object}  OkResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Router       /api/entries/{id}/shares [post]
func (h *EntrySharesHandler) AddShares(w http.ResponseWriter, r *http.Request) {
	entryID := chi.URLParam(r, "id")
	callerID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !h.canManageShares(r, entryID) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	var body struct {
		UserIDs []string `json:"userIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.UserIDs) == 0 {
		writeError(w, http.StatusBadRequest, "userIds required")
		return
	}

	now := nowUnix()
	for _, uid := range body.UserIDs {
		_, _ = h.db.ExecContext(r.Context(),
			`INSERT OR IGNORE INTO entry_shares (id, entry_id, user_id, shared_by, shared_at) VALUES (?,?,?,?,?)`,
			newID(), entryID, uid, callerID, now,
		)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// RemoveShare godoc
// @Summary      Remove entry share
// @Description  Revokes a user's access to an entry. Caller must be the entry owner.
// @Tags         shares
// @Produce      json
// @Param        id      path  string  true  "Entry ID"
// @Param        userId  path  string  true  "User ID to remove"
// @Success      200  {object}  OkResponse
// @Failure      403  {object}  ErrorResponse
// @Router       /api/entries/{id}/shares/{userId} [delete]
func (h *EntrySharesHandler) RemoveShare(w http.ResponseWriter, r *http.Request) {
	entryID := chi.URLParam(r, "id")
	userID := chi.URLParam(r, "userId")
	if !h.canManageShares(r, entryID) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	if _, err := h.db.ExecContext(r.Context(),
		`DELETE FROM entry_shares WHERE entry_id = ? AND user_id = ?`,
		entryID, userID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// SearchUsers godoc
// @Summary      Search users
// @Description  Proxies to auth-svc user search. Returns minimal user info for the share panel.
// @Tags         shares
// @Produce      json
// @Param        q  query  string  false  "Search term (email or username)"
// @Success      200  {array}  object
// @Failure      502  {object}  ErrorResponse
// @Router       /api/users/search [get]
func (h *EntrySharesHandler) SearchUsers(w http.ResponseWriter, r *http.Request) {
	raw, ok := middleware.RawJWTFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	q := r.URL.Query().Get("q")
	url := fmt.Sprintf("%s/auth/users/search?q=%s", h.authURL, q)

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, url, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	req.Header.Set("Authorization", "Bearer "+raw)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "auth service unavailable")
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}

// isEntrySharedWith checks whether the given user has been granted access to the entry.
func (h *EntrySharesHandler) isEntrySharedWith(r *http.Request, entryID, userID string) bool {
	var count int
	_ = h.db.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM entry_shares WHERE entry_id = ? AND user_id = ?`,
		entryID, userID,
	).Scan(&count)
	return count > 0
}

// isOwner checks whether the given user is the owner of the entry.
func isEntryOwner(db *sql.DB, r *http.Request, entryID, userID string) (bool, error) {
	var ownerID string
	err := db.QueryRowContext(r.Context(), `SELECT COALESCE(owner_user_id,'') FROM entries WHERE id = ?`, entryID).Scan(&ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("entry not found")
	}
	return ownerID == userID, err
}
