// Package contentclient is svc's connectrpc client for the quillit/content
// service's ContentInternalService — the live cross-service dependency svc
// has left after issue #35's cutover: Game Mode's "share card" chat feature
// needs to read an entry's title/body/project to snapshot it into a chat
// message (Get), and #44 added the reverse direction — svc telling content
// when a project it owned was deleted (NotifyProjectDeleted), so content can
// apply its own entry-retention policy rather than svc reaching into
// content's database directly. See docs/web-refactor-spec.md §10.11 for the
// wider svc↔content seam both are instances of.
//
// Both RPCs used to be plain HTTP calls; they're now connectrpc calls
// against content's ContentInternalService (see
// gen/quillit/content/internal/v1 and app/content/internal/rpc for the
// server side), authenticated with the shared INTERNAL_RPC_SECRET rather
// than per-request headers this package used to build by hand. Client's
// exported shape (Get/NotifyProjectDeleted's signatures, the Entry struct,
// ErrNotFound) is unchanged — only the transport underneath it moved.
package contentclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"connectrpc.com/connect"

	"github.com/quillit/gen/internalauth"
	v1 "github.com/quillit/gen/quillit/content/v1"
	"github.com/quillit/gen/quillit/content/v1/contentv1connect"
)

var ErrNotFound = errors.New("entry not found")

type Entry struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	Title     string `json:"title"`
	Body      string `json:"body"`
}

// Client is a connectrpc client for content's ContentInternalService.
type Client struct {
	rpc contentv1connect.ContentInternalServiceClient
}

// NewClient builds a Client pointed at content's baseURL, authenticating
// every call with secret (INTERNAL_RPC_SECRET — see gen/internalauth).
// httpClient may be nil, in which case http.DefaultClient is used.
func NewClient(baseURL, secret string, httpClient *http.Client) *Client {
	var hc connect.HTTPClient = http.DefaultClient
	if httpClient != nil {
		hc = httpClient
	}
	return &Client{
		rpc: contentv1connect.NewContentInternalServiceClient(
			hc,
			baseURL,
			connect.WithInterceptors(internalauth.NewClientInterceptor(secret)),
		),
	}
}

// Get fetches entryID's id/project/title/body. Returns ErrNotFound if
// content has no such entry (connect.CodeNotFound), or a wrapped error for
// anything else (network failure, an internal error from content, ...).
func (c *Client) Get(ctx context.Context, entryID string) (Entry, error) {
	resp, err := c.rpc.GetEntry(ctx, connect.NewRequest(&v1.GetEntryRequest{EntryId: entryID}))
	if err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			return Entry{}, ErrNotFound
		}
		return Entry{}, fmt.Errorf("GetEntry(%s): %w", entryID, err)
	}
	return Entry{
		ID:        resp.Msg.GetId(),
		ProjectID: resp.Msg.GetProjectId(),
		Title:     resp.Msg.GetTitle(),
		Body:      resp.Msg.GetBody(),
	}, nil
}

// NotifyProjectDeleted tells content that projectID was deleted in svc
// (called from ProjectsHandler.Delete after svc's own delete — and
// everything that cascades from it — has already succeeded). content
// applies its own policy to that project's entries (orphan-and-report,
// not hard-delete — see app/content/internal/rpc/content_internal.go's
// NotifyProjectDeleted for the reasoning) rather than svc dictating it.
func (c *Client) NotifyProjectDeleted(ctx context.Context, projectID string) error {
	_, err := c.rpc.NotifyProjectDeleted(ctx, connect.NewRequest(&v1.NotifyProjectDeletedRequest{ProjectId: projectID}))
	if err != nil {
		return fmt.Errorf("NotifyProjectDeleted(%s): %w", projectID, err)
	}
	return nil
}
