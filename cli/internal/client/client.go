// Package client is the CLI's HTTP client for the web app's gateway —
// login, project listing, and project import (spec §6).
package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrUnauthorized = errors.New("session expired or invalid — run `quillit login`")

type Client struct {
	Server string
	Token  string
	HTTP   *http.Client
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 120 * time.Second}
}

func (c *Client) do(method, path string, query url.Values, body io.Reader, contentType string) (*http.Response, error) {
	u := strings.TrimRight(c.Server, "/") + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequest(method, u, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.AddCookie(&http.Cookie{Name: "session", Value: c.Token})
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("reaching %s: %w", c.Server, err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		return nil, ErrUnauthorized
	}
	return resp, nil
}

// Login authenticates against the gateway and returns the raw session JWT
// extracted from the "session" cookie the gateway sets.
func Login(server, email, password string) (string, error) {
	payload, _ := json.Marshal(map[string]string{"email": email, "password": password})
	req, err := http.NewRequest(http.MethodPost,
		strings.TrimRight(server, "/")+"/api/auth/login", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return "", fmt.Errorf("reaching %s: %w", server, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("login failed (%s)", readErrorMessage(resp))
	}
	for _, c := range resp.Cookies() {
		if c.Name == "session" && c.Value != "" {
			return c.Value, nil
		}
	}
	return "", errors.New("login succeeded but no session cookie was returned")
}

type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (c *Client) ListProjects() ([]Project, error) {
	resp, err := c.do(http.MethodGet, "/api/projects", nil, nil, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("listing projects failed (%s)", readErrorMessage(resp))
	}
	var ps []Project
	if err := json.NewDecoder(resp.Body).Decode(&ps); err != nil {
		return nil, err
	}
	return ps, nil
}

type ImportOptions struct {
	Mode         string
	OnConflict   string
	CreateFacets bool
}

type ImportReportRow struct {
	Path   string `json:"path"`
	Action string `json:"action"`
	Detail string `json:"detail,omitempty"`
}

type ImportResponse struct {
	Applied bool              `json:"applied"`
	Report  []ImportReportRow `json:"report"`
	Facets  struct {
		Created []string `json:"created"`
	} `json:"facets"`
	Images []struct {
		Path     string `json:"path"`
		Uploaded bool   `json:"uploaded"`
		Detail   string `json:"detail,omitempty"`
	} `json:"images"`
}

type ValidationError struct {
	Entries []struct {
		Path  string `json:"path"`
		Error string `json:"error"`
	} `json:"entries"`
	MissingFacets []string `json:"missingFacets"`
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation failed: %d invalid entries, %d missing facets", len(e.Entries), len(e.MissingFacets))
}

func (c *Client) Import(projectID string, tarball io.Reader, opts ImportOptions) (*ImportResponse, error) {
	q := url.Values{}
	q.Set("mode", opts.Mode)
	q.Set("onConflict", opts.OnConflict)
	if opts.CreateFacets {
		q.Set("createFacets", "true")
	}
	resp, err := c.do(http.MethodPost, "/api/content/projects/"+projectID+"/import", q, tarball, "application/gzip")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var out ImportResponse
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return nil, err
		}
		return &out, nil
	case http.StatusUnprocessableEntity:
		var ve ValidationError
		if err := json.NewDecoder(resp.Body).Decode(&ve); err != nil {
			return nil, fmt.Errorf("import rejected (422) but response unreadable: %w", err)
		}
		return nil, &ve
	default:
		return nil, fmt.Errorf("import failed (%s)", readErrorMessage(resp))
	}
}

// readErrorMessage extracts {"error": "..."} bodies, falling back to the
// HTTP status.
func readErrorMessage(resp *http.Response) string {
	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err == nil && body.Error != "" {
		return body.Error
	}
	return resp.Status
}
