package migrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/quillit/contentengine/acceptance"
)

// BlobFetcher resolves a MinIO object key to its content, for entries whose
// body was offloaded to blob storage (entries.body_key set). Satisfied by
// *internal/storage.MinioStore in production; a test double in tests. A nil
// BlobFetcher is valid when no entry in the batch has a body_key.
type BlobFetcher interface {
	FetchBody(ctx context.Context, key string) (string, error)
}

// Report is the full dry-run output of one migration pass: every entry's
// conversion outcome, reviewable in one file before any cutover decision is
// made. Nothing referenced here is written back to the database or to blob
// storage — see issue #34's "writes nothing permanent" requirement.
type Report struct {
	GeneratedAt time.Time     `json:"generatedAt"`
	Entries     []EntryReport `json:"entries"`
}

// EntryReport is one entry's conversion outcome.
type EntryReport struct {
	EntryID string `json:"entryId"`
	Title   string `json:"title"`

	// Status is "clean" (parses, no flags), "flagged" (parses, but raised
	// flags for unconvertible fragments or dangling mentions), or "failed"
	// (rejected by the CLI parser/facet validator — a real migration
	// blocker, never silently downgraded to a warning).
	Status string `json:"status"`

	Slug          string `json:"slug"`
	DirectoryPath string `json:"directoryPath"`
	ProjectID     string `json:"projectId"`

	// MultiProjectIDs holds every campaign id when the legacy entry
	// belonged to more than one; ProjectID above is always the first
	// (spec §4.2's "pick primary" rule). Empty for single-project entries.
	MultiProjectIDs []string `json:"multiProjectIds,omitempty"`

	Flags      []string `json:"flags,omitempty"`
	ParseError string   `json:"parseError,omitempty"`

	// Markdown is the full converted body, included so a reviewer can read
	// the actual migration output directly from the report.
	Markdown string `json:"markdown"`
}

type legacyEntry struct {
	id, title, category, body, bodyKey string
	campaignIDs, tags, quickView       string
}

// Run performs one dry-run migration pass over every entry in database:
// backfills slug/directory_path/project_id (in memory only), converts each
// body to annotated markdown, folds in GM annotations and quick-view data,
// and validates every output through the CLI parser (spec §5's acceptance
// test). blobs resolves body_key-backed bodies; pass nil when none exist.
func Run(ctx context.Context, database *sql.DB, blobs BlobFetcher) (Report, error) {
	entries, err := loadEntries(database)
	if err != nil {
		return Report{}, fmt.Errorf("load entries: %w", err)
	}
	annotationsByEntry, err := loadGMAnnotations(database)
	if err != nil {
		return Report{}, fmt.Errorf("load annotations: %w", err)
	}
	vocabulary, err := loadFacetVocabulary(database)
	if err != nil {
		return Report{}, fmt.Errorf("load facet vocabulary: %w", err)
	}

	assignments, err := assignPaths(entries)
	if err != nil {
		return Report{}, err
	}

	resolveMention := func(id string) (string, bool) {
		a, ok := assignments[id]
		if !ok {
			return "", false
		}
		return a.path, true
	}

	reports := make(map[string]*EntryReport, len(entries))
	bodies := make(map[string][]byte, len(entries))
	order := make([]string, 0, len(entries))

	for _, e := range entries {
		a := assignments[e.id]
		r := &EntryReport{
			EntryID:         e.id,
			Title:           e.title,
			Slug:            a.slug,
			DirectoryPath:   a.directoryPath,
			ProjectID:       a.projectID,
			MultiProjectIDs: a.multiProjectIDs,
		}
		reports[e.id] = r
		order = append(order, e.id)

		bodyHTML, err := resolveBody(ctx, e, blobs)
		if err != nil {
			r.Status = "failed"
			r.ParseError = fmt.Sprintf("resolve body: %v", err)
			continue
		}

		tags, err := decodeStringArray(e.tags)
		if err != nil {
			r.Status = "failed"
			r.ParseError = fmt.Sprintf("decode tags: %v", err)
			continue
		}
		quickView, err := decodeStringMap(e.quickView)
		if err != nil {
			r.Status = "failed"
			r.ParseError = fmt.Sprintf("decode quick_view_data: %v", err)
			continue
		}

		input := EntryInput{
			ID:              e.id,
			Title:           e.title,
			Tags:            tags,
			BodyHTML:        bodyHTML,
			Facet:           a.directoryPath,
			QuickViewFields: quickView,
			GMAnnotations:   annotationsByEntry[e.id],
		}
		markdown, flags := ConvertEntry(input, resolveMention)
		r.Markdown = markdown
		r.Flags = flags
		bodies[a.path] = []byte(markdown)
	}

	for _, result := range acceptance.Validate(bodies, vocabulary) {
		id := assignmentPathToID(assignments, result.Name)
		r := reports[id]
		if r == nil {
			continue
		}
		if result.Err != nil {
			r.Status = "failed"
			r.ParseError = result.Err.Error()
		}
	}

	var out []EntryReport
	for _, id := range order {
		r := reports[id]
		if r.Status == "" {
			if len(r.Flags) > 0 {
				r.Status = "flagged"
			} else {
				r.Status = "clean"
			}
		}
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EntryID < out[j].EntryID })

	return Report{GeneratedAt: time.Now().UTC(), Entries: out}, nil
}

