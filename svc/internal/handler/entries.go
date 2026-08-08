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

// NewEntriesForTest creates a handler that uses test context for caller ID
// (see WithTestCallerID) instead of parsing a real JWT.
func NewEntriesForTest(db *sql.DB) *EntriesHandler {
	return &EntriesHandler{db: db, jwtSecret: nil}
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
// @Description  Only returns entries whose campaign_ids intersects a project the caller is a member of.
// @Tags         entries
// @Produce      json
// @Success      200  {array}   Entry
// @Failure      401  {object}  ErrorResponse
// @Router       /api/entries [get]
func (h *EntriesHandler) List(w http.ResponseWriter, r *http.Request) {
	callerID, ok := callerIDFromRequest(r, h.jwtSecret)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	memberProjects, err := callerProjectIDs(r.Context(), h.db, callerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	rows, err := h.db.QueryContext(r.Context(), entrySelect+" ORDER BY created_at ASC")
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
		if !entryAccessible(e.CampaignIDs, memberProjects) {
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
// @Description  Only accessible if the entry's campaign_ids intersects a project the caller is a member of.
// @Tags         entries
// @Produce      json
// @Param        id   path      string         true  "Entry ID"
// @Success      200  {object}  Entry
// @Failure      401  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /api/entries/{id} [get]
func (h *EntriesHandler) Get(w http.ResponseWriter, r *http.Request) {
	callerID, ok := callerIDFromRequest(r, h.jwtSecret)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := chi.URLParam(r, "id")
	e, err := h.fetchResolved(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	memberProjects, err := callerProjectIDs(r.Context(), h.db, callerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	// Same "not found" response whether the entry genuinely doesn't exist
	// or the caller just can't see it — deliberately doesn't confirm
	// existence to a non-member, matching the pattern the rest of this
	// package uses for membership-gated resources (e.g. projects.go's
	// Update/AddMember/CreateInvite all respond 404 on a failed
	// membership check, not 403).
	if !entryAccessible(e.CampaignIDs, memberProjects) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, e)
}

// fetchResolved loads an entry by id with its body resolved from blob storage.
// Shared by the Get handler and the Game Mode chat share_card path.
func (h *EntriesHandler) fetchResolved(ctx context.Context, id string) (Entry, error) {
	e, bodyKey, err := scanEntryRaw(h.db.QueryRowContext(ctx, entrySelect+" WHERE id = ?", id))
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
	_, err := h.db.ExecContext(r.Context(), "UPDATE entries SET "+setClauses+" WHERE id = ?", args...)
	if err != nil {
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
