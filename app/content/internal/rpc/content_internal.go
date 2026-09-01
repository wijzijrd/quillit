// Package rpc implements content's server-to-server connect RPC surface:
// ContentInternalService (proto/quillit/content/internal/v1, generated into
// github.com/quillit/gen/quillit/content/v1). It is the connectrpc
// replacement for two HTTP routes that used to live in
// app/content/internal/handler:
//   - GetEntry replaces GET /content/entries/{id} for svc's internal caller
//     (contentclient.Client.Get, used by Game Mode's "share card" chat
//     feature) — but note that HTTP route itself is NOT removed, because
//     svc also proxies real, per-user, UI-facing requests through it (see
//     app/svc/internal/handler/content_entries.go's GetEntry, mounted at
//     GET /api/content/entries/{id}) and that path must keep its existing
//     JWT + project-membership authorization. GetEntry here is a separate,
//     deliberately un-authenticated-by-JWT surface: it's reached only by
//     svc itself, gated by the shared-secret interceptor
//     (gen/internalauth), the same trust boundary NotifyProjectDeleted
//     below already relied on before this package existed.
//   - NotifyProjectDeleted replaces POST
//     /content/internal/projects/{id}/deleted (the old
//     handler.InternalHandler.ProjectDeleted), which is removed now that
//     this RPC is its only caller.
//
// Both methods are backed by the exact same sqlc-based logic the old HTTP
// handlers used (handler.FetchResolvedEntry, handler.OrphanProjectEntries)
// — this package only swaps the transport.
package rpc

import (
	"context"
	"database/sql"
	"errors"

	"connectrpc.com/connect"

	v1 "github.com/quillit/gen/quillit/content/v1"

	"github.com/quillit/content-svc/internal/db/sqlc"
	"github.com/quillit/content-svc/internal/handler"
)

// ContentInternalServer implements contentv1connect.ContentInternalServiceHandler.
type ContentInternalServer struct {
	q     *sqlc.Queries
	blobs handler.BlobStore
}

// NewContentInternalServer builds a ContentInternalServer over db. blobs may
// be nil (matching handler.NewEntries' convention when blob storage isn't
// configured) — GetEntry then returns entries with an empty Body, same as
// FetchResolvedEntry does for any other caller.
func NewContentInternalServer(db *sql.DB, blobs handler.BlobStore) *ContentInternalServer {
	return &ContentInternalServer{q: sqlc.New(db), blobs: blobs}
}

// GetEntry looks up entryID and returns its id/project/title/body. A
// missing entry is the one definitive, callers-should-branch-on-this
// outcome — it's returned as connect.CodeNotFound (mirroring the old HTTP
// route's 404), distinct from any other (e.g. DB) failure, which is
// returned as connect.CodeInternal.
func (s *ContentInternalServer) GetEntry(ctx context.Context, req *connect.Request[v1.GetEntryRequest]) (*connect.Response[v1.GetEntryResponse], error) {
	entryID := req.Msg.GetEntryId()
	e, err := handler.FetchResolvedEntry(ctx, s.q, s.blobs, entryID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("entry not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.GetEntryResponse{
		Id:        e.ID,
		ProjectId: e.ProjectID,
		Title:     e.Title,
		Body:      e.Body,
	}), nil
}

// NotifyProjectDeleted orphans projectID's entries and reports how many
// were orphaned by this call (already-orphaned entries from a prior,
// idempotent call aren't recounted).
func (s *ContentInternalServer) NotifyProjectDeleted(ctx context.Context, req *connect.Request[v1.NotifyProjectDeletedRequest]) (*connect.Response[v1.NotifyProjectDeletedResponse], error) {
	projectID := req.Msg.GetProjectId()
	n, err := handler.OrphanProjectEntries(ctx, s.q, projectID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.NotifyProjectDeletedResponse{
		ProjectId:       projectID,
		EntriesOrphaned: int32(n),
	}), nil
}
