package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type AnnotationsHandler struct{ db *sql.DB }

func NewAnnotations(db *sql.DB) *AnnotationsHandler { return &AnnotationsHandler{db: db} }

type Annotation struct {
	ID         string `json:"id"`
	EntryID    string `json:"entryId"`
	Text       string `json:"text"`
	Visibility string `json:"visibility"`
	SharedWith string `json:"sharedWith"`
	CreatedAt  int64  `json:"createdAt"`
	UpdatedAt  int64  `json:"updatedAt"`
}

const annoSelect = `SELECT id, entry_id, text, visibility, shared_with, created_at, updated_at FROM annotations`

func scanAnnotation(row interface{ Scan(...any) error }) (Annotation, error) {
	var a Annotation
	return a, row.Scan(&a.ID, &a.EntryID, &a.Text, &a.Visibility, &a.SharedWith, &a.CreatedAt, &a.UpdatedAt)
}

// List godoc
// @Summary      List annotations
// @Tags         annotations
// @Produce      json
// @Param        entryId  query     string  false  "Filter by entry ID"
// @Success      200      {array}   Annotation
// @Router       /api/annotations [get]
func (h *AnnotationsHandler) List(w http.ResponseWriter, r *http.Request) {
	q := annoSelect + " WHERE 1=1"
	args := []any{}
	if entryID := r.URL.Query().Get("entryId"); entryID != "" {
		q += " AND entry_id = ?"
		args = append(args, entryID)
	}
	q += " ORDER BY created_at ASC"
	rows, err := h.db.QueryContext(r.Context(), q, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()
	annotations := []Annotation{}
	for rows.Next() {
		if a, err := scanAnnotation(rows); err == nil {
			annotations = append(annotations, a)
		}
	}
	writeJSON(w, http.StatusOK, annotations)
}

// CreateAnnotationRequest is the body for creating an annotation.
type CreateAnnotationRequest struct {
	EntryID    string `json:"entryId"`
	Text       string `json:"text"`
	Visibility string `json:"visibility"`
	SharedWith string `json:"sharedWith"`
}

// Create godoc
// @Summary      Create annotation
// @Tags         annotations
// @Accept       json
// @Produce      json
// @Param        body  body      CreateAnnotationRequest  true  "Annotation data"
// @Success      201   {object}  Annotation
// @Failure      400   {object}  ErrorResponse
// @Router       /api/annotations [post]
func (h *AnnotationsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		EntryID    string `json:"entryId"`
		Text       string `json:"text"`
		Visibility string `json:"visibility"`
		SharedWith string `json:"sharedWith"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.EntryID == "" {
		writeError(w, http.StatusBadRequest, "entryId required")
		return
	}
	defaults(&body.Visibility, "gm")
	defaults(&body.SharedWith, "[]")

	now := nowUnix()
	a := Annotation{ID: newID(), EntryID: body.EntryID, Text: body.Text, Visibility: body.Visibility, SharedWith: body.SharedWith, CreatedAt: now, UpdatedAt: now}
	if _, err := h.db.ExecContext(r.Context(),
		`INSERT INTO annotations (id,entry_id,text,visibility,shared_with,created_at,updated_at) VALUES (?,?,?,?,?,?,?)`,
		a.ID, a.EntryID, a.Text, a.Visibility, a.SharedWith, a.CreatedAt, a.UpdatedAt,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

// Update godoc
// @Summary      Update annotation
// @Description  Partial update — send only the fields to change.
// @Tags         annotations
// @Accept       json
// @Produce      json
// @Param        id    path      string                   true  "Annotation ID"
// @Param        body  body      CreateAnnotationRequest  true  "Fields to update"
// @Success      200   {object}  Annotation
// @Failure      404   {object}  ErrorResponse
// @Router       /api/annotations/{id} [patch]
func (h *AnnotationsHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}
	body["updated_at"] = nowUnix()
	allowed := map[string]string{
		"text": "text", "visibility": "visibility", "sharedWith": "shared_with", "updated_at": "updated_at",
	}
	setClauses, args := buildSet(body, allowed)
	if len(setClauses) == 0 {
		writeError(w, http.StatusBadRequest, "no valid fields")
		return
	}
	args = append(args, id)
	if _, err := h.db.ExecContext(r.Context(), "UPDATE annotations SET "+setClauses+" WHERE id = ?", args...); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	a, err := scanAnnotation(h.db.QueryRowContext(r.Context(), annoSelect+" WHERE id = ?", id))
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// Delete godoc
// @Summary      Delete annotation
// @Tags         annotations
// @Produce      json
// @Param        id   path      string      true  "Annotation ID"
// @Success      200  {object}  OkResponse
// @Router       /api/annotations/{id} [delete]
func (h *AnnotationsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := h.db.ExecContext(r.Context(), "DELETE FROM annotations WHERE id = ?", id); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

var _ = sql.ErrNoRows // keep import
