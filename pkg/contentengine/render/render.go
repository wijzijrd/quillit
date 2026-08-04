// Package render turns a filter.FilteredEntry into an HTML fragment, per
// docs/cli-spec.md §4 and docs/web-refactor-spec.md §7.1. It performs the
// spec's depth-1 card expansion and delegates non-expanded-link
// presentation to a caller-supplied LinkRenderer — the package itself
// never decides "clipboard-link" vs "in-app-link" styling, and touches
// neither the filesystem nor a database: entry lookups go through the
// Resolver interface, implemented differently by the CLI and (later) the
// quillit/content service.
package render

import (
	"bytes"
	"fmt"
	"html"
	"strings"

	"github.com/quillit/contentengine/filter"
	"github.com/quillit/contentengine/parse"
	"github.com/yuin/goldmark"
	goldmarkhtml "github.com/yuin/goldmark/renderer/html"
)

// Resolver looks up other entries by path, for wikilink resolution and
// depth-1 card expansion (CLI spec §4 "Card expansion"). The CLI
// implements this over the project tree + links.conf; a future service
// implements it over a database — this package touches neither directly.
type Resolver interface {
	// Resolve reports whether path names a real entry. If it does,
	// cardForFacet fetches that entry's card content for a given facet
	// (ok is false if the entry has no card for that facet — CLI spec
	// §4: "Linked entry lacks card for requested facet → ... render
	// doesn't fail").
	Resolve(path string) (exists bool, cardForFacet func(facet string) (content string, ok bool))
}

// LinkInfo is what a LinkRenderer needs to produce presentation for one
// wikilink that isn't being expanded inline.
type LinkInfo struct {
	Path  string
	Label string

	// Facet is set when this link was encountered while rendering a
	// card view — the facet being rendered, so e.g. the CLI's
	// clipboard-link presentation can build a
	// "quillit render <path> --card <facet>" command. Empty for links
	// in a DM full-entry render, which has no single active facet.
	Facet string

	// NoMatchingCard is true specifically for CLI spec §4's "linked
	// entry lacks card for requested facet" case: the link's target
	// exists but has no card for Facet. render doesn't hardcode any
	// note text for this — that's presentation, left to the caller.
	NoMatchingCard bool
}

// LinkRenderer produces the HTML for one non-expanded wikilink. Called
// for every link except (a) ones successfully depth-1 expanded in a card
// view, and (b) any link at all in player view — player view never calls
// this, structurally: golden rule 5's player-safe guarantee extends to
// link presentation, not just secret content (CLI spec §4: "Links are
// DM-render-only... Player view... show[s] link's plain text label
// only").
type LinkRenderer func(LinkInfo) string

var markdown = goldmark.New(
	goldmark.WithRendererOptions(goldmarkhtml.WithUnsafe()),
)

// Render turns entry into an HTML fragment — no <html>/<head>/<body>
// wrapper (docs/web-refactor-spec.md §7.1 requirement 2); the caller
// assembles it into a full document (CLI) or app shell (web).
//
// view must be the same View used to produce entry via filter.Filter —
// Render needs it to know whether it's rendering a card facet (for
// depth-1 expansion and LinkInfo.Facet) and, above all, whether it's
// player view, in which case linkRenderer is never invoked and nothing
// is ever expanded, full stop.
//
// resolver may be nil (e.g. a caller that never needs card expansion,
// such as a player-view/export-only render); expansion is then simply
// never attempted, same as if every Resolve call reported the link as
// dangling.
func Render(entry *filter.FilteredEntry, view filter.View, resolver Resolver, linkRenderer LinkRenderer) (string, error) {
	var b strings.Builder
	if entry.Name != "" {
		b.WriteString("<h1>" + html.EscapeString(entry.Name) + "</h1>\n")
	}

	for _, block := range entry.Blocks {
		rendered, err := renderBlock(block, view, resolver, linkRenderer, 0)
		if err != nil {
			return "", err
		}
		b.WriteString(rendered)
	}

	return b.String(), nil
}

