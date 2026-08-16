// Package contentclient is svc's HTTP client for the quillit/content
// service — the live cross-service dependency svc has left after issue
// #35's cutover: Game Mode's "share card" chat feature needs to read an
// entry's title/body/project to snapshot it into a chat message (Get), and
// #44 added the reverse direction — svc telling content when a project it
// owned was deleted (NotifyProjectDeleted), so content can apply its own
// entry-retention policy rather than svc reaching into content's database
// directly. See docs/web-refactor-spec.md §10.11 for the wider svc↔content
// seam both are instances of.
package contentclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

var ErrNotFound = errors.New("entry not found")

type Entry struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	Title     string `json:"title"`
	Body      string `json:"body"`
}

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func (c *Client) Get(ctx context.Context, entryID string) (Entry, error) {
	reqURL := fmt.Sprintf("%s/content/entries/%s", c.BaseURL, url.PathEscape(entryID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return Entry{}, fmt.Errorf("build request: %w", err)
	}

	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return Entry{}, fmt.Errorf("GET %s: %w", reqURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return Entry{}, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return Entry{}, fmt.Errorf("GET %s: unexpected status %d", reqURL, resp.StatusCode)
	}

	var e Entry
	if err := json.NewDecoder(resp.Body).Decode(&e); err != nil {
		return Entry{}, fmt.Errorf("decode response: %w", err)
	}
	return e, nil
}

// NotifyProjectDeleted tells content that projectID was deleted in svc
// (called from ProjectsHandler.Delete after svc's own delete — and
// everything that cascades from it — has already succeeded). content
// applies its own policy to that project's entries (orphan-and-report,
// not hard-delete — see app/content/internal/handler/internal.go's
// ProjectDeleted for the reasoning) rather than svc dictating it.
func (c *Client) NotifyProjectDeleted(ctx context.Context, projectID string) error {
	reqURL := fmt.Sprintf("%s/content/internal/projects/%s/deleted", c.BaseURL, url.PathEscape(projectID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", reqURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("POST %s: unexpected status %d", reqURL, resp.StatusCode)
	}
	return nil
}
