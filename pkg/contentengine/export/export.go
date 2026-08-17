// Package export renders one or more already-filtered entries to a
// single PDF, per docs/cli-spec.md §7 "export". It reuses render.Render
// to get the exact same view-filtered content the browser render/#26
// produces, then walks that HTML — a small, entirely self-produced tag
// vocabulary (headings, paragraphs, emphasis, lists, the card-block/
// secret-block wrapper divs) — into PDF drawing calls via a pure-Go PDF
// library. No system toolchain (wkhtmltopdf, headless Chrome, ...) is
// shelled out to, and the font is embedded in the binary (the Go font
// family, via golang.org/x/image), so this works unmodified inside a
// single static CLI binary and inside a container.
//
// PDFs never expand wikilinks or show interactive affordances, per CLI
// spec §4: "Links are DM-render-only... PDF export show[s] link's plain
// text label only" — this holds structurally, not by convention: Bundle
// never passes a Resolver to render.Render, so render's depth-1 card
// expansion can never trigger regardless of which view produced the
// entries, and the LinkRenderer used here always returns the plain
// escaped label with no wrapping markup at all.
package export

import (
	"bytes"
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/go-pdf/fpdf"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/goregular"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/quillit/contentengine/filter"
	"github.com/quillit/contentengine/render"
)

const fontFamily = "go"

// Bundle renders one or more already-filtered entries into a single
// combined PDF, in document order — the main entry first, followed by
// any bundled linked entries (CLI spec §7 "export" --with-links). Each
// entry starts on its own page. entries must be non-empty and should
// already reflect the caller's desired view (filter.Filter having been
// applied per entry) — Bundle itself doesn't know or care which view
// produced them, since PDF presentation (no expansion, no clipboard
// links) is identical regardless.
func Bundle(entries []*filter.FilteredEntry) ([]byte, error) {
	if len(entries) == 0 {
		return nil, fmt.Errorf("export.Bundle: no entries given")
	}

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.AddUTF8FontFromBytes(fontFamily, "", goregular.TTF)
	pdf.AddUTF8FontFromBytes(fontFamily, "B", gobold.TTF)
	pdf.AddUTF8FontFromBytes(fontFamily, "I", goitalic.TTF)
	pdf.AddUTF8FontFromBytes(fontFamily, "BI", gobolditalic.TTF)
	pdf.SetMargins(20, 20, 20)
	pdf.SetAutoPageBreak(true, 20)

	sink := fpdfSink{pdf: pdf}
	for _, entry := range entries {
		fragment, err := renderFragment(entry)
		if err != nil {
			return nil, err
		}
		pdf.AddPage()
		if err := writeFragment(sink, fragment); err != nil {
			return nil, err
		}
	}

	if err := pdf.Error(); err != nil {
		return nil, fmt.Errorf("generating PDF: %w", err)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("generating PDF: %w", err)
	}
	return buf.Bytes(), nil
}

// renderFragment produces the HTML fragment for one entry, always with
// resolver=nil and plainLabelRenderer — the two structural guarantees
// that no wikilink ever expands or gets interactive markup in PDF
// output, regardless of which view produced entry.
func renderFragment(entry *filter.FilteredEntry) (string, error) {
	fragment, err := render.Render(entry, filter.View{}, nil, plainLabelRenderer, nil)
	if err != nil {
		return "", fmt.Errorf("rendering entry for PDF: %w", err)
	}
	return fragment, nil
}

// plainLabelRenderer is the render.LinkRenderer used for every PDF
// export: no markup, no command, just the label — render.LinkInfo's
// Facet/NoMatchingCard are irrelevant here since there's no interactive
// surface to annotate.
func plainLabelRenderer(info render.LinkInfo) string {
	return html.EscapeString(info.Label)
}

// run is one contiguous span of text sharing the same bold/italic state,
// in document order — the flattened form of nested <strong>/<em>
// elements a sink lays out one call at a time.
type run struct {
	text         string
	bold, italic bool
}

func (r run) style() string {
	s := ""
	if r.bold {
		s += "B"
	}
	if r.italic {
		s += "I"
	}
	return s
}

