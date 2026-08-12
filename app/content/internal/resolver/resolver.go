// Package resolver implements the content-engine render.Resolver interface
// over the content service's own SQLite entries table and MinIO body
// storage — the web analogue of the CLI's filesystem-backed resolver
// (cli/internal/resolver), per docs/web-refactor-spec.md §7.1 requirement 1
// ("web implements it over SQLite + entry_links").
package resolver

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/quillit/contentengine/parse"
	"github.com/quillit/contentengine/render"
)

// BlobGetter is the one storage operation SQLite needs — a fetched entry's
// raw body, to re-parse it looking for a matching card. Kept minimal (not
// the full storage.MinioStore/handler.BlobStore surface) so this package
// doesn't need to import the handler package, which would create an import
// cycle (handler constructs a SQLite per render request).
type BlobGetter interface {
	Get(ctx context.Context, key string) ([]byte, error)
}

// SQLite resolves wikilinks over a single project's entries.
type SQLite struct {
	DB        *sql.DB
	Blobs     BlobGetter // nil is valid — Resolve then always reports no card (dangling-for-expansion, same as a resolver that never finds targets)
	ProjectID string
	Ctx       context.Context
}

var _ render.Resolver = (*SQLite)(nil)

// Resolve reports whether path names a real entry in this project — see
// EntryBodyKey for the path/slug convention this mirrors from
// internal/handler/links.go's resolveEntryPath (same split, independent
// copy here to avoid an import cycle: handler -> resolver at render-request
// time, resolver -> handler would close the loop).
func (r *SQLite) Resolve(path string) (exists bool, cardForFacet func(facet string) (content string, ok bool)) {
	dir, slug := splitEntryPath(path)
	var id string
	err := r.DB.QueryRowContext(r.Ctx,
		`SELECT id FROM entries WHERE project_id = ? AND directory_path = ? AND slug = ?`,
		r.ProjectID, dir, slug,
	).Scan(&id)
	if err != nil {
		return false, nil
	}
	return true, func(facet string) (string, bool) {
		return r.cardContent(id, facet)
	}
}

func (r *SQLite) cardContent(entryID, facet string) (string, bool) {
	if r.Blobs == nil {
		return "", false
	}
	data, err := r.Blobs.Get(r.Ctx, entryBodyKey(entryID))
	if err != nil {
		return "", false
	}
	entry, err := parse.Parse(data)
	if err != nil {
		return "", false
	}
	block, ok := findCard(entry.Body, facet)
	if !ok {
		return "", false
	}
	return block.Content, true
}

// findCard returns the first card block matching facet, in document order,
// searching nested blocks too (a card may sit inside a secret — CLI spec
// §4). Mirrors cli/internal/resolver.findCard.
func findCard(blocks []parse.Block, facet string) (parse.Block, bool) {
	for _, b := range blocks {
		if b.Kind == parse.BlockCard && b.Facet == facet {
			return b, true
		}
		if found, ok := findCard(b.Blocks, facet); ok {
			return found, ok
		}
	}
	return parse.Block{}, false
}

// entryBodyKey is the MinIO key convention for an entry's body — must stay
// identical to internal/handler/entries.go's bodyKey.
func entryBodyKey(entryID string) string {
	return fmt.Sprintf("entries/%s/body.md", entryID)
}

// splitEntryPath splits a wikilink target path into (directory_path, slug),
// matching docs/web-refactor-spec.md §4.1: "Entry path = directory_path +
// '/' + slug". Must stay identical to internal/handler/links.go's
// splitEntryPath.
func splitEntryPath(path string) (dir, slug string) {
	path = strings.Trim(path, "/")
	idx := strings.LastIndex(path, "/")
	if idx < 0 {
		return "", path
	}
	return path[:idx], path[idx+1:]
}
