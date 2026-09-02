package handler

import (
	"context"
	"database/sql"
	"strings"

	"github.com/quillit/contentengine/linkindex"
	"github.com/quillit/contentengine/parse"

	"github.com/quillit/content-svc/internal/db/sqlc"
)

// recompileLinks replaces entryID's entry_links rows with the outgoing
// wikilinks extracted from entry — the web analogue of the CLI's
// links.conf recompile (docs/web-refactor-spec.md §4.6): a save is the
// staleness event, so every write recomputes the full set rather than
// diffing. Dangling links (Resolved=false) are recorded, never dropped or
// treated as an error — matching CLI `compile` behavior. q is expected to be
// transaction-scoped (sqlc.Queries.WithTx) by every caller.
func recompileLinks(ctx context.Context, q *sqlc.Queries, entryID, projectID string, entry *parse.Entry) error {
	if err := q.DeleteEntryLinks(ctx, entryID); err != nil {
		return err
	}
	for _, rec := range linkindex.Extract(entry) {
		targetID, resolved, err := resolveEntryPath(ctx, q, projectID, rec.TargetPath)
		if err != nil {
			return err
		}
		if err := q.InsertEntryLink(ctx, sqlc.InsertEntryLinkParams{
			EntryID:       entryID,
			TargetPath:    rec.TargetPath,
			TargetEntryID: nullString(targetID),
			Label:         rec.Label,
			CardFacet:     nullString(rec.CardFacet),
			Resolved:      boolToInt64(resolved),
		}); err != nil {
			return err
		}
	}
	return nil
}

// resolveEntryPath looks up the entry at path (directory_path + "/" + slug,
// or bare slug when directory_path is "") within projectID.
func resolveEntryPath(ctx context.Context, q *sqlc.Queries, projectID, path string) (id string, resolved bool, err error) {
	dir, slug := splitEntryPath(path)
	id, err = q.FindEntryIDAtPath(ctx, sqlc.FindEntryIDAtPathParams{
		ProjectID:     projectID,
		DirectoryPath: dir,
		Slug:          slug,
	})
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return id, true, nil
}

// splitEntryPath splits a wikilink target path into (directory_path, slug),
// matching docs/web-refactor-spec.md §4.1: "Entry path = directory_path +
// '/' + slug". A path with no "/" (project-root entry) has directory_path "".
func splitEntryPath(path string) (dir, slug string) {
	path = strings.Trim(path, "/")
	idx := strings.LastIndex(path, "/")
	if idx < 0 {
		return "", path
	}
	return path[:idx], path[idx+1:]
}

// joinEntryPath is splitEntryPath's inverse: builds an entry's path from its
// directory_path and slug, per the same §4.1 convention. A blank dir
// (project-root entry) yields the bare slug. Used by assign.go to compute
// an entry's old/new path around a directory move.
func joinEntryPath(dir, slug string) string {
	if dir == "" {
		return slug
	}
	return strings.TrimRight(dir, "/") + "/" + slug
}

// nullString converts a Go string into the sql.NullString sqlc's generated
// params expect for a nullable TEXT column — "" means NULL (matching the
// pre-sqlc convention of only binding a value when non-empty), any other
// value is bound as-is.
func nullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

// boolToInt64 converts a Go bool into the int64 sqlc's generated params
// expect for entry_links.resolved (INTEGER NOT NULL, no boolean affinity
// hint in the schema).
func boolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
