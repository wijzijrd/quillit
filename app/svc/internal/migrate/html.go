package migrate

import (
	"fmt"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// ConvertResult is the outcome of converting one entry body's HTML to
// markdown: the converted markdown, plus a human-readable flag per fragment
// that could not be cleanly converted (dangling mentions, unrecognized
// tags) — never a silently dropped one (spec §5 point 8).
type ConvertResult struct {
	Markdown string
	Flags    []string
	// ConsumedAnnotationIDs lists the `annotations` row ids folded in-place
	// into a ":::secret" block via a matching data-annotation-id mark. The
	// caller should append any *un*consumed GM annotation row as a trailing
	// "Secrets" section block (spec §5 point 2/5) — never drop it.
	ConsumedAnnotationIDs []string
}

// resolveMention resolves a TipTap mention node's entry id to the target
// entry's "directory/slug" path. ok is false when the id doesn't resolve
// (deleted entry, or not yet slugged) — the mention is then left as a plain
// "[[label]]" and flagged as dangling.
type resolveMentionFunc func(id string) (path string, ok bool)

// ConvertHTML converts a legacy TipTap-rendered entry body to CLI-aligned
// annotated markdown (spec §5). Block-level elements become markdown blocks
// joined by blank lines; a GM annotation-mark span always becomes its own
// ":::secret" block, since the ::: directive format has no inline form.
// Anything not recognized is preserved as raw HTML passthrough and flagged,
// never dropped.
func ConvertHTML(htmlBody string, resolveMention resolveMentionFunc) ConvertResult {
	return ConvertHTMLWithAnnotations(htmlBody, resolveMention, nil)
}

// ConvertHTMLWithAnnotations is ConvertHTML, additionally folding each GM
// mark's matching `annotations` row text (keyed by data-annotation-id) into
// its ":::secret" block in place of the mark's own visible span text (spec
// §5 point 2). Rows in annotationText with no matching mark in the body are
// not this function's concern — the caller appends those as a trailing
// section, see ConsumedAnnotationIDs.
func ConvertHTMLWithAnnotations(htmlBody string, resolveMention resolveMentionFunc, annotationText map[string]string) ConvertResult {
	nodes, err := html.ParseFragment(strings.NewReader(htmlBody), &html.Node{
		Type:     html.ElementNode,
		Data:     "body",
		DataAtom: atom.Body,
	})
	c := &converter{resolveMention: resolveMention, annotationText: annotationText}
	if err != nil {
		c.flag(fmt.Sprintf("HTML parse error: %v", err))
		return ConvertResult{Markdown: htmlBody, Flags: c.flags}
	}

	var blocks []string
	for _, n := range nodes {
		blocks = append(blocks, c.blocksFor(n)...)
	}
	return ConvertResult{Markdown: strings.Join(blocks, "\n\n"), Flags: c.flags, ConsumedAnnotationIDs: c.consumed}
}

type converter struct {
	resolveMention resolveMentionFunc
	annotationText map[string]string
	flags          []string
	consumed       []string
}

// secretTextFor returns the markdown body for a GM mark: the matching
// annotations row's text when data-annotation-id resolves, else the mark's
// own visible span text.
func (c *converter) secretTextFor(n *html.Node) string {
	id := attr(n, "data-annotation-id")
	if id != "" {
		if text, ok := c.annotationText[id]; ok {
			c.consumed = append(c.consumed, id)
			return text
		}
	}
	return c.inlineChildren(n)
}

func (c *converter) flag(msg string) { c.flags = append(c.flags, msg) }

// blocksFor renders n (and, for text/inline nodes, its position within the
// surrounding flow) as zero or more markdown blocks.
func (c *converter) blocksFor(n *html.Node) []string {
	if n.Type == html.TextNode {
		if strings.TrimSpace(n.Data) == "" {
			return nil
		}
		return []string{n.Data}
	}
	if n.Type != html.ElementNode {
		return nil
	}

	switch n.DataAtom {
	case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
		return []string{strings.Repeat("#", headingLevel(n.DataAtom)) + " " + c.inlineChildren(n)}
	case atom.P, atom.Div:
		return c.mixedBlocks(n)
	case atom.Hr:
		return []string{"---"}
	case atom.Ul:
		return []string{c.list(n, false)}
	case atom.Ol:
		return []string{c.list(n, true)}
	case atom.Img:
		return []string{c.image(n)}
	case atom.Mark:
		if isGMMark(n) {
			return []string{":::secret\n" + c.secretTextFor(n) + "\n:::"}
		}
		return []string{c.inlineChildren(n)}
	case atom.Span:
		if id, isMention := mentionID(n); isMention {
			return []string{c.mention(n, id)}
		}
		return c.mixedBlocks(n)
	default:
		c.flag(fmt.Sprintf("unconvertible tag <%s> preserved as raw HTML passthrough", n.Data))
		return []string{renderNode(n)}
	}
}

// mixedBlocks handles a container (p, div, mention-bearing span) whose
// children may themselves contain block-breaking content (a GM mark, e.g.) —
// it splits on those, keeping surrounding inline text together.
func (c *converter) mixedBlocks(n *html.Node) []string {
	var blocks []string
	var inline strings.Builder

	flushInline := func() {
		if s := inline.String(); strings.TrimSpace(s) != "" {
			blocks = append(blocks, strings.TrimSpace(s))
		}
		inline.Reset()
	}

	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.DataAtom == atom.Mark && isGMMark(child) {
			flushInline()
			blocks = append(blocks, ":::secret\n"+c.secretTextFor(child)+"\n:::")
			continue
		}
		inline.WriteString(c.inlineText(child))
	}
	flushInline()
	return blocks
}

