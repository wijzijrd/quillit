package handler

import (
	"context"
	"database/sql"
	"strings"

	"github.com/quillit/contentengine/linkindex"
	"github.com/quillit/contentengine/parse"
)

// recompileLinks replaces entryID's entry_links rows with the outgoing
// wikilinks extracted from entry — the web analogue of the CLI's
// links.conf recompile (docs/web-refactor-spec.md §4.6): a save is the
// staleness event, so every write recomputes the full set rather than
// diffing. Dangling links (Resolved=false) are recorded, never dropped or
// treated as an error — matching CLI `compile` behavior.
func recompileLinks(ctx context.Context, tx *sql.Tx, entryID, projectID string, entry *parse.Entry) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM entry_links WHERE entry_id = ?`, entryID); err != nil {
		return err
	}
	for _, rec := range linkindex.Extract(entry) {
		targetID, resolved, err := resolveEntryPath(ctx, tx, projectID, rec.TargetPath)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO entry_links (entry_id, target_path, target_entry_id, label, card_facet, resolved)
			VALUES (?, ?, ?, ?, ?, ?)
		`, entryID, rec.TargetPath, nullStr(targetID), rec.Label, nullStr(rec.CardFacet), resolved); err != nil {
			return err
		}
	}
	return nil
}

// resolveEntryPath looks up the entry at path (directory_path + "/" + slug,
// or bare slug when directory_path is "") within projectID.
func resolveEntryPath(ctx context.Context, tx *sql.Tx, projectID, path string) (id string, resolved bool, err error) {
	dir, slug := splitEntryPath(path)
	err = tx.QueryRowContext(ctx,
		`SELECT id FROM entries WHERE project_id = ? AND directory_path = ? AND slug = ?`,
		projectID, dir, slug,
	).Scan(&id)
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

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
