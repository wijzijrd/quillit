package migrate

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// ErrAlreadyExists means content already has an entry at the requested
// (project, directory, slug) path — a 409 from POST .../entries. Cutover
// treats this as a successful (idempotent) outcome, not a failure: it's
// what a safe re-run after a partial failure looks like.
var ErrAlreadyExists = errors.New("entry already exists in content at this path")

// ContentImporter creates one entry in the already-deployed quillit/content
// service. Satisfied by *ContentClient in production; a test double in
// Cutover's own tests.
type ContentImporter interface {
	CreateEntry(ctx context.Context, projectID, slug, directoryPath, body string) error
}

// ContentClient calls content's existing entry-creation endpoint
// (docs/web-refactor-spec.md §6.4/§7.2, built by issue #37) rather than
// writing to content's SQLite file directly: svc and content are separate
// Go modules with separate databases, and content's endpoint already does
// everything a migrated entry needs — parse+facet-validate the body,
// write entries/{id}/body.md to MinIO, insert the metadata row, and
// recompile entry_links. Duplicating any of that here would be redundant
// at best and inconsistent at worst.
type ContentClient struct {
	BaseURL string
	HTTP    *http.Client
}

func (c *ContentClient) CreateEntry(ctx context.Context, projectID, slug, directoryPath, body string) error {
	payload, err := json.Marshal(struct {
		Slug          string `json:"slug"`
		DirectoryPath string `json:"directoryPath"`
		Body          string `json:"body"`
	}{Slug: slug, DirectoryPath: directoryPath, Body: body})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/content/projects/%s/entries", c.BaseURL, projectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", url, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusCreated:
		return nil
	case http.StatusConflict:
		return ErrAlreadyExists
	default:
		return fmt.Errorf("POST %s: unexpected status %d", url, resp.StatusCode)
	}
}

// CutoverResult is one entry's outcome from Cutover.
type CutoverResult struct {
	EntryID string
	Path    string // directoryPath + "/" + slug, or just slug at project root
	Status  string // "imported" | "skipped-failed" | "error"
	Err     string `json:",omitempty"`
}

// Cutover is the write phase of the content migration (issue #35): it reruns
// Run's existing dry-run pipeline unchanged (same conversion, same
// path-assignment, same parser validation — issue #34's pipeline is not
// touched by #35) and, for every entry Run did not mark "failed", creates
// it in content via importer. Entries marked "failed" are never imported —
// a parser rejection is a migration blocker (spec §5), not a warning to
// push through. Cutover never writes to svc's own database; the legacy
// entries table is untouched until schema v8 (see internal/db toV8) drops
// it, which must only ship after this has been run and verified.
func Cutover(ctx context.Context, database *sql.DB, blobs BlobFetcher, importer ContentImporter) ([]CutoverResult, error) {
	report, err := Run(ctx, database, blobs)
	if err != nil {
		return nil, fmt.Errorf("conversion pass: %w", err)
	}

	results := make([]CutoverResult, 0, len(report.Entries))
	for _, e := range report.Entries {
		path := e.Slug
		if e.DirectoryPath != "" {
			path = e.DirectoryPath + "/" + e.Slug
		}

		if e.Status == "failed" {
			results = append(results, CutoverResult{EntryID: e.EntryID, Path: path, Status: "skipped-failed", Err: e.ParseError})
			continue
		}

		err := importer.CreateEntry(ctx, e.ProjectID, e.Slug, e.DirectoryPath, e.Markdown)
		switch {
		case err == nil, errors.Is(err, ErrAlreadyExists):
			results = append(results, CutoverResult{EntryID: e.EntryID, Path: path, Status: "imported"})
		default:
			results = append(results, CutoverResult{EntryID: e.EntryID, Path: path, Status: "error", Err: err.Error()})
		}
	}
	return results, nil
}