// inlineText renders n and its descendants as inline markdown (bold, italic,
// links, mentions, plain text) with no block breaks.
func (c *converter) inlineText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	if n.Type != html.ElementNode {
		return c.inlineChildren(n)
	}

	switch n.DataAtom {
	case atom.Strong, atom.B:
		return "**" + c.inlineChildren(n) + "**"
	case atom.Em, atom.I:
		return "*" + c.inlineChildren(n) + "*"
	case atom.A:
		return "[" + c.inlineChildren(n) + "](" + attr(n, "href") + ")"
	case atom.Img:
		return c.image(n)
	case atom.Mark:
		// A GM mark inside inline flow is promoted to a block by mixedBlocks
		// before inlineText ever sees it; a bare call here means it's nested
		// inside another inline element (e.g. <strong><mark>...) — flag it
		// rather than lose the secret boundary.
		c.flag("GM annotation mark nested inside another inline element; secret boundary may be imprecise")
		return c.inlineChildren(n)
	case atom.Span:
		if id, isMention := mentionID(n); isMention {
			return c.mention(n, id)
		}
		return c.inlineChildren(n)
	case atom.Br:
		return "\n"
	default:
		c.flag(fmt.Sprintf("unconvertible inline tag <%s> preserved as raw HTML passthrough", n.Data))
		return renderNode(n)
	}
}

func (c *converter) inlineChildren(n *html.Node) string {
	var b strings.Builder
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		b.WriteString(c.inlineText(child))
	}
	return b.String()
}

func (c *converter) list(n *html.Node, ordered bool) string {
	var lines []string
	i := 1
	for li := n.FirstChild; li != nil; li = li.NextSibling {
		if li.Type != html.ElementNode || li.DataAtom != atom.Li {
			continue
		}
		text := c.inlineChildren(li)
		if ordered {
			lines = append(lines, fmt.Sprintf("%d. %s", i, text))
			i++
		} else {
			lines = append(lines, "- "+text)
		}
	}
	return strings.Join(lines, "\n")
}

func (c *converter) image(n *html.Node) string {
	return "![" + attr(n, "alt") + "](" + attr(n, "src") + ")"
}

// mention resolves a TipTap mention span (rendered client-side as the
// literal text "[[label]]") to a path-addressed wikilink. Unresolved ids are
// left as the original literal text and flagged as dangling — never dropped.
func (c *converter) mention(n *html.Node, id string) string {
	label := strings.Trim(strings.TrimSpace(c.inlineChildren(n)), "[]")
	if c.resolveMention == nil {
		c.flag(fmt.Sprintf("dangling mention: entry id %q has no resolver configured", id))
		return "[[" + label + "]]"
	}
	path, ok := c.resolveMention(id)
	if !ok {
		c.flag(fmt.Sprintf("dangling mention: entry id %q does not resolve to a slugged entry", id))
		return "[[" + label + "]]"
	}
	return "[[" + path + "|" + label + "]]"
}

func headingLevel(a atom.Atom) int {
	switch a {
	case atom.H1:
		return 1
	case atom.H2:
		return 2
	case atom.H3:
		return 3
	case atom.H4:
		return 4
	case atom.H5:
		return 5
	default:
		return 6
	}
}

func isGMMark(n *html.Node) bool {
	return n.DataAtom == atom.Mark && attr(n, "data-visibility") == "gm"
}

// mentionID reports whether n is a TipTap mention node and its target entry id.
func mentionID(n *html.Node) (string, bool) {
	if n.DataAtom != atom.Span {
		return "", false
	}
	id := attr(n, "data-id")
	if id == "" {
		return "", false
	}
	if attr(n, "data-type") != "mention" && !strings.Contains(attr(n, "class"), "entry-mention") {
		return "", false
	}
	return id, true
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func renderNode(n *html.Node) string {
	var b strings.Builder
	if err := html.Render(&b, n); err != nil {
		return n.Data
	}
	return b.String()
}
