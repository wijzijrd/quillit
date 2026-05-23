package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type EntriesHandler struct{ db *sql.DB }

func NewEntries(db *sql.DB) *EntriesHandler { return &EntriesHandler{db: db} }

type Entry struct {
	ID            string          `json:"id"`
	Title         string          `json:"title"`
	Category      string          `json:"category"`
	Body          string          `json:"body"`
	Visibility    string          `json:"visibility"`
	CampaignIDs   json.RawMessage `json:"campaignIds"`
	LinkedEntries json.RawMessage `json:"linkedEntries"`
	Tags          json.RawMessage `json:"tags"`
	QuickViewData json.RawMessage `json:"quickViewData"`
	CreatedAt     int64           `json:"createdAt"`
	UpdatedAt     int64           `json:"updatedAt"`
}

const entrySelect = `SELECT id, title, category, body, visibility, campaign_ids, linked_entries, tags, quick_view_data, created_at, updated_at FROM entries`

func scanEntry(row interface{ Scan(...any) error }) (Entry, error) {
	var e Entry
	var campaignIDs, linkedEntries, tags, quickViewData string
	err := row.Scan(
		&e.ID, &e.Title, &e.Category, &e.Body, &e.Visibility,
		&campaignIDs, &linkedEntries, &tags, &quickViewData,
		&e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		return e, err
	}
	e.CampaignIDs = json.RawMessage(campaignIDs)
	e.LinkedEntries = json.RawMessage(linkedEntries)
	e.Tags = json.RawMessage(tags)
	e.QuickViewData = json.RawMessage(quickViewData)
	return e, nil
}

// List godoc
// @Summary      List entries
// @Tags         entries
// @Produce      json
// @Success      200  {array}   Entry
// @Router       /api/entries [get]
func (h *EntriesHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.QueryContext(r.Context(), entrySelect+" ORDER BY created_at ASC")
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

// Get godoc
// @Summary      Get entry
// @Tags         entries
// @Produce      json
// @Param        id   path      string         true  "Entry ID"
// @Success      200  {object}  Entry
// @Failure      404  {object}  ErrorResponse
// @Router       /api/entries/{id} [get]
func (h *EntriesHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	e, err := scanEntry(h.db.QueryRowContext(r.Context(), entrySelect+" WHERE id = ?", id))
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, e)
}

// Create godoc
// @Summary      Create entry
// @Tags         entries
// @Accept       json
// @Produce      json
// @Success      201   {object}  Entry
// @Failure      400   {object}  ErrorResponse
// @Router       /api/entries [post]
func (h *EntriesHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title         string          `json:"title"`
		Category      string          `json:"category"`
		Body          string          `json:"body"`
		Visibility    string          `json:"visibility"`
		CampaignIDs   json.RawMessage `json:"campaignIds"`
		LinkedEntries json.RawMessage `json:"linkedEntries"`
		Tags          json.RawMessage `json:"tags"`
		QuickViewData json.RawMessage `json:"quickViewData"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}
	defaults(&body.Title, "Untitled Entry")
	defaults(&body.Category, "Lore")
	defaults(&body.Visibility, "private")
	if len(body.CampaignIDs) == 0   { body.CampaignIDs   = json.RawMessage("[]") }
	if len(body.LinkedEntries) == 0 { body.LinkedEntries = json.RawMessage("[]") }
	if len(body.Tags) == 0          { body.Tags          = json.RawMessage("[]") }
	if len(body.QuickViewData) == 0 { body.QuickViewData = json.RawMessage("{}") }

	now := nowUnix()
	e := Entry{
		ID: newID(), Title: body.Title, Category: body.Category, Body: body.Body,
		Visibility: body.Visibility, CampaignIDs: body.CampaignIDs,
		LinkedEntries: body.LinkedEntries, Tags: body.Tags, QuickViewData: body.QuickViewData,
		CreatedAt: now, UpdatedAt: now,
	}
	_, err := h.db.ExecContext(r.Context(),
		`INSERT INTO entries (id,title,category,body,visibility,campaign_ids,linked_entries,tags,quick_view_data,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		e.ID, e.Title, e.Category, e.Body, e.Visibility, e.CampaignIDs, e.LinkedEntries, e.Tags, e.QuickViewData, e.CreatedAt, e.UpdatedAt,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

// Update godoc
// @Summary      Update entry
// @Description  Partial update — send only the fields to change.
// @Tags         entries
// @Accept       json
// @Produce      json
// @Param        id    path      string              true  "Entry ID"
// @Param        body  body      CreateEntryRequest  true  "Fields to update"
// @Success      200   {object}  Entry
// @Failure      404   {object}  ErrorResponse
// @Router       /api/entries/{id} [patch]
func (h *EntriesHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}
	body["updated_at"] = nowUnix()

	allowed := map[string]string{
		"title": "title", "category": "category", "body": "body",
		"visibility": "visibility", "campaignIds": "campaign_ids",
		"linkedEntries": "linked_entries", "tags": "tags",
		"quickViewData": "quick_view_data", "updated_at": "updated_at",
	}
	setClauses, args := buildSet(body, allowed)
	if len(setClauses) == 0 {
		writeError(w, http.StatusBadRequest, "no valid fields")
		return
	}
	args = append(args, id)
	_, err := h.db.ExecContext(r.Context(), "UPDATE entries SET "+setClauses+" WHERE id = ?", args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	e, err := scanEntry(h.db.QueryRowContext(r.Context(), entrySelect+" WHERE id = ?", id))
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, e)
}

// Delete godoc
// @Summary      Delete entry
// @Tags         entries
// @Produce      json
// @Param        id   path      string      true  "Entry ID"
// @Success      200  {object}  OkResponse
// @Router       /api/entries/{id} [delete]
func (h *EntriesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := h.db.ExecContext(r.Context(), "DELETE FROM entries WHERE id = ?", id); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func defaults(s *string, val string) {
	if *s == "" {
		*s = val
	}
}

func buildSet(body map[string]any, allowed map[string]string) (string, []any) {
	var clauses string
	var args []any
	for k, col := range allowed {
		if v, ok := body[k]; ok {
			if clauses != "" {
				clauses += ", "
			}
			clauses += col + " = ?"
			// JSON fields: if value is not a string, marshal it
			switch v := v.(type) {
			case string:
				args = append(args, v)
			default:
				b, _ := json.Marshal(v)
				args = append(args, string(b))
			}
		}
	}
	return clauses, args
}
