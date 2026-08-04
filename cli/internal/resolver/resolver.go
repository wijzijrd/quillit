// Package resolver implements the content-engine render.Resolver
// interface over a project's entry folders on disk, and provides the
// mtime-based staleness check the compile/render/export commands use to
// decide when a links.conf needs recompiling.
package resolver

import (
	"os"
	"path/filepath"

	"github.com/quillit/contentengine/parse"
	"github.com/quillit/contentengine/render"
)

// FS resolves wikilinks over a single project's directory tree.
type FS struct {
	ProjectRoot string
}

var _ render.Resolver = FS{}

// Resolve reports whether path names a real entry — a directory
// containing <basename>.md, per CLI spec §5's entry folder anatomy — and
// if so returns a function that parses that entry looking for a card
// matching a given facet.
func (r FS) Resolve(path string) (exists bool, cardForFacet func(facet string) (content string, ok bool)) {
	entryDir := filepath.Join(r.ProjectRoot, path)
	info, err := os.Stat(entryDir)
	if err != nil || !info.IsDir() {
		return false, nil
	}
	return true, func(facet string) (string, bool) {
		return cardContent(entryDir, path, facet)
	}
}

func cardContent(entryDir, entryPath, facet string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(entryDir, filepath.Base(entryPath)+".md"))
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

// findCard returns the first card block matching facet, in document
// order, searching nested blocks too (a card may sit inside a secret —
// CLI spec §4). If more than one block shares a facet, the first one
// found wins; combining multiple same-facet cards isn't something the
// spec describes, so this package doesn't attempt it.
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

// IsStale reports whether mdPath's content is newer than confPath (or
// confPath doesn't exist yet) — the lazy auto-refresh check CLI spec §7
// "compile" describes: "render/export compare .md mtime vs links.conf,
// recompile when stale — compile never required, just forces/warms the
// cache."
func IsStale(mdPath, confPath string) (bool, error) {
	mdInfo, err := os.Stat(mdPath)
	if err != nil {
		return false, err
	}
	confInfo, err := os.Stat(confPath)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return mdInfo.ModTime().After(confInfo.ModTime()), nil
}
