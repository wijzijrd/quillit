package handler

import (
	"context"
	"database/sql"

	"github.com/quillit/content-svc/internal/db/sqlc"
)

// OrphanProjectEntries stamps orphaned_at on every not-yet-orphaned entry in
// projectID and returns how many were orphaned — the "and-report" half of
// #44's orphan-and-report policy: content has no FK to svc's projects
// table, so a mistaken project deletion doesn't have to cascade into
// irreversible entry/blob loss the way svc's own project-owned tables do.
// Idempotent — entries already orphaned by a prior call aren't recounted.
//
// This is the one piece of transport-independent logic behind svc's
// "a project was deleted" notification. It used to live inline in an
// HTTP-only InternalHandler.ProjectDeleted (POST
// /content/internal/projects/{id}/deleted); that route and handler are gone
// as of the connectrpc cutover (see internal/rpc/content_internal.go's
// ContentInternalServer.NotifyProjectDeleted, mounted in main.go) — this
// function is exported so that RPC handler can call the exact same DB logic
// rather than reimplementing it.
func OrphanProjectEntries(ctx context.Context, q *sqlc.Queries, projectID string) (int, error) {
	n, err := q.OrphanProjectEntries(ctx, sqlc.OrphanProjectEntriesParams{
		OrphanedAt: sql.NullInt64{Int64: nowUnix(), Valid: true},
		ProjectID:  projectID,
	})
	if err != nil {
		return 0, err
	}
	return int(n), nil
}
