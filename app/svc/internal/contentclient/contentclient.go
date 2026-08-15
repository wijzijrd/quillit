// Package contentclient is svc's HTTP client for the quillit/content
// service — the one live cross-service dependency svc has left after
// issue #35's cutover (Game Mode's "share card" chat feature needs to read
// an entry's title/body/project to snapshot it into a chat message; content
// now owns that data). See docs/web-refactor-spec.md §10.11 for the wider
// svc↔content seam this is a minimal instance of.
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
