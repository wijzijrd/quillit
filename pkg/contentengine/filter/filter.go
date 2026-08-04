// Package filter reduces a parsed entry to exactly what a given view is
// allowed to show, per docs/cli-spec.md §4 "View semantics" and §7
// "render". It never does I/O or rendering — Filter takes a *parse.Entry
// and a View and returns a FilteredEntry the render package walks to
// produce HTML.
package filter

import (
	"fmt"
	"strings"

	"github.com/quillit/contentengine/parse"
)

// ViewKind selects which of the CLI spec §4 views to filter for.
type ViewKind int

const (
	// ViewDM is the default: everything, unchanged.
	ViewDM ViewKind = iota
	// ViewPlayer strips every :::secret block (and anything nested
	// inside one) entirely.
	ViewPlayer
	// ViewCard extracts only :::card blocks matching Facet, wherever
	// they occur in the tree (including inside a :::secret — card view
	// is a DM tool, per CLI spec §4, so secret-nested cards are still
	// reachable by facet).
	ViewCard
)

// View describes a render request. Kind and Facet mirror docs/cli-spec.md
// §7 "render"'s `--view`/`--card` flags, which are mutually exclusive —
// callers construct a View with Facet set only when Kind is ViewCard.
type View struct {
	Kind  ViewKind
	Facet string // set only when Kind == ViewCard
	Quick bool   // -Q: truncate to the first surviving block
}

// FilteredEntry is a view's surviving content: the same Block shape
// parse.Entry.Body uses, already reduced to exactly what that view
// permits. render walks Blocks identically regardless of which View
// produced them — every view-specific decision has already been made
// here.
type FilteredEntry struct {
	Name   string
	Tags   []string
	Blocks []parse.Block
}

// UnknownFacet is returned both when the requested View's Facet (for a
// card view) isn't in vocabulary, and when the entry's own content
// references an undeclared facet regardless of the requested view — CLI
// spec §4: "Render with unknown facet, or .md containing undeclared
// facet → error." Fail loud either way (golden rule 6): vocabulary is
// listed so the caller can present the fix.
type UnknownFacet struct {
	Name       string
	Vocabulary []string
}

func (e UnknownFacet) Error() string {
	return fmt.Sprintf("unknown facet %q (effective vocabulary: %s)", e.Name, strings.Join(e.Vocabulary, ", "))
}

// Filter reduces entry to exactly what view permits. vocabulary is the
// effective facet vocabulary (global ∪ project extras) — Filter never
// reads config.yaml/quillit.yaml itself; the vocabulary is a value the
// caller supplies (docs/web-refactor-spec.md §7.1 requirement 1).
func Filter(entry *parse.Entry, view View, vocabulary []string) (*FilteredEntry, error) {
	if err := validateFacets(entry.Body, vocabulary); err != nil {
		return nil, err
	}
	if view.Kind == ViewCard && !contains(vocabulary, view.Facet) {
		return nil, UnknownFacet{Name: view.Facet, Vocabulary: vocabulary}
	}

	var blocks []parse.Block
	switch view.Kind {
	case ViewCard:
		blocks = collectCards(entry.Body, view.Facet)
	case ViewPlayer:
		blocks = stripSecrets(entry.Body)
	default:
		blocks = entry.Body
	}

	if view.Quick && len(blocks) > 0 {
		blocks = blocks[:1]
	}

	return &FilteredEntry{Name: entry.Frontmatter.Name, Tags: entry.Frontmatter.Tags, Blocks: blocks}, nil
}

func contains(vocabulary []string, name string) bool {
	for _, v := range vocabulary {
		if v == name {
			return true
		}
	}
	return false
}

// validateFacets walks the entire block tree, including nested blocks,
// checking every :::card facet against vocabulary.
func validateFacets(blocks []parse.Block, vocabulary []string) error {
	for _, b := range blocks {
		if b.Kind == parse.BlockCard && !contains(vocabulary, b.Facet) {
			return UnknownFacet{Name: b.Facet, Vocabulary: vocabulary}
		}
		if err := validateFacets(b.Blocks, vocabulary); err != nil {
			return err
		}
	}
	return nil
}

// stripSecrets returns blocks with every BlockSecret — and everything
// nested inside it — removed. This is the golden-rule-5 guarantee: the
// excluded content is never copied into the result, not merely marked
// hidden for later stripping.
func stripSecrets(blocks []parse.Block) []parse.Block {
	var out []parse.Block
	for _, b := range blocks {
		if b.Kind == parse.BlockSecret {
			continue
		}
		out = append(out, b)
	}
	return out
}

// collectCards recursively finds every card block matching facet
// anywhere in the tree, flattened into document order. Returned blocks
// have Blocks cleared: once flattened into a card-view result they're no
// longer nested inside anything.
func collectCards(blocks []parse.Block, facet string) []parse.Block {
	var out []parse.Block
	for _, b := range blocks {
		if b.Kind == parse.BlockCard && b.Facet == facet {
			flat := b
			flat.Blocks = nil
			out = append(out, flat)
		}
		out = append(out, collectCards(b.Blocks, facet)...)
	}
	return out
}
