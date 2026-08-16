package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/quillit/contentengine/parse"

	"github.com/quillit/content-svc/internal/authz"
)

// BlobStore is the subset of storage.MinioStore's methods entries.go needs.
// An interface here (rather than depending on the concrete storage.MinioStore
// type) lets handler tests substitute a fake in-memory store.
type BlobStore interface {
	Put(ctx context.Context, key, contentType string, data []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
	DeletePrefix(ctx context.Context, prefix string) error
}

type EntriesHandler struct {
	db        *sql.DB
	jwtSecret []byte
	blobs     BlobStore
	checker   authz.Checker
}

func NewEntries(db *sql.DB, jwtSecret string, blobs BlobStore, checker authz.Checker) *EntriesHandler {
	return &EntriesHandler{db: db, jwtSecret: []byte(jwtSecret), blobs: blobs, checker: checker}
}

// EntryMeta is an entry's metadata, without its body — what the list
// endpoint returns (docs/web-refactor-spec.md §6.3: bodies stay lazily
// hydrated, list omits body, detail resolves it).
type EntryMeta struct {
	ID            string   `json:"id"`
	ProjectID     string   `json:"projectId"`
	Slug          string   `json:"slug"`
	DirectoryPath string   `json:"directoryPath"`
	Title         string   `json:"title"`
	Tags          []string `json:"tags"`
	OwnerUserID   string   `json:"ownerUserId,omitempty"`
	CreatedAt     int64    `json:"createdAt"`
	UpdatedAt     int64    `json:"updatedAt"`
	// OrphanedAt is set once svc reports this entry's project was deleted
	// (#44's orphan-and-report policy — see InternalHandler.ProjectDeleted).
	// nil means the entry's project is still live.
	OrphanedAt *int64 `json:"orphanedAt,omitempty"`
}

// Entry is EntryMeta plus the resolved markdown body — what Get/Create/
// Update return.
type Entry struct {
	EntryMeta
	Body string `json:"body"`
}

const entryMetaSelect = `SELECT id, project_id, slug, directory_path, title, tags, COALESCE(owner_user_id,''), created_at, updated_at, orphaned_at FROM entries`

func scanEntryMeta(row interface{ Scan(...any) error }) (EntryMeta, error) {
	var m EntryMeta
	var tagsJSON string
	var orphanedAt sql.NullInt64
	if err := row.Scan(&m.ID, &m.ProjectID, &m.Slug, &m.DirectoryPath, &m.Title, &tagsJSON, &m.OwnerUserID, &m.CreatedAt, &m.UpdatedAt, &orphanedAt); err != nil {
		return m, err
	}
	_ = json.Unmarshal([]byte(tagsJSON), &m.Tags)
	if orphanedAt.Valid {
		m.OrphanedAt = &orphanedAt.Int64
	}
	return m, nil
}

func bodyKey(id string) string {
	return fmt.Sprintf("entries/%s/body.md", id)
}

func imagesPrefix(id string) string {
	return fmt.Sprintf("entries/%s/images/", id)
}

// List godoc
// @Summary      List entries in a project
// @Description  Metadata only — bodies are lazily hydrated via Get.
// @Tags         entries
// @Produce      json
// @Param        id   path      string  true  "Project ID"
// @Success      200  {array}   EntryMeta
// @Router       /content/projects/{id}/entries [get]
func (h *EntriesHandler) List(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "missing project id")
		return
	}
	if _, ok := requireProjectMember(w, r, h.jwtSecret, h.checker, projectID); !ok {
		return
	}
	rows, err := h.db.QueryContext(r.Context(), entryMetaSelect+" WHERE project_id = ? ORDER BY directory_path, slug", projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()

	entries := []EntryMeta{}
	for rows.Next() {
		m, err := scanEntryMeta(rows)
		if err != nil {
			continue
		}
		entries = append(entries, m)
	}
	writeJSON(w, http.StatusOK, entries)
}

