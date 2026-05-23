package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

type MigrateHandler struct{ db *sql.DB }

func NewMigrate(db *sql.DB) *MigrateHandler { return &MigrateHandler{db: db} }

// ImportPayload is the body for the bulk import endpoint.
type ImportPayload struct {
	Campaigns        []interface{} `json:"campaigns"`
	Players          []interface{} `json:"players"`
	Entries          []interface{} `json:"entries"`
	Annotations      []interface{} `json:"annotations"`
	PlayerNotes      []interface{} `json:"playerNotes"`
	QuickViewTemplates []interface{} `json:"quickViewTemplates"`
}

// Import godoc
// @Summary      Bulk import
// @Description  Clears all existing data and imports the provided dataset in a single transaction.
// @Tags         migrate
// @Accept       json
// @Produce      json
// @Param        body  body      ImportPayload  true  "Full data export"
// @Success      200   {object}  OkResponse
// @Failure      400   {object}  ErrorResponse
// @Router       /api/migrate/import [post]
func (h *MigrateHandler) Import(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Campaigns   []json.RawMessage `json:"campaigns"`
		Players     []json.RawMessage `json:"players"`
		Entries     []json.RawMessage `json:"entries"`
		Annotations []json.RawMessage `json:"annotations"`
		PlayerNotes []json.RawMessage `json:"playerNotes"`
		Templates   []json.RawMessage `json:"quickViewTemplates"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}

	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer tx.Rollback()

	tables := []string{"player_notes", "annotations", "entries", "players", "campaigns", "quick_view_templates"}
	for _, t := range tables {
		if _, err := tx.ExecContext(r.Context(), "DELETE FROM "+t); err != nil {
			writeError(w, http.StatusInternalServerError, "db error")
			return
		}
	}

	if err := importRows(r, tx, "campaigns", []string{"id", "name", "created_at"}, payload.Campaigns); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := importRows(r, tx, "players", []string{"id", "campaign_id", "name", "token", "created_at"}, payload.Players); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := importRows(r, tx, "entries", []string{"id", "title", "category", "body", "visibility", "campaign_ids", "linked_entries", "tags", "quick_view_data", "created_at", "updated_at"}, payload.Entries); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := importRows(r, tx, "annotations", []string{"id", "entry_id", "text", "visibility", "shared_with", "created_at", "updated_at"}, payload.Annotations); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func importRows(r *http.Request, tx *sql.Tx, table string, cols []string, rows []json.RawMessage) error {
	for _, raw := range rows {
		var row map[string]any
		if err := json.Unmarshal(raw, &row); err != nil {
			continue
		}
		vals := make([]any, len(cols))
		for i, col := range cols {
			// Try camelCase key first, then snake_case
			camel := snakeToCamel(col)
			if v, ok := row[camel]; ok {
				vals[i] = marshalIfNeeded(v)
			} else if v, ok := row[col]; ok {
				vals[i] = marshalIfNeeded(v)
			}
		}
		placeholders := "?"
		for i := 1; i < len(cols); i++ {
			placeholders += ",?"
		}
		q := "INSERT OR IGNORE INTO " + table + " (" + joinCols(cols) + ") VALUES (" + placeholders + ")"
		if _, err := tx.ExecContext(r.Context(), q, vals...); err != nil {
			return err
		}
	}
	return nil
}

func joinCols(cols []string) string {
	out := cols[0]
	for _, c := range cols[1:] {
		out += "," + c
	}
	return out
}

func marshalIfNeeded(v any) any {
	switch v.(type) {
	case string, float64, int64, bool, nil:
		return v
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func snakeToCamel(s string) string {
	out := ""
	upper := false
	for _, c := range s {
		if c == '_' {
			upper = true
			continue
		}
		if upper {
			if c >= 'a' && c <= 'z' {
				c -= 32
			}
			upper = false
		}
		out += string(c)
	}
	return out
}
