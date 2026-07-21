package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/quillit/svc/internal/middleware"
	"github.com/quillit/svc/internal/storage"
)

type BlobStore interface {
	Put(ctx context.Context, key, contentType string, data []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
}

type EntriesHandler struct {
	db        *sql.DB
	jwtSecret []byte
	blobs     BlobStore // nil when MinIO is not configured
}

func NewEntries(db *sql.DB, jwtSecret string) *EntriesHandler {
	return &EntriesHandler{db: db, jwtSecret: []byte(jwtSecret)}
}

func NewEntriesWithBlobs(db *sql.DB, jwtSecret string, blobs *storage.MinioStore) *EntriesHandler {
	return &EntriesHandler{db: db, jwtSecret: []byte(jwtSecret), blobs: blobs}
}

// NewEntriesForTest creates a handler that resolves the caller ID from the test
// context (WithTestCallerID) rather than a real JWT.
func NewEntriesForTest(db *sql.DB) *EntriesHandler {
	return &EntriesHandler{db: db, jwtSecret: nil}
}

// callerID extracts the user ID (sub) from the JWT stored in request context.
// In tests, a caller ID can be injected directly via WithTestCallerID.
func (h *EntriesHandler) callerID(r *http.Request) (string, bool) {
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
	OwnerUserID   string          `json:"ownerUserId,omitempty"`
}

const entrySelect = `SELECT id, title, category, body, COALESCE(body_key,''), visibility, campaign_ids, linked_entries, tags, quick_view_data, created_at, updated_at, COALESCE(owner_user_id,'') FROM entries`

// entryReadablePredicate is the authorization filter for reading an entry: the
// caller must own it, hold an explicit entry_shares grant, or be a member of a
// project the entry is filed under (campaign_ids is the live entry↔project link).
//
// It is a parenthesized boolean expression, NOT a full WHERE clause, so callers
// can safely append ` AND id = ?`. SQLite's AND binds tighter than OR, so without
// the outer parens `... OR C AND id = ?` would parse as `A OR B OR (C AND id=?)`
// and leak other rows the caller owns/shares. The three `?` bind the caller id.
//
// visibility = 'public' is deliberately NOT part of this predicate: public entries
// are reachable only through the token-scoped share.go flow, never the generic
// authenticated /api/entries endpoints.
const entryReadablePredicate = `(owner_user_id = ?
	OR EXISTS (SELECT 1 FROM entry_shares es WHERE es.entry_id = entries.id AND es.user_id = ?)
	OR EXISTS (
		SELECT 1 FROM json_each(entries.campaign_ids) je
		JOIN project_members pm ON pm.project_id = je.value
		WHERE pm.user_id = ?
	))`

// scanEntry is used by handlers that don't need the body_key (member, share).
func scanEntry(row interface{ Scan(...any) error }) (Entry, error) {
	e, _, err := scanEntryRaw(row)
	return e, err
}

func scanEntryRaw(row interface{ Scan(...any) error }) (Entry, string, error) {
	var e Entry
	var campaignIDs, linkedEntries, tags, quickViewData, bodyKey string
	err := row.Scan(
		&e.ID, &e.Title, &e.Category, &e.Body, &bodyKey, &e.Visibility,
		&campaignIDs, &linkedEntries, &tags, &quickViewData,
		&e.CreatedAt, &e.UpdatedAt, &e.OwnerUserID,
	)
	if err != nil {
		return e, "", err
	}
	e.CampaignIDs = json.RawMessage(campaignIDs)
	e.LinkedEntries = json.RawMessage(linkedEntries)
	e.Tags = json.RawMessage(tags)
	e.QuickViewData = json.RawMessage(quickViewData)
	return e, bodyKey, nil
}

func (h *EntriesHandler) resolveBody(ctx context.Context, e *Entry, bodyKey string) {
	if bodyKey == "" || h.blobs == nil {
		return
	}
	data, err := h.blobs.Get(ctx, bodyKey)
	if err == nil {
		e.Body = string(data)
	}
}

// List godoc
// @Summary      List entries
// @Tags         entries
// @Produce      json
// @Success      200  {array}   Entry
// @Router       /api/entries [get]
func (h *EntriesHandler) List(w http.ResponseWriter, r *http.Request) {
	callerID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	rows, err := h.db.QueryContext(r.Context(),
		entrySelect+" WHERE "+entryReadablePredicate+" ORDER BY created_at ASC",
		callerID, callerID, callerID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()
	entries := []Entry{}
	for rows.Next() {
		e, bodyKey, err := scanEntryRaw(rows)
		if err != nil {
			continue
		}
		// Body is fetched from MinIO only for individual Get — list returns inline body or empty.
		if bodyKey != "" {
			e.Body = ""
		}
		entries = append(entries, e)
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
	callerID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	e, err := h.fetchResolved(r.Context(), id, callerID)
	if err != nil {
		// 404 for both "doesn't exist" and "exists but caller can't see it" so we
		// don't leak the existence of entries to callers with no access to them.
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, e)
}

// fetchResolved loads an entry by id with its body resolved from blob storage,
// scoped to what callerID is authorized to read (see entryReadablePredicate).
// Shared by the Get handler and the Game Mode chat share_card path.
func (h *EntriesHandler) fetchResolved(ctx context.Context, id, callerID string) (Entry, error) {
	e, bodyKey, err := scanEntryRaw(h.db.QueryRowContext(ctx,
		entrySelect+" WHERE "+entryReadablePredicate+" AND id = ?",
		callerID, callerID, callerID, id,
	))
	if err != nil {
		return Entry{}, err
	}
	h.resolveBody(ctx, &e, bodyKey)
	return e, nil
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

	ownerID := ""
	if mc, err := middleware.ClaimsFromContext(r.Context(), h.jwtSecret); err == nil {
		ownerID, _ = mc["sub"].(string)
	}

	now := nowUnix()
	e := Entry{
		ID: newID(), Title: body.Title, Category: body.Category, Body: body.Body,
		Visibility: body.Visibility, CampaignIDs: body.CampaignIDs,
		LinkedEntries: body.LinkedEntries, Tags: body.Tags, QuickViewData: body.QuickViewData,
		CreatedAt: now, UpdatedAt: now, OwnerUserID: ownerID,
	}

	bodyKey := ""
	inlineBody := e.Body
	if h.blobs != nil && e.Body != "" {
		bodyKey = fmt.Sprintf("entries/%s/body.html", e.ID)
		if err := h.blobs.Put(r.Context(), bodyKey, "text/html; charset=utf-8", []byte(e.Body)); err != nil {
			writeError(w, http.StatusInternalServerError, "blob error")
			return
		}
		inlineBody = "" // don't duplicate body in SQLite
	}

	_, err := h.db.ExecContext(r.Context(),
		`INSERT INTO entries (id,title,category,body,body_key,visibility,campaign_ids,linked_entries,tags,quick_view_data,created_at,updated_at,owner_user_id) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		e.ID, e.Title, e.Category, inlineBody, nullStr(bodyKey), e.Visibility, e.CampaignIDs, e.LinkedEntries, e.Tags, e.QuickViewData, e.CreatedAt, e.UpdatedAt, e.OwnerUserID,
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
// @Param        body  body      Entry  true  "Fields to update (partial Entry)"
// @Success      200   {object}  Entry
// @Failure      404   {object}  ErrorResponse
// @Router       /api/entries/{id} [patch]
func (h *EntriesHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	callerID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	// Writes are owner-only: read access via sharing or project membership must
	// NOT imply write access.
	owner, err := isEntryOwner(h.db, r, id, callerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if !owner {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	var patch map[string]any
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}
	patch["updated_at"] = nowUnix()

	// If body is being updated and MinIO is configured, offload it to blob storage.
	if h.blobs != nil {
		if bodyVal, ok := patch["body"]; ok {
			if bodyStr, ok := bodyVal.(string); ok && bodyStr != "" {
				bodyKey := fmt.Sprintf("entries/%s/body.html", id)
				if err := h.blobs.Put(r.Context(), bodyKey, "text/html; charset=utf-8", []byte(bodyStr)); err != nil {
					writeError(w, http.StatusInternalServerError, "blob error")
					return
				}
				patch["body"] = ""
				patch["body_key"] = bodyKey
			}
		}
	}

	allowed := map[string]string{
		"title": "title", "category": "category", "body": "body",
		"body_key": "body_key",
		"visibility": "visibility", "campaignIds": "campaign_ids",
		"linkedEntries": "linked_entries", "tags": "tags",
		"quickViewData": "quick_view_data", "updated_at": "updated_at",
	}
	setClauses, args := buildSet(patch, allowed)
	if len(setClauses) == 0 {
		writeError(w, http.StatusBadRequest, "no valid fields")
		return
	}
	args = append(args, id)
	if _, err := h.db.ExecContext(r.Context(), "UPDATE entries SET "+setClauses+" WHERE id = ?", args...); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	e, bodyKey, err := scanEntryRaw(h.db.QueryRowContext(r.Context(), entrySelect+" WHERE id = ?", id))
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	h.resolveBody(r.Context(), &e, bodyKey)
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
	callerID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	// Writes are owner-only: read access via sharing or project membership must
	// NOT imply write access.
	owner, err := isEntryOwner(h.db, r, id, callerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if !owner {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	// Clean up blob storage objects for this entry before deleting the row.
	if h.blobs != nil {
		var bodyKey sql.NullString
		_ = h.db.QueryRowContext(r.Context(), "SELECT body_key FROM entries WHERE id = ?", id).Scan(&bodyKey)
		if bodyKey.Valid && bodyKey.String != "" {
			_ = h.blobs.Delete(r.Context(), bodyKey.String)
		}
	}

	if _, err := h.db.ExecContext(r.Context(), "DELETE FROM entries WHERE id = ?", id); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// UploadImage godoc
// @Summary      Upload image attachment for an entry
// @Tags         entries
// @Accept       multipart/form-data
// @Produce      json
// @Param        id     path      string  true  "Entry ID"
// @Param        image  formData  file    true  "Image file"
// @Success      200    {object}  map[string]string
// @Failure      400    {object}  ErrorResponse
// @Router       /api/entries/{id}/images [post]
func (h *EntriesHandler) UploadImage(w http.ResponseWriter, r *http.Request) {
	if h.blobs == nil {
		writeError(w, http.StatusServiceUnavailable, "blob storage not configured")
		return
	}

	id := chi.URLParam(r, "id")
	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10 MB limit
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing image field")
		return
	}
	defer file.Close()

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = mime.TypeByExtension(filepath.Ext(header.Filename))
	}
	if !isAllowedImageType(contentType) {
		writeError(w, http.StatusBadRequest, "unsupported image type")
		return
	}

	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read error")
		return
	}

	ext := filepath.Ext(header.Filename)
	if ext == "" {
		ext = extensionForMIME(contentType)
	}
	imgID := newID()
	key := fmt.Sprintf("entries/%s/images/%s%s", id, imgID, ext)

	if err := h.blobs.Put(r.Context(), key, contentType, data); err != nil {
		writeError(w, http.StatusInternalServerError, "blob error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"url": blobURL(key)})
}

func isAllowedImageType(ct string) bool {
	ct = strings.Split(ct, ";")[0]
	switch ct {
	case "image/jpeg", "image/png", "image/webp", "image/gif", "image/svg+xml":
		return true
	}
	return false
}

func extensionForMIME(ct string) string {
	switch strings.Split(ct, ";")[0] {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	}
	return ""
}

func blobURL(key string) string {
	endpoint := os.Getenv("MINIO_ENDPOINT")
	if endpoint == "" {
		endpoint = "localhost:9000"
	}
	bucket := os.Getenv("MINIO_BUCKET")
	if bucket == "" {
		bucket = "quillit-entries"
	}
	scheme := "http"
	if os.Getenv("MINIO_USE_SSL") == "true" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/%s/%s", scheme, endpoint, bucket, key)
}

func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
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
