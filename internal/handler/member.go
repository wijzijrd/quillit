package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/quillit/svc/internal/middleware"
)

type MemberHandler struct {
	db        *sql.DB
	jwtSecret []byte
}

func NewMember(db *sql.DB, jwtSecret string) *MemberHandler {
	return &MemberHandler{db: db, jwtSecret: []byte(jwtSecret)}
}

func (h *MemberHandler) callerID(r *http.Request) (string, bool) {
	mc, err := middleware.ClaimsFromContext(r.Context(), h.jwtSecret)
	if err != nil {
		return "", false
	}
	sub, _ := mc["sub"].(string)
	return sub, sub != ""
}

func (h *MemberHandler) ownsFolder(r *http.Request, folderID, userID string) bool {
	var count int
	_ = h.db.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM member_folders WHERE id = ? AND user_id = ?`,
		folderID, userID,
	).Scan(&count)
	return count > 0
}

// ── Response types ────────────────────────────────────────────────────────────

type MemberFolder struct {
	ID        string `json:"id"`
	UserID    string `json:"userId"`
	ProjectID string `json:"projectId,omitempty"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	SortOrder int    `json:"sortOrder"`
	CreatedAt int64  `json:"createdAt"`
}

type MemberEntryMeta struct {
	UserID    string `json:"userId"`
	EntryID   string `json:"entryId"`
	Pinned    bool   `json:"pinned"`
	SortOrder *int   `json:"sortOrder,omitempty"`
	Tags      string `json:"tags"`
}

// sharedEntrySelect mirrors entrySelect but joins entry_shares so scanEntry can be reused.
const sharedEntrySelect = `SELECT e.id, e.title, e.category, e.body, e.visibility, e.campaign_ids, e.linked_entries, e.tags, e.quick_view_data, e.created_at, e.updated_at, COALESCE(e.owner_user_id,'') FROM entries e JOIN entry_shares es ON es.entry_id = e.id`

// ── Shared notes ──────────────────────────────────────────────────────────────

