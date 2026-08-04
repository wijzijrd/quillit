package acceptance

import (
	"fmt"

	"github.com/quillit/contentengine/parse"
	"github.com/quillit/contentengine/render"
)

// memResolver implements render.Resolver over a fixed, in-memory set of
// already-parsed fixture entries, keyed by whatever wikilink path the
// fixtures in a given test corpus use to reference each other (e.g.
// "characters/npcs/mary", or a plain "chain-b" for fixtures that don't
// need to look like a real project tree). It exists only so this
// package's golden tests can exercise render's card-expansion logic
// (#24) without any real project or $QUILLIT_HOME — the equivalent of
// cli/internal/resolver.FS, but backed by a map instead of a
// filesystem.
type memResolver map[string]*parse.Entry

func (r memResolver) Resolve(path string) (exists bool, cardForFacet func(facet string) (content string, ok bool)) {
	entry, ok := r[path]
	if !ok {
		return false, nil
	}
	return true, func(facet string) (string, bool) {
		return findCard(entry.Body, facet)
	}
}

// findCard returns the first block matching facet, in document order,
// searching nested blocks too (a card may sit inside a secret).
func findCard(blocks []parse.Block, facet string) (string, bool) {
	for _, b := range blocks {
		if b.Kind == parse.BlockCard && b.Facet == facet {
			return b.Content, true
		}
		if content, ok := findCard(b.Blocks, facet); ok {
			return content, true
		}
	}
	return "", false
}

// plainLinkRenderer is a deterministic, content-engine-only stand-in for
// a real presentation layer (the CLI's clipboard-links, or a future
// web UI's in-app navigation). Golden files only need to prove *that* a
// non-expanded link renders as a stable, recognizable marker — what a
// specific consumer's markup for it looks like is that consumer's own
// concern and own tests (see cli/cmd/render_test.go's
// clipboardLinkRenderer tests).
func plainLinkRenderer(info render.LinkInfo) string {
	if info.NoMatchingCard {
		return fmt.Sprintf("[link:%s (no %s card)]", info.Path, info.Facet)
	}
	return fmt.Sprintf("[link:%s]", info.Path)
}