// sink receives layout instructions from the HTML walker below. This
// indirection exists so the walker's own correctness — which text ends
// up in which block, in what order — is testable independently of
// fpdf's internal PDF byte encoding (an embedded TrueType font stores
// glyph indices in the content stream, not literal characters, so
// searching real PDF output for text isn't a meaningful test of the
// walker itself). fpdfSink is the real, PDF-producing implementation;
// export_test.go uses a capturing one.
type sink interface {
	heading(text string, size float64)
	paragraph(runs []run)
	listItem(prefix string, runs []run)
	endList()
}

type fpdfSink struct {
	pdf *fpdf.Fpdf
}

func (s fpdfSink) heading(text string, size float64) {
	s.pdf.SetFont(fontFamily, "B", size)
	s.pdf.MultiCell(0, size*0.6, text, "", "", false)
	s.pdf.Ln(2)
}

func (s fpdfSink) paragraph(runs []run) {
	s.writeRuns(runs)
	s.pdf.Ln(8)
}

func (s fpdfSink) listItem(prefix string, runs []run) {
	s.pdf.SetFont(fontFamily, "", 11)
	s.pdf.Write(6, prefix)
	s.writeRuns(runs)
	s.pdf.Ln(6)
}

func (s fpdfSink) endList() {
	s.pdf.Ln(2)
}

// writeRuns lays out a sequence of styled text runs on the current line,
// wrapping as needed — fpdf.Write is designed for exactly this: repeated
// calls continue from the current position, switching font style between
// calls, until Ln() starts a new line.
func (s fpdfSink) writeRuns(runs []run) {
	for _, r := range runs {
		s.pdf.SetFont(fontFamily, r.style(), 11)
		s.pdf.Write(6, r.text)
	}
}

// writeFragment parses a render.Render HTML fragment and walks it into s.
func writeFragment(s sink, fragment string) error {
	context := &xhtml.Node{Type: xhtml.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := xhtml.ParseFragment(strings.NewReader(fragment), context)
	if err != nil {
		return fmt.Errorf("parsing rendered fragment for PDF layout: %w", err)
	}
	for _, n := range nodes {
		writeBlock(s, n)
	}
	return nil
}

// writeBlock lays out one block-level node. Unrecognized elements (there
// shouldn't be any, since the fragment is entirely self-produced by
// render.Render) are treated as transparent containers: their children
// are laid out, nothing is lost.
func writeBlock(s sink, n *xhtml.Node) {
	if n.Type != xhtml.ElementNode {
		return
	}

	switch n.Data {
	case "h1":
		s.heading(textContent(n), 18)
	case "h2":
		s.heading(textContent(n), 15)
	case "h3":
		s.heading(textContent(n), 13)
	case "p":
		s.paragraph(collectRuns(n, false, false))
	case "ul":
		writeList(s, n, false)
	case "ol":
		writeList(s, n, true)
	default: // div (card-block/secret-block wrappers), or anything else: transparent container
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			writeBlock(s, c)
		}
	}
}

func writeList(s sink, n *xhtml.Node, ordered bool) {
	i := 1
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != xhtml.ElementNode || c.Data != "li" {
			continue
		}
		prefix := "•  "
		if ordered {
			prefix = strconv.Itoa(i) + ".  "
			i++
		}
		s.listItem(prefix, collectRuns(c, false, false))
	}
	s.endList()
}

// collectRuns walks n's children (text possibly nested inside <strong>/
// <b>/<em>/<i>) into a flat sequence of styled runs, in order.
func collectRuns(n *xhtml.Node, bold, italic bool) []run {
	var runs []run
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		switch c.Type {
		case xhtml.TextNode:
			if c.Data != "" {
				runs = append(runs, run{text: c.Data, bold: bold, italic: italic})
			}
		case xhtml.ElementNode:
			switch c.Data {
			case "strong", "b":
				runs = append(runs, collectRuns(c, true, italic)...)
			case "em", "i":
				runs = append(runs, collectRuns(c, bold, true)...)
			default:
				runs = append(runs, collectRuns(c, bold, italic)...)
			}
		}
	}
	return runs
}

// textContent flattens n's entire text content, ignoring any inline
// styling — used for headings, which render.Render only ever produces
// as plain text (the entry name).
func textContent(n *xhtml.Node) string {
	var b strings.Builder
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}
