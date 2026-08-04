package export

import (
	"bytes"
	"testing"

	"github.com/quillit/contentengine/filter"
	"github.com/quillit/contentengine/parse"
)

// tomExample is the worked example from docs/cli-spec.md §4.
const tomExample = `---
name: Tom the Innkeeper
tags: [npc, waterdeep]
---

# Tom the Innkeeper

Tom runs the Gilded Goose inn. He rarely speaks of [[characters/npcs/mary|Mary]].

:::secret
Tom is secretly a spy for the Crimson Hand.
:::

:::card motivation
Wants to buy back his family farm.
:::

:::card description
Round-faced, ale-stained apron, **booming laugh**.
Spouse: [[characters/npcs/mary|Mary]]
:::
`

var defaultVocab = []string{"motivation", "description", "history"}

func mustFilter(t *testing.T, src string, view filter.View) *filter.FilteredEntry {
	t.Helper()
	entry, err := parse.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse.Parse: %v", err)
	}
	f, err := filter.Filter(entry, view, defaultVocab)
	if err != nil {
		t.Fatalf("filter.Filter: %v", err)
	}
	return f
}

// captureSink records writeFragment's layout calls instead of drawing a
// PDF, so tests can assert on the walker's own logic (which text ends up
// where, in what order) without going anywhere near PDF byte encoding.
type captureSink struct {
	headings   []string
	paragraphs [][]run
	listItems  []capturedItem
}

type capturedItem struct {
	prefix string
	runs   []run
}

func (s *captureSink) heading(text string, size float64) { s.headings = append(s.headings, text) }
func (s *captureSink) paragraph(runs []run)              { s.paragraphs = append(s.paragraphs, runs) }
func (s *captureSink) listItem(prefix string, runs []run) {
	s.listItems = append(s.listItems, capturedItem{prefix, runs})
}
func (s *captureSink) endList() {}

