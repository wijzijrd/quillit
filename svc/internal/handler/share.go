package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type ShareHandler struct{ db *sql.DB }

func NewShare(db *sql.DB) *ShareHandler { return &ShareHandler{db: db} }

// GetEntries godoc
// @Summary      Get shared entries
// @Description  Returns public entries visible to the player identified by token.
// @Tags         share
// @Produce      json
// @Param        token  path      string  true  "Player share token"
// @Success      200    {array}   Entry
// @Failure      404    {object}  ErrorResponse
// @Router       /api/share/{token} [get]
func (h *ShareHandler) GetEntries(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")

	var campaignID string
	err := h.db.QueryRowContext(r.Context(), "SELECT campaign_id FROM players WHERE token = ?", token).Scan(&campaignID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "invalid token")
		return
	}

	rows, err := h.db.QueryContext(r.Context(),
		entrySelect+` WHERE visibility = 'public' AND EXISTS (
			SELECT 1 FROM json_each(entries.campaign_ids) je WHERE je.value = ?
		)`,
		campaignID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()

	entries := []Entry{}
	for rows.Next() {
		if e, err := scanEntry(rows); err == nil {
			entries = append(entries, e)
		}
	}
	writeJSON(w, http.StatusOK, entries)
}

// ListNotes godoc
// @Summary      List player notes
// @Tags         share
// @Produce      json
// @Param        token  path      string  true  "Player share token"
// @Success      200    {array}   PlayerNote
// @Failure      404    {object}  ErrorResponse
// @Router       /api/share/{token}/notes [get]
func (h *ShareHandler) ListNotes(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if !h.tokenExists(r, token) {
		writeError(w, http.StatusNotFound, "invalid token")
		return
	}
	rows, err := h.db.QueryContext(r.Context(),
		`SELECT id, token, title, body, category, visibility, tags, created_at, updated_at FROM player_notes WHERE token = ? ORDER BY created_at ASC`, token,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()
	notes := []PlayerNote{}
	for rows.Next() {
		var n PlayerNote
		if err := rows.Scan(&n.ID, &n.Token, &n.Title, &n.Body, &n.Category, &n.Visibility, &n.Tags, &n.CreatedAt, &n.UpdatedAt); err == nil {
			notes = append(notes, n)
		}
	}
	writeJSON(w, http.StatusOK, notes)
}

// CreateNoteRequest is the body for creating a player note.
type CreateNoteRequest struct {
	Title    string `json:"title"`
	Body     string `json:"body"`
	Category string `json:"category"`
	Tags     string `json:"tags"`
}

// CreateNote godoc
// @Summary      Create player note
// @Tags         share
// @Accept       json
// @Produce      json
// @Param        token  path      string             true  "Player share token"
// @Param        body   body      CreateNoteRequest  true  "Note data"
// @Success      201    {object}  PlayerNote
// @Failure      404    {object}  ErrorResponse
// @Router       /api/share/{token}/notes [post]
func (h *ShareHandler) CreateNote(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if !h.tokenExists(r, token) {
		writeError(w, http.StatusNotFound, "invalid token")
		return
	}
	var body struct {
		Title    string `json:"title"`
		Body     string `json:"body"`
		Category string `json:"category"`
		Tags     string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}
	defaults(&body.Category, "Note")
	defaults(&body.Tags, "[]")

	now := nowUnix()
	n := PlayerNote{ID: newID(), Token: token, Title: body.Title, Body: body.Body, Category: body.Category, Visibility: "private", Tags: body.Tags, CreatedAt: now, UpdatedAt: now}
	if _, err := h.db.ExecContext(r.Context(),
		`INSERT INTO player_notes (id,token,title,body,category,visibility,tags,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?)`,
		n.ID, n.Token, n.Title, n.Body, n.Category, n.Visibility, n.Tags, n.CreatedAt, n.UpdatedAt,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusCreated, n)
}

// UpdateNote godoc
// @Summary      Update player note
// @Tags         share
// @Accept       json
// @Produce      json
// @Param        token   path      string             true  "Player share token"
// @Param        noteId  path      string             true  "Note ID"
// @Param        body    body      CreateNoteRequest  true  "Fields to update"
// @Success      200     {object}  PlayerNote
// @Router       /api/share/{token}/notes/{noteId} [patch]
func (h *ShareHandler) UpdateNote(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	noteID := chi.URLParam(r, "noteId")
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}
	body["updated_at"] = nowUnix()
	allowed := map[string]string{
		"title": "title", "body": "body", "category": "category", "tags": "tags", "updated_at": "updated_at",
	}
	setClauses, args := buildSet(body, allowed)
	if len(setClauses) == 0 {
		writeError(w, http.StatusBadRequest, "no valid fields")
		return
	}
	args = append(args, noteID, token)
	if _, err := h.db.ExecContext(r.Context(), "UPDATE player_notes SET "+setClauses+" WHERE id = ? AND token = ?", args...); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	var n PlayerNote
	_ = h.db.QueryRowContext(r.Context(),
		`SELECT id, token, title, body, category, visibility, tags, created_at, updated_at FROM player_notes WHERE id = ?`, noteID,
	).Scan(&n.ID, &n.Token, &n.Title, &n.Body, &n.Category, &n.Visibility, &n.Tags, &n.CreatedAt, &n.UpdatedAt)
	writeJSON(w, http.StatusOK, n)
}

// DeleteNote godoc
// @Summary      Delete player note
// @Tags         share
// @Produce      json
// @Param        token   path      string      true  "Player share token"
// @Param        noteId  path      string      true  "Note ID"
// @Success      200     {object}  OkResponse
// @Router       /api/share/{token}/notes/{noteId} [delete]
func (h *ShareHandler) DeleteNote(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	noteID := chi.URLParam(r, "noteId")
	if _, err := h.db.ExecContext(r.Context(), "DELETE FROM player_notes WHERE id = ? AND token = ?", noteID, token); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *ShareHandler) tokenExists(r *http.Request, token string) bool {
	var count int
	_ = h.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM players WHERE token = ?", token).Scan(&count)
	return count > 0
}

type PlayerNote struct {
	ID         string `json:"id"`
	Token      string `json:"token"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	Category   string `json:"category"`
	Visibility string `json:"visibility"`
	Tags       string `json:"tags"`
	CreatedAt  int64  `json:"createdAt"`
	UpdatedAt  int64  `json:"updatedAt"`
}
