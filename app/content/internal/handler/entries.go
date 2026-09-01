package handler

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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
	"github.com/quillit/content-svc/internal/db/sqlc"
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
	q         *sqlc.Queries
	jwtSecret []byte
	blobs     BlobStore
	checker   authz.Checker
}

func NewEntries(db *sql.DB, jwtSecret string, blobs BlobStore, checker authz.Checker) *EntriesHandler {
	return &EntriesHandler{db: db, q: sqlc.New(db), jwtSecret: []byte(jwtSecret), blobs: blobs, checker: checker}
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
	// BodyHash is the SHA-256 hex hash of the entry's raw stored body —
	// empty when unset (pre-#126-migration rows that haven't been
	// touched since). See bodyHash() and #126's delta-push design.
	BodyHash string `json:"bodyHash,omitempty"`
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

// toEntryMeta converts one sqlc-generated entries row into EntryMeta,
// unmarshaling the stored tags JSON and translating the nullable
// orphaned_at column into EntryMeta's *int64 — the shared post-processing
// step every entries-row-shaped sqlc query (GetEntryMeta,
// ListEntriesForProject) needs, replacing the old scanEntryMeta.
func toEntryMeta(id, projectID, slug, directoryPath, title, tagsJSON, ownerUserID string, createdAt, updatedAt int64, bodyHash string, orphanedAt sql.NullInt64) EntryMeta {
	m := EntryMeta{
		ID: id, ProjectID: projectID, Slug: slug, DirectoryPath: directoryPath,
		Title: title, OwnerUserID: ownerUserID,
		CreatedAt: createdAt, UpdatedAt: updatedAt, BodyHash: bodyHash,
	}
	_ = json.Unmarshal([]byte(tagsJSON), &m.Tags)
	if orphanedAt.Valid {
		v := orphanedAt.Int64
		m.OrphanedAt = &v
	}
	return m
}

func bodyKey(id string) string {
	return fmt.Sprintf("entries/%s/body.md", id)
}

// bodyHash is the SHA-256 (hex) of an entry's raw body bytes, stored
// alongside every body write (Create, Update, import apply) and exposed
// on List for #126's delta-push follow-up. No normalization: hashing the
// exact bytes that get stored means a client hashing its own local file
// the same way gets a directly comparable value.
func bodyHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
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
	rows, err := h.q.ListEntriesForProject(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	entries := []EntryMeta{}
	for _, row := range rows {
		entries = append(entries, toEntryMeta(row.ID, row.ProjectID, row.Slug, row.DirectoryPath, row.Title, row.Tags, row.OwnerUserID, row.CreatedAt, row.UpdatedAt, row.BodyHash, row.OrphanedAt))
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
	projectID, err := projectIDForEntry(r.Context(), h.q, id)
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
	return fetchResolvedEntry(ctx, h.q, h.blobs, id)
}

// fetchResolvedEntry loads id's metadata plus its resolved body — the
// package-level form of EntriesHandler.fetchResolved, factored out so other
// handler types (assign.go's AssignHandler) that also need to return a full
// Entry after a mutation don't duplicate this lookup.
func fetchResolvedEntry(ctx context.Context, q *sqlc.Queries, blobs BlobStore, id string) (Entry, error) {
	row, err := q.GetEntryMeta(ctx, id)
	if err != nil {
		return Entry{}, err
	}
	m := toEntryMeta(row.ID, row.ProjectID, row.Slug, row.DirectoryPath, row.Title, row.Tags, row.OwnerUserID, row.CreatedAt, row.UpdatedAt, row.BodyHash, row.OrphanedAt)
	e := Entry{EntryMeta: m}
	if blobs != nil {
		if data, err := blobs.Get(ctx, bodyKey(id)); err == nil {
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
	hash := bodyHash([]byte(req.Body))

	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer tx.Rollback()
	qtx := h.q.WithTx(tx)

	err = qtx.InsertEntry(r.Context(), sqlc.InsertEntryParams{
		ID: id, ProjectID: projectID, Slug: req.Slug, DirectoryPath: req.DirectoryPath,
		Title: title, Tags: string(tagsJSON), OwnerUserID: nullString(ownerID),
		CreatedAt: now, UpdatedAt: now, BodyHash: nullString(hash),
	})
	if err != nil {
		_ = h.blobs.Delete(r.Context(), bodyKey(id))
		if isUniqueConstraintErr(err) {
			writeError(w, http.StatusConflict, "an entry already exists at this path")
			return
		}
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	if err := recompileLinks(r.Context(), qtx, id, projectID, parsed); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	var tags []string
	_ = json.Unmarshal(tagsJSON, &tags)

	if err := refreshSearchIndex(r.Context(), qtx, id, title, tags, parsed); err != nil {
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
	existingRow, err := h.q.GetEntryMeta(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	existing := toEntryMeta(existingRow.ID, existingRow.ProjectID, existingRow.Slug, existingRow.DirectoryPath, existingRow.Title, existingRow.Tags, existingRow.OwnerUserID, existingRow.CreatedAt, existingRow.UpdatedAt, existingRow.BodyHash, existingRow.OrphanedAt)
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
	hash := existing.BodyHash
	if bodyChanged {
		hash = bodyHash([]byte(newBody))
	}

	now := nowUnix()
	tagsJSON, _ := json.Marshal(tags)

	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer tx.Rollback()
	qtx := h.q.WithTx(tx)

	err = qtx.UpdateEntry(r.Context(), sqlc.UpdateEntryParams{
		Slug: slug, DirectoryPath: dir, Title: title, Tags: string(tagsJSON),
		UpdatedAt: now, BodyHash: sql.NullString{String: hash, Valid: true}, ID: id,
	})
	if err != nil {
		if isUniqueConstraintErr(err) {
			writeError(w, http.StatusConflict, "an entry already exists at this path")
			return
		}
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	if bodyChanged {
		if err := recompileLinks(r.Context(), qtx, id, existing.ProjectID, parsed); err != nil {
			writeError(w, http.StatusInternalServerError, "db error")
			return
		}
		if err := refreshSearchIndex(r.Context(), qtx, id, title, tags, parsed); err != nil {
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
	projectID, err := projectIDForEntry(r.Context(), h.q, id)
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

	if err := deleteSearchIndex(r.Context(), h.q, id); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	if err := h.q.DeleteEntry(r.Context(), id); err != nil {
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
	projectID, err := projectIDForEntry(r.Context(), h.q, id)
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

// GetImage godoc
// @Summary      Get an entry image
// @Tags         entries
// @Produce      image/*
// @Param        id        path  string  true  "Entry ID"
// @Param        filename  path  string  true  "Image filename"
// @Success      200
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /content/entries/{id}/images/{filename} [get]
func (h *EntriesHandler) GetImage(w http.ResponseWriter, r *http.Request) {
	if h.blobs == nil {
		writeError(w, http.StatusServiceUnavailable, "blob storage not configured")
		return
	}

	id := chi.URLParam(r, "id")
	filename := chi.URLParam(r, "filename")
	if filename == "" || filename == ".." || strings.Contains(filename, "/") {
		writeError(w, http.StatusBadRequest, "invalid filename")
		return
	}

	projectID, err := projectIDForEntry(r.Context(), h.q, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if _, ok := requireProjectMember(w, r, h.jwtSecret, h.checker, projectID); !ok {
		return
	}

	data, err := h.blobs.Get(r.Context(), imagesPrefix(id)+filename)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	contentType := mime.TypeByExtension(filepath.Ext(filename))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	// Defense-in-depth against stored XSS: an image type such as
	// image/svg+xml can embed <script>, and this endpoint's response is
	// served from the same origin as the web app's SPA (see
	// app/svc/internal/handler/content_images.go's proxy and
	// app/ui/nginx.conf). nosniff stops browsers from ignoring
	// Content-Type and sniffing the body into HTML; the sandboxed
	// default-src 'none' CSP stops any script that does execute (e.g. via
	// direct navigation to an SVG's image URL) from doing anything.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// validateFacetsOrError validates parsed's card blocks against projectID's
// effective facet vocabulary, writing a structured 422 (naming the bad facet
// and the vocabulary) and returning false if validation fails or the
// vocabulary lookup itself errors.
func (h *EntriesHandler) validateFacetsOrError(w http.ResponseWriter, ctx context.Context, projectID string, parsed *parse.Entry) bool {
	vocab, sorted, err := effectiveFacetVocabulary(ctx, h.q, projectID)
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