func (s *captureSink) allText() string {
	var b bytes.Buffer
	for _, h := range s.headings {
		b.WriteString(h)
		b.WriteByte('\n')
	}
	for _, p := range s.paragraphs {
		for _, r := range p {
			b.WriteString(r.text)
		}
		b.WriteByte('\n')
	}
	for _, li := range s.listItems {
		b.WriteString(li.prefix)
		for _, r := range li.runs {
			b.WriteString(r.text)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// captureFragment runs the real renderFragment (same helper Bundle uses)
// for each entry, then walks the result into a captureSink instead of a
// PDF — exercising the real render path while keeping assertions at the
// text/structure level.
func captureFragment(t *testing.T, entries []*filter.FilteredEntry) *captureSink {
	t.Helper()
	sink := &captureSink{}
	for _, e := range entries {
		fragment, err := renderFragment(e)
		if err != nil {
			t.Fatalf("renderFragment: %v", err)
		}
		if err := writeFragment(sink, fragment); err != nil {
			t.Fatalf("writeFragment: %v", err)
		}
	}
	return sink
}

func TestBundle_ValidPDF(t *testing.T) {
	f := mustFilter(t, tomExample, filter.View{Kind: filter.ViewDM})
	data, err := Bundle([]*filter.FilteredEntry{f})
	if err != nil {
		t.Fatalf("Bundle: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Fatalf("output doesn't look like a PDF, first bytes: %q", data[:min(20, len(data))])
	}
	if len(data) < 1000 {
		t.Errorf("suspiciously small PDF: %d bytes", len(data))
	}
}

func TestBundle_EmptyEntries_Errors(t *testing.T) {
	if _, err := Bundle(nil); err == nil {
		t.Fatal("expected an error for zero entries")
	}
}

func TestWriteFragment_PlayerView_NoSecretText(t *testing.T) {
	f := mustFilter(t, tomExample, filter.View{Kind: filter.ViewPlayer})
	sink := captureFragment(t, []*filter.FilteredEntry{f})
	text := sink.allText()
	if bytesContainsStr(text, "Crimson Hand") {
		t.Fatalf("player-view output leaked secret text: %q", text)
	}
	if !bytesContainsStr(text, "Gilded Goose") {
		t.Errorf("player-view output should still contain the opening prose: %q", text)
	}
}

func TestWriteFragment_LinksAsPlainText(t *testing.T) {
	f := mustFilter(t, tomExample, filter.View{Kind: filter.ViewDM})
	sink := captureFragment(t, []*filter.FilteredEntry{f})
	text := sink.allText()
	if !bytesContainsStr(text, "Mary") {
		t.Errorf("expected the link label \"Mary\" as plain text: %q", text)
	}
	if bytesContainsStr(text, "quillit render") {
		t.Error("PDF layout must never contain a clipboard-link command — PDFs are static, CLI spec §4")
	}
}

func TestWriteFragment_CardView_DescriptionOnly(t *testing.T) {
	f := mustFilter(t, tomExample, filter.View{Kind: filter.ViewCard, Facet: "description"})
	sink := captureFragment(t, []*filter.FilteredEntry{f})
	text := sink.allText()
	if !bytesContainsStr(text, "Round-faced") {
		t.Errorf("expected the description card's content: %q", text)
	}
	if bytesContainsStr(text, "buy back his family farm") {
		t.Errorf("card view should not include the motivation card: %q", text)
	}
}

func TestWriteFragment_NeverExpandsCards(t *testing.T) {
	// A card view that *would* expand Mary's card in the browser (#26)
	// must not expand here — Bundle always renders with resolver=nil.
	f := mustFilter(t, tomExample, filter.View{Kind: filter.ViewCard, Facet: "description"})
	sink := captureFragment(t, []*filter.FilteredEntry{f})
	text := sink.allText()
	if bytesContainsStr(text, "Sharp-eyed shopkeeper") {
		t.Error("PDF export must never expand card links, even in a card view")
	}
}

func TestWriteFragment_MultipleEntries_BothPresent(t *testing.T) {
	main := mustFilter(t, tomExample, filter.View{Kind: filter.ViewPlayer})
	other, err := parse.Parse([]byte("---\nname: Mary\ntags: []\n---\n:::card description\nSharp-eyed shopkeeper.\n:::\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	otherFiltered, err := filter.Filter(other, filter.View{Kind: filter.ViewPlayer}, defaultVocab)
	if err != nil {
		t.Fatalf("filter: %v", err)
	}

	sink := captureFragment(t, []*filter.FilteredEntry{main, otherFiltered})
	text := sink.allText()
	if !bytesContainsStr(text, "Gilded Goose") {
		t.Error("missing main entry content")
	}
	if !bytesContainsStr(text, "Sharp-eyed shopkeeper") {
		t.Error("missing bundled entry content")
	}
	if bytesContainsStr(text, "Crimson Hand") {
		t.Error("bundled export leaked secret text")
	}
}

func TestWriteFragment_BoldRunPreserved(t *testing.T) {
	f := mustFilter(t, tomExample, filter.View{Kind: filter.ViewCard, Facet: "description"})
	sink := captureFragment(t, []*filter.FilteredEntry{f})

	var found bool
	for _, p := range sink.paragraphs {
		for _, r := range p {
			if r.text == "booming laugh" && r.bold {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected a bold run with text \"booming laugh\", got paragraphs: %+v", sink.paragraphs)
	}
}

func TestWriteFragment_Heading(t *testing.T) {
	// tomExample carries the title twice — once via frontmatter `name:`
	// (render.Render's own <h1>) and once via the body's own "# Tom the
	// Innkeeper" markdown heading (goldmark's <h1>) — so two headings is
	// correct here, not a bug; both must still say the right thing.
	f := mustFilter(t, tomExample, filter.View{Kind: filter.ViewDM})
	sink := captureFragment(t, []*filter.FilteredEntry{f})
	if len(sink.headings) != 2 {
		t.Fatalf("headings = %v, want 2 (frontmatter name + body heading)", sink.headings)
	}
	for _, h := range sink.headings {
		if h != "Tom the Innkeeper" {
			t.Errorf("heading = %q, want \"Tom the Innkeeper\"", h)
		}
	}
}

func bytesContainsStr(haystack, needle string) bool {
	return bytes.Contains([]byte(haystack), []byte(needle))
}
