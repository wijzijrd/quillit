package migrate

import "testing"

func noMentions(id string) (string, bool) { return "", false }

func TestConvertHTML_Paragraph(t *testing.T) {
	got := ConvertHTML(`<p>Hello world</p>`, noMentions)
	if got.Markdown != "Hello world" {
		t.Errorf("Markdown = %q, want %q", got.Markdown, "Hello world")
	}
	if len(got.Flags) != 0 {
		t.Errorf("expected no flags, got %v", got.Flags)
	}
}

func TestConvertHTML_Heading(t *testing.T) {
	got := ConvertHTML(`<h2>Section Title</h2>`, noMentions)
	if got.Markdown != "## Section Title" {
		t.Errorf("Markdown = %q, want %q", got.Markdown, "## Section Title")
	}
}

func TestConvertHTML_BoldAndItalic(t *testing.T) {
	got := ConvertHTML(`<p><strong>bold</strong> and <em>italic</em></p>`, noMentions)
	want := "**bold** and *italic*"
	if got.Markdown != want {
		t.Errorf("Markdown = %q, want %q", got.Markdown, want)
	}
}

func TestConvertHTML_Link(t *testing.T) {
	got := ConvertHTML(`<p><a href="https://example.com">example</a></p>`, noMentions)
	want := "[example](https://example.com)"
	if got.Markdown != want {
		t.Errorf("Markdown = %q, want %q", got.Markdown, want)
	}
}

func TestConvertHTML_Image(t *testing.T) {
	got := ConvertHTML(`<img src="https://minio/x.png" alt="a map">`, noMentions)
	want := "![a map](https://minio/x.png)"
	if got.Markdown != want {
		t.Errorf("Markdown = %q, want %q", got.Markdown, want)
	}
}

func TestConvertHTML_HorizontalRule(t *testing.T) {
	got := ConvertHTML(`<p>a</p><hr><p>b</p>`, noMentions)
	want := "a\n\n---\n\nb"
	if got.Markdown != want {
		t.Errorf("Markdown = %q, want %q", got.Markdown, want)
	}
}

func TestConvertHTML_UnorderedList(t *testing.T) {
	got := ConvertHTML(`<ul><li>one</li><li>two</li></ul>`, noMentions)
	want := "- one\n- two"
	if got.Markdown != want {
		t.Errorf("Markdown = %q, want %q", got.Markdown, want)
	}
}

func TestConvertHTML_OrderedList(t *testing.T) {
	got := ConvertHTML(`<ol><li>one</li><li>two</li></ol>`, noMentions)
	want := "1. one\n2. two"
	if got.Markdown != want {
		t.Errorf("Markdown = %q, want %q", got.Markdown, want)
	}
}

func TestConvertHTML_GMAnnotationMarkBecomesSecretBlock(t *testing.T) {
	html := `<p>The innkeeper is <mark data-annotation-id="a1" data-visibility="gm" class="annotation-mark annotation-mark--gm">secretly a vampire</mark>.</p>`
	got := ConvertHTML(html, noMentions)
	want := "The innkeeper is\n\n:::secret\nsecretly a vampire\n:::\n\n."
	if got.Markdown != want {
		t.Errorf("Markdown =\n%q\nwant\n%q", got.Markdown, want)
	}
}

func TestConvertHTML_GMAnnotationMarkProducesNoFlags(t *testing.T) {
	html := `<p>The innkeeper is <mark data-annotation-id="a1" data-visibility="gm" class="annotation-mark annotation-mark--gm">secretly a vampire</mark>.</p>`
	got := ConvertHTML(html, noMentions)
	if len(got.Flags) != 0 {
		t.Errorf("expected a well-formed GM mark to produce no flags, got %v", got.Flags)
	}
}

func TestConvertHTML_GMMarkUsesMatchingAnnotationRowText(t *testing.T) {
	html := `<p>The innkeeper is <mark data-annotation-id="a1" data-visibility="gm" class="annotation-mark annotation-mark--gm">a vampire</mark>.</p>`
	res := ConvertHTMLWithAnnotations(html, noMentions, map[string]string{"a1": "Actually a vampire hunter in disguise."})
	want := "The innkeeper is\n\n:::secret\nActually a vampire hunter in disguise.\n:::\n\n."
	if res.Markdown != want {
		t.Errorf("Markdown =\n%q\nwant\n%q", res.Markdown, want)
	}
	if len(res.ConsumedAnnotationIDs) != 1 || res.ConsumedAnnotationIDs[0] != "a1" {
		t.Errorf("ConsumedAnnotationIDs = %v, want [a1]", res.ConsumedAnnotationIDs)
	}
}

func TestConvertHTML_GMMarkFallsBackToSpanTextWhenNoAnnotationRow(t *testing.T) {
	html := `<p><mark data-annotation-id="missing" data-visibility="gm" class="annotation-mark annotation-mark--gm">span text</mark></p>`
	res := ConvertHTMLWithAnnotations(html, noMentions, map[string]string{})
	want := ":::secret\nspan text\n:::"
	if res.Markdown != want {
		t.Errorf("Markdown = %q, want %q", res.Markdown, want)
	}
	if len(res.ConsumedAnnotationIDs) != 0 {
		t.Errorf("expected no consumed annotation ids when the row is missing, got %v", res.ConsumedAnnotationIDs)
	}
}

func TestConvertHTML_ResolvedMentionBecomesWikilink(t *testing.T) {
	resolve := func(id string) (string, bool) {
		if id == "e42" {
			return "characters/mary", true
		}
		return "", false
	}
	html := `<p>Talk to <span data-type="mention" class="entry-mention" data-id="e42">[[Mary]]</span> first.</p>`
	got := ConvertHTML(html, resolve)
	want := "Talk to [[characters/mary|Mary]] first."
	if got.Markdown != want {
		t.Errorf("Markdown = %q, want %q", got.Markdown, want)
	}
	if len(got.Flags) != 0 {
		t.Errorf("expected no flags for a resolved mention, got %v", got.Flags)
	}
}

func TestConvertHTML_UnresolvedMentionFlaggedAsDangling(t *testing.T) {
	html := `<p><span data-type="mention" class="entry-mention" data-id="ghost">[[Nobody]]</span></p>`
	got := ConvertHTML(html, noMentions)
	if got.Markdown != "[[Nobody]]" {
		t.Errorf("Markdown = %q, want %q", got.Markdown, "[[Nobody]]")
	}
	if len(got.Flags) != 1 {
		t.Fatalf("expected 1 flag for a dangling mention, got %v", got.Flags)
	}
}

func TestConvertHTML_UnknownTagPreservedAsRawPassthroughAndFlagged(t *testing.T) {
	html := `<table><tr><td>x</td></tr></table>`
	got := ConvertHTML(html, noMentions)
	if got.Markdown == "" {
		t.Fatal("expected unconvertible content to be preserved, got empty markdown")
	}
	if len(got.Flags) != 1 {
		t.Fatalf("expected 1 flag for an unconvertible tag, got %v", got.Flags)
	}
}