type pathAssignment struct {
	slug, directoryPath, projectID, path string
	multiProjectIDs                      []string
}

// assignPaths backfills slug/directory_path/project_id for every entry
// in memory (spec §4.2, §4.4), deterministically (entries processed in id
// order) so re-running Run against unchanged source data is idempotent.
func assignPaths(entries []legacyEntry) (map[string]pathAssignment, error) {
	sorted := make([]legacyEntry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].id < sorted[j].id })

	takenByScope := map[string]map[string]bool{}
	assignments := make(map[string]pathAssignment, len(entries))

	for _, e := range sorted {
		campaignIDs, err := decodeStringArray(e.campaignIDs)
		if err != nil {
			return nil, fmt.Errorf("entry %s: decode campaign_ids: %w", e.id, err)
		}
		projectID := ""
		var multi []string
		if len(campaignIDs) > 0 {
			projectID = campaignIDs[0]
		}
		if len(campaignIDs) > 1 {
			multi = campaignIDs
		}

		directoryPath := DirectoryPath(e.category)
		scope := projectID + "\x00" + directoryPath
		if takenByScope[scope] == nil {
			takenByScope[scope] = map[string]bool{}
		}
		slug := UniqueSlug(Slugify(e.title), takenByScope[scope])
		takenByScope[scope][slug] = true

		path := slug
		if directoryPath != "" {
			path = directoryPath + "/" + slug
		}

		assignments[e.id] = pathAssignment{
			slug:            slug,
			directoryPath:   directoryPath,
			projectID:       projectID,
			path:            path,
			multiProjectIDs: multi,
		}
	}
	return assignments, nil
}

func assignmentPathToID(assignments map[string]pathAssignment, path string) string {
	for id, a := range assignments {
		if a.path == path {
			return id
		}
	}
	return ""
}

func resolveBody(ctx context.Context, e legacyEntry, blobs BlobFetcher) (string, error) {
	if e.bodyKey == "" {
		return e.body, nil
	}
	if blobs == nil {
		return "", fmt.Errorf("entry has body_key %q but no blob store is configured", e.bodyKey)
	}
	return blobs.FetchBody(ctx, e.bodyKey)
}

func loadEntries(database *sql.DB) ([]legacyEntry, error) {
	rows, err := database.Query(`
		SELECT id, title, category, body, COALESCE(body_key,''), campaign_ids, tags, quick_view_data
		FROM entries
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []legacyEntry
	for rows.Next() {
		var e legacyEntry
		if err := rows.Scan(&e.id, &e.title, &e.category, &e.body, &e.bodyKey, &e.campaignIDs, &e.tags, &e.quickView); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func loadGMAnnotations(database *sql.DB) (map[string][]Annotation, error) {
	rows, err := database.Query(`
		SELECT id, entry_id, text FROM annotations WHERE visibility = 'gm' ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byEntry := map[string][]Annotation{}
	for rows.Next() {
		var a Annotation
		var entryID string
		if err := rows.Scan(&a.ID, &entryID, &a.Text); err != nil {
			return nil, err
		}
		byEntry[entryID] = append(byEntry[entryID], a)
	}
	return byEntry, rows.Err()
}

// loadFacetVocabulary reads the global facet vocabulary seeded by schema v7
// (issue #33). Project-specific facets (project_facets) aren't consulted
// here: at this point in the migration no entry has a project_id backfilled
// into the live schema yet (this pass only computes it in memory), so
// there's no project scope to look them up against.
func loadFacetVocabulary(database *sql.DB) ([]string, error) {
	rows, err := database.Query(`SELECT name FROM facets ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

func decodeStringArray(raw string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func decodeStringMap(raw string) (map[string]string, error) {
	if raw == "" {
		return nil, nil
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}