// SharedNotes godoc
// @Summary      List notes shared with me
// @Description  Returns all entries shared with the authenticated user via entry_shares.
// @Tags         member
// @Produce      json
// @Success      200  {array}   Entry
// @Failure      401  {object}  ErrorResponse
// @Router       /api/member/shared [get]
func (h *MemberHandler) SharedNotes(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	rows, err := h.db.QueryContext(r.Context(),
		sharedEntrySelect+` WHERE es.user_id = ? ORDER BY es.shared_at DESC`,
		userID,
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

// ── Folders ───────────────────────────────────────────────────────────────────

// ListFolders godoc
// @Summary      List my folders
// @Tags         member
// @Produce      json
// @Success      200  {array}   MemberFolder
// @Router       /api/member/folders [get]
func (h *MemberHandler) ListFolders(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	rows, err := h.db.QueryContext(r.Context(),
		`SELECT id, user_id, COALESCE(project_id,''), name, color, sort_order, created_at FROM member_folders WHERE user_id = ? ORDER BY sort_order ASC, created_at ASC`,
		userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()

	folders := []MemberFolder{}
	for rows.Next() {
		var f MemberFolder
		if err := rows.Scan(&f.ID, &f.UserID, &f.ProjectID, &f.Name, &f.Color, &f.SortOrder, &f.CreatedAt); err == nil {
			folders = append(folders, f)
		}
	}
	writeJSON(w, http.StatusOK, folders)
}

// CreateFolder godoc
// @Summary      Create a folder
// @Tags         member
// @Accept       json
// @Produce      json
// @Param        body  body  object  true  "{ name, color?, projectId? }"
// @Success      201  {object}  MemberFolder
// @Failure      400  {object}  ErrorResponse
// @Router       /api/member/folders [post]
func (h *MemberHandler) CreateFolder(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var body struct {
		Name      string `json:"name"`
		Color     string `json:"color"`
		ProjectID string `json:"projectId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}

	f := MemberFolder{
		ID:        newID(),
		UserID:    userID,
		ProjectID: body.ProjectID,
		Name:      body.Name,
		Color:     body.Color,
		CreatedAt: nowUnix(),
	}

	var projectIDVal any
	if f.ProjectID != "" {
		projectIDVal = f.ProjectID
	}

	if _, err := h.db.ExecContext(r.Context(),
		`INSERT INTO member_folders (id, user_id, project_id, name, color, sort_order, created_at) VALUES (?,?,?,?,?,?,?)`,
		f.ID, f.UserID, projectIDVal, f.Name, f.Color, f.SortOrder, f.CreatedAt,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusCreated, f)
}

// UpdateFolder godoc
// @Summary      Update a folder
// @Tags         member
// @Accept       json
// @Produce      json
// @Param        id    path  string  true  "Folder ID"
// @Param        body  body  object  true  "{ name?, color?, sortOrder? }"
// @Success      200  {object}  MemberFolder
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /api/member/folders/{id} [patch]
func (h *MemberHandler) UpdateFolder(w http.ResponseWriter, r *http.Request) {
	folderID := chi.URLParam(r, "id")
	userID, ok := h.callerID(r)
	if !ok || !h.ownsFolder(r, folderID, userID) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}
	allowed := map[string]string{
		"name": "name", "color": "color", "sortOrder": "sort_order",
	}
	setClauses, args := buildSet(body, allowed)
	if len(setClauses) == 0 {
		writeError(w, http.StatusBadRequest, "no valid fields")
		return
	}
	args = append(args, folderID)
	if _, err := h.db.ExecContext(r.Context(), "UPDATE member_folders SET "+setClauses+" WHERE id = ?", args...); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	var f MemberFolder
	if err := h.db.QueryRowContext(r.Context(),
		`SELECT id, user_id, COALESCE(project_id,''), name, color, sort_order, created_at FROM member_folders WHERE id = ?`, folderID,
	).Scan(&f.ID, &f.UserID, &f.ProjectID, &f.Name, &f.Color, &f.SortOrder, &f.CreatedAt); err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, f)
}

// DeleteFolder godoc
// @Summary      Delete a folder
// @Tags         member
// @Produce      json
// @Param        id  path  string  true  "Folder ID"
// @Success      200  {object}  OkResponse
// @Failure      403  {object}  ErrorResponse
// @Router       /api/member/folders/{id} [delete]
func (h *MemberHandler) DeleteFolder(w http.ResponseWriter, r *http.Request) {
	folderID := chi.URLParam(r, "id")
	userID, ok := h.callerID(r)
	if !ok || !h.ownsFolder(r, folderID, userID) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if _, err := h.db.ExecContext(r.Context(), `DELETE FROM member_folders WHERE id = ?`, folderID); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// AddFolderEntry godoc
// @Summary      Add entry to folder
// @Tags         member
// @Accept       json
// @Produce      json
// @Param        id    path  string  true  "Folder ID"
// @Param        body  body  object  true  "{ entryId }"
// @Success      200  {object}  OkResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Router       /api/member/folders/{id}/entries [post]
func (h *MemberHandler) AddFolderEntry(w http.ResponseWriter, r *http.Request) {
	folderID := chi.URLParam(r, "id")
	userID, ok := h.callerID(r)
	if !ok || !h.ownsFolder(r, folderID, userID) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	var body struct {
		EntryID string `json:"entryId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.EntryID == "" {
		writeError(w, http.StatusBadRequest, "entryId required")
		return
	}

	if _, err := h.db.ExecContext(r.Context(),
		`INSERT OR IGNORE INTO member_folder_entries (folder_id, entry_id) VALUES (?,?)`,
		folderID, body.EntryID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// RemoveFolderEntry godoc
// @Summary      Remove entry from folder
// @Tags         member
// @Produce      json
// @Param        id       path  string  true  "Folder ID"
// @Param        entryId  path  string  true  "Entry ID"
// @Success      200  {object}  OkResponse
// @Failure      403  {object}  ErrorResponse
// @Router       /api/member/folders/{id}/entries/{entryId} [delete]
func (h *MemberHandler) RemoveFolderEntry(w http.ResponseWriter, r *http.Request) {
	folderID := chi.URLParam(r, "id")
	entryID := chi.URLParam(r, "entryId")
	userID, ok := h.callerID(r)
	if !ok || !h.ownsFolder(r, folderID, userID) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if _, err := h.db.ExecContext(r.Context(),
		`DELETE FROM member_folder_entries WHERE folder_id = ? AND entry_id = ?`,
		folderID, entryID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ── Entry meta ────────────────────────────────────────────────────────────────

// UpsertEntryMeta godoc
// @Summary      Upsert personal entry metadata
// @Description  Sets pinned status, sort order, and personal tags for an entry.
// @Tags         member
// @Accept       json
// @Produce      json
// @Param        id    path  string  true  "Entry ID"
// @Param        body  body  object  true  "{ pinned?, sortOrder?, tags? }"
// @Success      200  {object}  MemberEntryMeta
// @Failure      400  {object}  ErrorResponse
// @Router       /api/member/entries/{id}/meta [put]
func (h *MemberHandler) UpsertEntryMeta(w http.ResponseWriter, r *http.Request) {
	entryID := chi.URLParam(r, "id")
	userID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var body struct {
		Pinned    *bool   `json:"pinned"`
		SortOrder *int    `json:"sortOrder"`
		Tags      *string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}

	pinnedVal := 0
	if body.Pinned != nil && *body.Pinned {
		pinnedVal = 1
	}
	tags := "[]"
	if body.Tags != nil {
		tags = *body.Tags
	}

	if _, err := h.db.ExecContext(r.Context(),
		`INSERT INTO member_entry_meta (user_id, entry_id, pinned, sort_order, tags)
		 VALUES (?,?,?,?,?)
		 ON CONFLICT(user_id, entry_id) DO UPDATE SET
		   pinned = excluded.pinned,
		   sort_order = excluded.sort_order,
		   tags = excluded.tags`,
		userID, entryID, pinnedVal, body.SortOrder, tags,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	m := MemberEntryMeta{
		UserID:    userID,
		EntryID:   entryID,
		Pinned:    pinnedVal == 1,
		SortOrder: body.SortOrder,
		Tags:      tags,
	}
	writeJSON(w, http.StatusOK, m)
}

// ── Session notes ─────────────────────────────────────────────────────────────

// ListSessionNotes godoc
// @Summary      List my session notes
// @Description  Returns personal entries (owned by caller, not attached to any project/campaign).
// @Tags         member
// @Produce      json
// @Success      200  {array}   Entry
// @Router       /api/member/session-notes [get]
func (h *MemberHandler) ListSessionNotes(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	rows, err := h.db.QueryContext(r.Context(),
		entrySelect+` WHERE owner_user_id = ? AND campaign_ids = '[]' ORDER BY created_at DESC`,
		userID,
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

// CreateSessionNote godoc
// @Summary      Create a session note
// @Description  Creates a personal entry owned by the caller with no project/campaign association.
// @Tags         member
// @Accept       json
// @Produce      json
// @Param        body  body  object  true  "{ title?, category?, body? }"
// @Success      201  {object}  Entry
// @Failure      400  {object}  ErrorResponse
// @Router       /api/member/session-notes [post]
func (h *MemberHandler) CreateSessionNote(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var body struct {
		Title    string `json:"title"`
		Category string `json:"category"`
		Body     string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}
	defaults(&body.Title, "Untitled Note")
	defaults(&body.Category, "Note")

	now := nowUnix()
	e := Entry{
		ID:            newID(),
		Title:         body.Title,
		Category:      body.Category,
		Body:          body.Body,
		Visibility:    "private",
		CampaignIDs:   json.RawMessage("[]"),
		LinkedEntries: json.RawMessage("[]"),
		Tags:          json.RawMessage("[]"),
		QuickViewData: json.RawMessage("{}"),
		CreatedAt:     now,
		UpdatedAt:     now,
		OwnerUserID:   userID,
	}
	if _, err := h.db.ExecContext(r.Context(),
		`INSERT INTO entries (id,title,category,body,visibility,campaign_ids,linked_entries,tags,quick_view_data,created_at,updated_at,owner_user_id) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		e.ID, e.Title, e.Category, e.Body, e.Visibility, string(e.CampaignIDs), string(e.LinkedEntries), string(e.Tags), string(e.QuickViewData), e.CreatedAt, e.UpdatedAt, e.OwnerUserID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

// UpdateSessionNote godoc
// @Summary      Update a session note
// @Tags         member
// @Accept       json
// @Produce      json
// @Param        id    path  string  true  "Note ID"
// @Param        body  body  object  true  "Fields to update"
// @Success      200  {object}  Entry
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /api/member/session-notes/{id} [patch]
func (h *MemberHandler) UpdateSessionNote(w http.ResponseWriter, r *http.Request) {
	entryID := chi.URLParam(r, "id")
	userID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Verify ownership
	var count int
	_ = h.db.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM entries WHERE id = ? AND owner_user_id = ?`, entryID, userID,
	).Scan(&count)
	if count == 0 {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}
	body["updated_at"] = nowUnix()
	allowed := map[string]string{
		"title": "title", "category": "category", "body": "body", "updated_at": "updated_at",
	}
	setClauses, args := buildSet(body, allowed)
	if len(setClauses) == 0 {
		writeError(w, http.StatusBadRequest, "no valid fields")
		return
	}
	args = append(args, entryID)
	if _, err := h.db.ExecContext(r.Context(), "UPDATE entries SET "+setClauses+" WHERE id = ?", args...); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	e, err := scanEntry(h.db.QueryRowContext(r.Context(), entrySelect+" WHERE id = ?", entryID))
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, e)
}

// DeleteSessionNote godoc
// @Summary      Delete a session note
// @Tags         member
// @Produce      json
// @Param        id  path  string  true  "Note ID"
// @Success      200  {object}  OkResponse
// @Failure      403  {object}  ErrorResponse
// @Router       /api/member/session-notes/{id} [delete]
func (h *MemberHandler) DeleteSessionNote(w http.ResponseWriter, r *http.Request) {
	entryID := chi.URLParam(r, "id")
	userID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	res, err := h.db.ExecContext(r.Context(),
		`DELETE FROM entries WHERE id = ? AND owner_user_id = ?`, entryID, userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
