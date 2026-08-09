package migrate

import (
	"strings"
	"testing"

	"github.com/quillit/contentengine/acceptance"
)

func TestConvertEntry_PrependsFrontmatter(t *testing.T) {
	input := EntryInput{Title: "Mary the Innkeeper", Tags: []string{"npc", "tavern"}, BodyHTML: "<p>Hello</p>"}
	md, _ := ConvertEntry(input, noMentions)

	if !strings.HasPrefix(md, "---\n") {
		t.Fatalf("expected markdown to start with a frontmatter fence, got:\n%s", md)
	}
	if !strings.Contains(md, "name: Mary the Innkeeper") {
		t.Errorf("expected frontmatter to contain the entry title as name, got:\n%s", md)
	}
	if !strings.Contains(md, "Hello") {
		t.Errorf("expected converted body to be present, got:\n%s", md)
	}
}

func TestConvertEntry_AppendsQuickViewCard(t *testing.T) {
	input := EntryInput{
		Title:           "Mary",
		BodyHTML:        "<p>An innkeeper.</p>",
		Facet:           "characters",
		QuickViewFields: map[string]string{"Role": "innkeeper"},
	}
	md, _ := ConvertEntry(input, noMentions)
	if !strings.Contains(md, ":::card characters\n**Role:** innkeeper\n:::") {
		t.Errorf("expected a card block for the quick-view fields, got:\n%s", md)
	}
}

func TestConvertEntry_AppendsUnconsumedAnnotationsAsTrailingSecrets(t *testing.T) {
	input := EntryInput{
		Title:    "Mary",
		BodyHTML: "<p>An innkeeper.</p>",
		GMAnnotations: []Annotation{
			{ID: "a1", Text: "Secretly a dragon."},
		},
	}
	md, _ := ConvertEntry(input, noMentions)
	if !strings.Contains(md, "## Secrets") {
		t.Errorf("expected a trailing Secrets section, got:\n%s", md)
	}
	if !strings.Contains(md, ":::secret\nSecretly a dragon.\n:::") {
		t.Errorf("expected the unconsumed annotation as a secret block, got:\n%s", md)
	}
}

func TestConvertEntry_DoesNotDuplicateAnnotationConsumedByMark(t *testing.T) {
	input := EntryInput{
		Title:    "Mary",
		BodyHTML: `<p>Mary is <mark data-annotation-id="a1" data-visibility="gm" class="annotation-mark annotation-mark--gm">a dragon</mark>.</p>`,
		GMAnnotations: []Annotation{
			{ID: "a1", Text: "Secretly a dragon."},
		},
	}
	md, _ := ConvertEntry(input, noMentions)
	if strings.Contains(md, "## Secrets") {
		t.Errorf("expected no trailing Secrets section when the only annotation was consumed by a mark, got:\n%s", md)
	}
	if strings.Count(md, "Secretly a dragon.") != 1 {
		t.Errorf("expected the annotation text to appear exactly once, got:\n%s", md)
	}
}

func TestConvertEntry_ProducesValidParseableOutput(t *testing.T) {
	input := EntryInput{
		Title:           "Mary the Innkeeper",
		Tags:            []string{"npc"},
		BodyHTML:        `<h2>Background</h2><p>Mary runs <a href="https://example.com">the tavern</a>. She is <mark data-annotation-id="a1" data-visibility="gm" class="annotation-mark annotation-mark--gm">a dragon</mark>.</p>`,
		Facet:           "characters",
		QuickViewFields: map[string]string{"Role": "innkeeper"},
		GMAnnotations:   []Annotation{{ID: "a1", Text: "Secretly a dragon."}},
	}
	md, flags := ConvertEntry(input, noMentions)
	if len(flags) != 0 {
		t.Fatalf("expected no flags for a fully convertible entry, got %v", flags)
	}

	results := acceptance.Validate(map[string][]byte{"characters/mary": []byte(md)}, []string{"characters"})
	if len(results) != 1 {
		t.Fatalf("expected 1 validation result, got %d", len(results))
	}
	if results[0].Err != nil {
		t.Errorf("expected converted output to parse clean, got error: %v\n\nmarkdown:\n%s", results[0].Err, md)
	}
}
