package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type CategoryDefaultTag struct {
	ID         string `json:"id"`
	CategoryID string `json:"categoryId"`
	Label      string `json:"label"`
	SortOrder  int    `json:"sortOrder"`
}

type Category struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Icon        string               `json:"icon"`
	Color       string               `json:"color"`
	SortOrder   int                  `json:"sortOrder"`
	DefaultTags []CategoryDefaultTag `json:"defaultTags"`
	CreatedAt   int64                `json:"createdAt"`
	UpdatedAt   int64                `json:"updatedAt"`
}

type CategoriesHandler struct {
	db *sql.DB
}

func NewCategories(db *sql.DB) *CategoriesHandler {
	return &CategoriesHandler{db: db}
}

func (h *CategoriesHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.QueryContext(r.Context(),
		`SELECT id, name, icon, color, sort_order, created_at, updated_at FROM categories ORDER BY sort_order`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	cats := []Category{}
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.Name, &c.Icon, &c.Color, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		cats = append(cats, c)
	}

	tagRows, err := h.db.QueryContext(r.Context(),
		`SELECT id, category_id, label, sort_order FROM category_default_tags ORDER BY category_id, sort_order`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "tag query failed")
		return
	}
	defer tagRows.Close()
	tagMap := map[string][]CategoryDefaultTag{}
	for tagRows.Next() {
		var t CategoryDefaultTag
		if err := tagRows.Scan(&t.ID, &t.CategoryID, &t.Label, &t.SortOrder); err != nil {
			writeError(w, http.StatusInternalServerError, "tag scan failed")
			return
		}
		tagMap[t.CategoryID] = append(tagMap[t.CategoryID], t)
	}
	for i := range cats {
		if tags, ok := tagMap[cats[i].ID]; ok {
			cats[i].DefaultTags = tags
		} else {
			cats[i].DefaultTags = []CategoryDefaultTag{}
		}
	}
	writeJSON(w, http.StatusOK, cats)
}

func (h *CategoriesHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name      string `json:"name"`
		Icon      string `json:"icon"`
		Color     string `json:"color"`
		SortOrder int    `json:"sortOrder"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	now := nowUnix()
	id := newID()
	if _, err := h.db.ExecContext(r.Context(),
		`INSERT INTO categories (id, name, icon, color, sort_order, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, req.Name, req.Icon, req.Color, req.SortOrder, now, now,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "insert failed")
		return
	}
	writeJSON(w, http.StatusCreated, Category{
		ID: id, Name: req.Name, Icon: req.Icon, Color: req.Color,
		SortOrder: req.SortOrder, DefaultTags: []CategoryDefaultTag{},
		CreatedAt: now, UpdatedAt: now,
	})
}

func (h *CategoriesHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Name      *string `json:"name"`
		Icon      *string `json:"icon"`
		Color     *string `json:"color"`
		SortOrder *int    `json:"sortOrder"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}
	now := nowUnix()
	if _, err := h.db.ExecContext(r.Context(), `
		UPDATE categories
		SET name       = COALESCE(?, name),
		    icon       = COALESCE(?, icon),
		    color      = COALESCE(?, color),
		    sort_order = COALESCE(?, sort_order),
		    updated_at = ?
		WHERE id = ?`,
		req.Name, req.Icon, req.Color, req.SortOrder, now, id,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	var c Category
	if err := h.db.QueryRowContext(r.Context(),
		`SELECT id, name, icon, color, sort_order, created_at, updated_at FROM categories WHERE id = ?`, id,
	).Scan(&c.ID, &c.Name, &c.Icon, &c.Color, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt); err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	c.DefaultTags = h.defaultTags(r, id)
	writeJSON(w, http.StatusOK, c)
}

func (h *CategoriesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := h.db.ExecContext(r.Context(), `DELETE FROM categories WHERE id = ?`, id); err != nil {
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *CategoriesHandler) AddTag(w http.ResponseWriter, r *http.Request) {
	catID := chi.URLParam(r, "id")
	var req struct {
		Label     string `json:"label"`
		SortOrder int    `json:"sortOrder"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Label == "" {
		writeError(w, http.StatusBadRequest, "label required")
		return
	}
	id := newID()
	if _, err := h.db.ExecContext(r.Context(),
		`INSERT INTO category_default_tags (id, category_id, label, sort_order) VALUES (?, ?, ?, ?)`,
		id, catID, req.Label, req.SortOrder,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "insert failed")
		return
	}
	writeJSON(w, http.StatusCreated, CategoryDefaultTag{
		ID: id, CategoryID: catID, Label: req.Label, SortOrder: req.SortOrder,
	})
}

func (h *CategoriesHandler) RemoveTag(w http.ResponseWriter, r *http.Request) {
	tagID := chi.URLParam(r, "tagId")
	if _, err := h.db.ExecContext(r.Context(),
		`DELETE FROM category_default_tags WHERE id = ?`, tagID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *CategoriesHandler) defaultTags(r *http.Request, catID string) []CategoryDefaultTag {
	rows, err := h.db.QueryContext(r.Context(),
		`SELECT id, category_id, label, sort_order FROM category_default_tags WHERE category_id = ? ORDER BY sort_order`, catID)
	if err != nil {
		return []CategoryDefaultTag{}
	}
	defer rows.Close()
	tags := []CategoryDefaultTag{}
	for rows.Next() {
		var t CategoryDefaultTag
		_ = rows.Scan(&t.ID, &t.CategoryID, &t.Label, &t.SortOrder)
		tags = append(tags, t)
	}
	return tags
}
