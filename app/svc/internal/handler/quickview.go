package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type QuickViewHandler struct{ db *sql.DB }

func NewQuickView(db *sql.DB) *QuickViewHandler { return &QuickViewHandler{db: db} }

type QuickViewTemplate struct {
	Category string `json:"category"`
	Fields   string `json:"fields"`
}

// List godoc
// @Summary      List quick-view templates
// @Tags         quickview
// @Produce      json
// @Success      200  {array}   QuickViewTemplate
// @Router       /api/quickview [get]
func (h *QuickViewHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.QueryContext(r.Context(), "SELECT category, fields FROM quick_view_templates ORDER BY category ASC")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()
	templates := []QuickViewTemplate{}
	for rows.Next() {
		var t QuickViewTemplate
		if err := rows.Scan(&t.Category, &t.Fields); err == nil {
			templates = append(templates, t)
		}
	}
	writeJSON(w, http.StatusOK, templates)
}

// UpsertQuickViewRequest is the body for upserting a quick-view template.
type UpsertQuickViewRequest struct {
	Fields interface{} `json:"fields"`
}

// Upsert godoc
// @Summary      Upsert quick-view template
// @Tags         quickview
// @Accept       json
// @Produce      json
// @Param        category  path      string                  true  "Category name"
// @Param        body      body      UpsertQuickViewRequest  true  "Template fields (JSON array)"
// @Success      200       {object}  QuickViewTemplate
// @Router       /api/quickview/{category} [put]
func (h *QuickViewHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	category := chi.URLParam(r, "category")
	var body struct {
		Fields json.RawMessage `json:"fields"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}
	fields := string(body.Fields)
	if fields == "" {
		fields = "[]"
	}
	_, err := h.db.ExecContext(r.Context(),
		"INSERT INTO quick_view_templates (category, fields) VALUES (?, ?) ON CONFLICT(category) DO UPDATE SET fields = excluded.fields",
		category, fields,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, QuickViewTemplate{Category: category, Fields: fields})
}

// Delete godoc
// @Summary      Delete quick-view template
// @Tags         quickview
// @Produce      json
// @Param        category  path      string      true  "Category name"
// @Success      200       {object}  OkResponse
// @Router       /api/quickview/{category} [delete]
func (h *QuickViewHandler) Delete(w http.ResponseWriter, r *http.Request) {
	category := chi.URLParam(r, "category")
	if _, err := h.db.ExecContext(r.Context(), "DELETE FROM quick_view_templates WHERE category = ?", category); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