// renderBlock renders one block (and, for a card block at depth 0 in a
// card view, its depth-1 expansions) to HTML. depth is 0 for blocks from
// the entry itself, 1 for an expanded card's own content — depth is never
// more than 1 by construction: expansion only happens when depth == 0.
func renderBlock(block parse.Block, view filter.View, resolver Resolver, linkRenderer LinkRenderer, depth int) (string, error) {
	decisions := decideLinks(block.Links, view, resolver, depth)

	resolveInline := func(link parse.Wikilink) string {
		if view.Kind == filter.ViewPlayer {
			// Structural guarantee, not a default: player view never
			// calls linkRenderer, full stop.
			return html.EscapeString(link.Label)
		}
		if d, ok := decisions[link.Path]; ok && d.expanded {
			// Successfully expanded below (see the appended block in
			// renderBlock's BlockCard case) — the inline occurrence is
			// plain text, not a callback-rendered link, since the full
			// card content already follows immediately after. This is
			// also why linkRenderer is only ever called for
			// *non*-expanded links, at any depth.
			return html.EscapeString(link.Label)
		}
		info := LinkInfo{Path: link.Path, Label: link.Label}
		if view.Kind == filter.ViewCard {
			info.Facet = view.Facet
		}
		if d, ok := decisions[link.Path]; ok && d.attempted && !d.expanded {
			info.NoMatchingCard = true
		}
		return linkRenderer(info)
	}

	body, err := renderMarkdown(block.Content, block.Links, resolveInline)
	if err != nil {
		return "", err
	}

	switch block.Kind {
	case parse.BlockCard:
		class := "card-block"
		if depth > 0 {
			class += " card-block--expanded"
		}
		out := fmt.Sprintf(`<div class="%s">%s</div>`+"\n", class, body)

		// Depth-1 card expansion (CLI spec §4 "Card expansion") only
		// applies when actually rendering a card view, and only at
		// depth 0 — an already-expanded card's own links never expand
		// further ("Links inside inlined (expanded) card do not expand
		// further"). Full DM-view rendering of a card block never
		// expands either: the spec's "Rendering card: link inside
		// rendered card block pulls..." describes --card rendering
		// specifically, where there's a single active facet to look up
		// on the linked entry; plain DM view has no such single facet
		// in play.
		if depth == 0 && view.Kind == filter.ViewCard {
			for _, link := range block.Links {
				d, ok := decisions[link.Path]
				if !ok || !d.expanded {
					continue
				}
				expanded, err := renderExpandedCard(d.content, view.Facet, resolver, linkRenderer)
				if err != nil {
					return "", err
				}
				out += expanded
			}
		}
		return out, nil

	case parse.BlockSecret:
		return `<div class="secret-block">` + body + "</div>\n", nil

	default: // BlockProse
		return body, nil
	}
}

// renderExpandedCard renders a linked entry's card content (as returned
// raw by a Resolver's cardForFacet) at depth 1: its own links always go
// through linkRenderer, never expand further.
func renderExpandedCard(content, facet string, resolver Resolver, linkRenderer LinkRenderer) (string, error) {
	block := parse.Block{Kind: parse.BlockCard, Facet: facet, Content: content, Links: parse.ExtractLinks(content)}
	return renderBlock(block, filter.View{Kind: filter.ViewCard, Facet: facet}, resolver, linkRenderer, 1)
}

// linkDecision records what, if anything, was attempted for a link's
// depth-1 expansion, so both the inline placeholder substitution and the
// appended expanded-block logic in renderBlock can share one Resolver
// lookup per link instead of duplicating it.
type linkDecision struct {
	attempted bool   // true if expansion was attempted at all (depth 0, card view, resolver non-nil, target exists)
	expanded  bool   // true if the target had a card for the requested facet
	content   string // that card's raw content, when expanded is true
}

// decideLinks precomputes the expansion decision for every link in a
// block, once, per docs/cli-spec.md §4. Returns an empty map (nothing
// attempted for any link) whenever expansion isn't in play at all: wrong
// depth, not a card view, or no resolver supplied.
func decideLinks(links []parse.Wikilink, view filter.View, resolver Resolver, depth int) map[string]linkDecision {
	decisions := make(map[string]linkDecision, len(links))
	if depth != 0 || view.Kind != filter.ViewCard || resolver == nil {
		return decisions
	}
	for _, link := range links {
		if _, already := decisions[link.Path]; already {
			continue
		}
		exists, cardForFacet := resolver.Resolve(link.Path)
		if !exists {
			// Dangling link: not "attempted" in the NoMatchingCard sense
			// — that's specifically for a target that exists but lacks
			// the facet. Dangling links are compile's concern
			// (reported as warnings there), not an error here.
			continue
		}
		content, ok := cardForFacet(view.Facet)
		decisions[link.Path] = linkDecision{attempted: true, expanded: ok, content: content}
	}
	return decisions
}

// renderMarkdown converts one block's raw markdown Content to HTML,
// substituting each wikilink with resolveInline's result. links must be
// exactly the wikilinks found in content (a Block's own Links, or
// parse.ExtractLinks run on raw Resolver content).
func renderMarkdown(content string, links []parse.Wikilink, resolveInline func(parse.Wikilink) string) (string, error) {
	if len(links) == 0 {
		return convertMarkdown(content)
	}

	placeholders := make([]string, len(links))
	replaced := content
	for i, link := range links {
		placeholders[i] = fmt.Sprintf(`<span data-quillit-link-placeholder="%d"></span>`, i)
		raw := "[[" + link.Path
		if link.Label != link.Path {
			raw += "|" + link.Label
		}
		raw += "]]"
		replaced = strings.Replace(replaced, raw, placeholders[i], 1)
	}

	out, err := convertMarkdown(replaced)
	if err != nil {
		return "", err
	}

	for i, link := range links {
		out = strings.Replace(out, placeholders[i], resolveInline(link), 1)
	}
	return out, nil
}

func convertMarkdown(source string) (string, error) {
	var buf bytes.Buffer
	if err := markdown.Convert([]byte(source), &buf); err != nil {
		return "", fmt.Errorf("converting markdown: %w", err)
	}
	return buf.String(), nil
}
