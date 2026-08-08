package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type EntryRelation struct {
	ID           string    `json:"id"`
	FromID       string    `json:"fromId"`
	ToID         string    `json:"toId"`
	Label        string    `json:"label"`
	Direction    string    `json:"direction"`
	RelatedEntry *EntryRef `json:"relatedEntry"`
	CreatedAt    int64     `json:"createdAt"`
}

type EntryRef struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Category string `json:"category"`
}

type RelationsHandler struct {
	db *sql.DB
}

func NewRelations(db *sql.DB) *RelationsHandler {
	return &RelationsHandler{db: db}
}

func (h *RelationsHandler) ListForEntry(w http.ResponseWriter, r *http.Request) {
	entryID := chi.URLParam(r, "id")

	out, err := h.scan(r, `
		SELECT er.id, er.from_id, er.to_id, er.label,
		       e.id, e.title, e.category, er.created_at
		FROM entry_relations er
		JOIN entries e ON e.id = er.to_id
		WHERE er.from_id = ?
		ORDER BY er.label, er.created_at`, entryID, "outgoing")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}

	inc, err := h.scan(r, `
		SELECT er.id, er.from_id, er.to_id, er.label,
		       e.id, e.title, e.category, er.created_at
		FROM entry_relations er
		JOIN entries e ON e.id = er.from_id
		WHERE er.to_id = ?
		ORDER BY er.label, er.created_at`, entryID, "incoming")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}

	writeJSON(w, http.StatusOK, append(out, inc...))
}

func (h *RelationsHandler) scan(r *http.Request, query, entryID, direction string) ([]EntryRelation, error) {
	rows, err := h.db.QueryContext(r.Context(), query, entryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []EntryRelation
	for rows.Next() {
		var rel EntryRelation
		var refID, refTitle, refCat string
		if err := rows.Scan(&rel.ID, &rel.FromID, &rel.ToID, &rel.Label,
			&refID, &refTitle, &refCat, &rel.CreatedAt); err != nil {
			return nil, err
		}
		rel.Direction = direction
		rel.RelatedEntry = &EntryRef{ID: refID, Title: refTitle, Category: refCat}
		results = append(results, rel)
	}
	return results, nil
}

func (h *RelationsHandler) Create(w http.ResponseWriter, r *http.Request) {
	fromID := chi.URLParam(r, "id")
	var req struct {
		ToID  string `json:"toId"`
		Label string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ToID == "" || req.Label == "" {
		writeError(w, http.StatusBadRequest, "toId and label required")
		return
	}
	if fromID == req.ToID {
		writeError(w, http.StatusBadRequest, "cannot link entry to itself")
		return
	}

	id := newID()
	now := nowUnix()
	if _, err := h.db.ExecContext(r.Context(),
		`INSERT OR IGNORE INTO entry_relations (id, from_id, to_id, label, created_at) VALUES (?, ?, ?, ?, ?)`,
		id, fromID, req.ToID, req.Label, now,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "insert failed")
		return
	}

	var rel EntryRelation
	var refID, refTitle, refCat string
	if err := h.db.QueryRowContext(r.Context(), `
		SELECT er.id, er.from_id, er.to_id, er.label,
		       e.id, e.title, e.category, er.created_at
		FROM entry_relations er JOIN entries e ON e.id = er.to_id
		WHERE er.from_id = ? AND er.to_id = ? AND er.label = ?`,
		fromID, req.ToID, req.Label,
	).Scan(&rel.ID, &rel.FromID, &rel.ToID, &rel.Label,
		&refID, &refTitle, &refCat, &rel.CreatedAt); err != nil {
		writeError(w, http.StatusInternalServerError, "fetch failed")
		return
	}
	rel.Direction = "outgoing"
	rel.RelatedEntry = &EntryRef{ID: refID, Title: refTitle, Category: refCat}
	writeJSON(w, http.StatusCreated, rel)
}

func (h *RelationsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "relationId")
	if _, err := h.db.ExecContext(r.Context(),
		`DELETE FROM entry_relations WHERE id = ?`, id,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *RelationsHandler) Labels(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.QueryContext(r.Context(),
		`SELECT DISTINCT label FROM entry_relations ORDER BY label`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	labels := []string{}
	for rows.Next() {
		var l string
		_ = rows.Scan(&l)
		labels = append(labels, l)
	}
	writeJSON(w, http.StatusOK, labels)
}