// Get godoc
// @Summary      Get entry, including its resolved body
// @Tags         entries
// @Produce      json
// @Param        id   path      string  true  "Entry ID"
// @Success      200  {object}  Entry
// @Failure      404  {object}  ErrorResponse
// @Router       /content/entries/{id} [get]
func (h *EntriesHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	projectID, err := projectIDForEntry(r.Context(), h.db, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if _, ok := requireProjectMember(w, r, h.jwtSecret, h.checker, projectID); !ok {
		return
	}
	e, err := h.fetchResolved(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (h *EntriesHandler) fetchResolved(ctx context.Context, id string) (Entry, error) {
	m, err := scanEntryMeta(h.db.QueryRowContext(ctx, entryMetaSelect+" WHERE id = ?", id))
	if err != nil {
		return Entry{}, err
	}
	e := Entry{EntryMeta: m}
	if h.blobs != nil {
		if data, err := h.blobs.Get(ctx, bodyKey(id)); err == nil {
			e.Body = string(data)
		}
	}
	return e, nil
}

type createEntryRequest struct {
	Slug          string `json:"slug"`
	DirectoryPath string `json:"directoryPath"`
	Body          string `json:"body"`
}

// Create godoc
// @Summary      Create an entry
// @Description  Validates the body (directive syntax + facet vocabulary) and recompiles entry_links before returning.
// @Tags         entries
// @Accept       json
// @Produce      json
// @Param        id    path      string              true  "Project ID"
// @Success      201   {object}  Entry
// @Failure      400   {object}  ErrorResponse
// @Failure      422   {object}  ErrorResponse
// @Router       /content/projects/{id}/entries [post]
func (h *EntriesHandler) Create(w http.ResponseWriter, r *http.Request) {
	if h.blobs == nil {
		writeError(w, http.StatusServiceUnavailable, "blob storage not configured")
		return
	}
	projectID := chi.URLParam(r, "id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "missing project id")
		return
	}
	ownerID, ok := requireProjectMember(w, r, h.jwtSecret, h.checker, projectID)
	if !ok {
		return
	}

	var req createEntryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}
	if req.Slug == "" {
		writeError(w, http.StatusBadRequest, "slug is required")
		return
	}

	parsed, err := parse.Parse([]byte(req.Body))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !h.validateFacetsOrError(w, r.Context(), projectID, parsed) {
		return
	}

	id := newID()
	now := nowUnix()
	title := parsed.Frontmatter.Name
	if title == "" {
		title = req.Slug
	}
	tagsJSON, _ := json.Marshal(parsed.Frontmatter.Tags)

	if err := h.blobs.Put(r.Context(), bodyKey(id), "text/markdown; charset=utf-8", []byte(req.Body)); err != nil {
		writeError(w, http.StatusInternalServerError, "blob error")
		return
	}

	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(r.Context(), `
		INSERT INTO entries (id, project_id, slug, directory_path, title, tags, owner_user_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, projectID, req.Slug, req.DirectoryPath, title, string(tagsJSON), nullStr(ownerID), now, now)
	if err != nil {
		_ = h.blobs.Delete(r.Context(), bodyKey(id))
		if isUniqueConstraintErr(err) {
			writeError(w, http.StatusConflict, "an entry already exists at this path")
			return
		}
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	if err := recompileLinks(r.Context(), tx, id, projectID, parsed); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	var tags []string
	_ = json.Unmarshal(tagsJSON, &tags)

	if err := refreshSearchIndex(r.Context(), tx, id, title, tags, parsed); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusCreated, Entry{
		EntryMeta: EntryMeta{
			ID: id, ProjectID: projectID, Slug: req.Slug, DirectoryPath: req.DirectoryPath,
			Title: title, Tags: tags, OwnerUserID: ownerID, CreatedAt: now, UpdatedAt: now,
		},
		Body: req.Body,
	})
}

type updateEntryRequest struct {
	Slug          *string `json:"slug"`
	DirectoryPath *string `json:"directoryPath"`
	Body          *string `json:"body"`
}

// Update godoc
// @Summary      Update an entry
// @Description  Partial update. A body change re-validates and recompiles entry_links.
// @Tags         entries
// @Accept       json
// @Produce      json
// @Param        id    path      string  true  "Entry ID"
// @Success      200   {object}  Entry
// @Failure      404   {object}  ErrorResponse
// @Failure      422   {object}  ErrorResponse
// @Router       /content/entries/{id} [patch]
func (h *EntriesHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	existing, err := scanEntryMeta(h.db.QueryRowContext(r.Context(), entryMetaSelect+" WHERE id = ?", id))
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if _, ok := requireProjectMember(w, r, h.jwtSecret, h.checker, existing.ProjectID); !ok {
		return
	}

	var req updateEntryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}

	slug := existing.Slug
	if req.Slug != nil {
		slug = *req.Slug
	}
	dir := existing.DirectoryPath
	if req.DirectoryPath != nil {
		dir = *req.DirectoryPath
	}
	title, tags := existing.Title, existing.Tags

	var parsed *parse.Entry
	var newBody string
	bodyChanged := req.Body != nil
	if bodyChanged {
		newBody = *req.Body
		parsed, err = parse.Parse([]byte(newBody))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if !h.validateFacetsOrError(w, r.Context(), existing.ProjectID, parsed) {
			return
		}
		title = parsed.Frontmatter.Name
		if title == "" {
			title = slug
		}
		tags = parsed.Frontmatter.Tags
	}

	if bodyChanged && h.blobs == nil {
		writeError(w, http.StatusServiceUnavailable, "blob storage not configured")
		return
	}
	if bodyChanged {
		if err := h.blobs.Put(r.Context(), bodyKey(id), "text/markdown; charset=utf-8", []byte(newBody)); err != nil {
			writeError(w, http.StatusInternalServerError, "blob error")
			return
		}
	}

	now := nowUnix()
	tagsJSON, _ := json.Marshal(tags)

	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(r.Context(), `
		UPDATE entries SET slug = ?, directory_path = ?, title = ?, tags = ?, updated_at = ? WHERE id = ?
	`, slug, dir, title, string(tagsJSON), now, id)
	if err != nil {
		if isUniqueConstraintErr(err) {
			writeError(w, http.StatusConflict, "an entry already exists at this path")
			return
		}
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	if bodyChanged {
		if err := recompileLinks(r.Context(), tx, id, existing.ProjectID, parsed); err != nil {
			writeError(w, http.StatusInternalServerError, "db error")
			return
		}
		if err := refreshSearchIndex(r.Context(), tx, id, title, tags, parsed); err != nil {
			writeError(w, http.StatusInternalServerError, "db error")
			return
		}
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	e, err := h.fetchResolved(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, e)
}

// Delete godoc
// @Summary      Delete an entry
// @Description  Removes the metadata row (entry_links cascade) and the entry's MinIO body/image objects.
// @Tags         entries
// @Produce      json
// @Param        id   path      string  true  "Entry ID"
// @Success      200  {object}  OkResponse
// @Router       /content/entries/{id} [delete]
func (h *EntriesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	projectID, err := projectIDForEntry(r.Context(), h.db, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if _, ok := requireProjectMember(w, r, h.jwtSecret, h.checker, projectID); !ok {
		return
	}

	if h.blobs != nil {
		_ = h.blobs.Delete(r.Context(), bodyKey(id))
		_ = h.blobs.DeletePrefix(r.Context(), imagesPrefix(id))
	}

	if err := deleteSearchIndex(r.Context(), h.db, id); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	if _, err := h.db.ExecContext(r.Context(), "DELETE FROM entries WHERE id = ?", id); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// UploadImage godoc
// @Summary      Upload an image attachment for an entry
// @Tags         entries
// @Accept       multipart/form-data
// @Produce      json
// @Param        id     path      string  true  "Entry ID"
// @Param        image  formData  file    true  "Image file"
// @Success      200    {object}  map[string]string
// @Failure      400    {object}  ErrorResponse
// @Router       /content/entries/{id}/images [post]
func (h *EntriesHandler) UploadImage(w http.ResponseWriter, r *http.Request) {
	if h.blobs == nil {
		writeError(w, http.StatusServiceUnavailable, "blob storage not configured")
		return
	}

	id := chi.URLParam(r, "id")
	projectID, err := projectIDForEntry(r.Context(), h.db, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if _, ok := requireProjectMember(w, r, h.jwtSecret, h.checker, projectID); !ok {
		return
	}

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

	writeJSON(w, http.StatusOK, map[string]string{"key": key})
}

// validateFacetsOrError validates parsed's card blocks against projectID's
// effective facet vocabulary, writing a structured 422 (naming the bad facet
// and the vocabulary) and returning false if validation fails or the
// vocabulary lookup itself errors.
func (h *EntriesHandler) validateFacetsOrError(w http.ResponseWriter, ctx context.Context, projectID string, parsed *parse.Entry) bool {
	vocab, sorted, err := effectiveFacetVocabulary(ctx, h.db, projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return false
	}
	if err := validateFacets(parsed, vocab, sorted); err != nil {
		var ufe UnknownFacetError
		if errors.As(err, &ufe) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"error":      "unknown facet",
				"facet":      ufe.Facet,
				"vocabulary": ufe.Vocabulary,
			})
			return false
		}
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return false
	}
	return true
}

func isUniqueConstraintErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
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
