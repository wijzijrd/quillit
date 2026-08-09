package migrate

import (
	"strings"

	"github.com/quillit/contentengine/parse"
	"gopkg.in/yaml.v3"
)

// Annotation is one row from the legacy `annotations` table.
type Annotation struct {
	ID   string
	Text string
}

// EntryInput is everything ConvertEntry needs from one legacy entry to
// produce its CLI-aligned annotated markdown body. Path assignment (slug,
// directory, project) is a whole-batch concern (cross-entry dedup) and
// happens before this call, in the batch orchestrator — not here.
type EntryInput struct {
	ID              string
	Title           string
	Tags            []string
	BodyHTML        string
	Facet           string // kebab-cased category, used as the quick-view card's facet
	QuickViewFields map[string]string
	GMAnnotations   []Annotation
}

// ConvertEntry converts one legacy entry to annotated markdown: frontmatter,
// converted body, an optional quick-view card block, and a trailing
// "## Secrets" section for any GM annotation row not already folded in place
// by a matching mark (spec §5). Returns the markdown plus any flags raised
// during HTML conversion (unconvertible fragments, dangling mentions) —
// never drops content silently.
func ConvertEntry(input EntryInput, resolveMention resolveMentionFunc) (markdown string, flags []string) {
	annotationText := make(map[string]string, len(input.GMAnnotations))
	for _, a := range input.GMAnnotations {
		annotationText[a.ID] = a.Text
	}

	htmlResult := ConvertHTMLWithAnnotations(input.BodyHTML, resolveMention, annotationText)

	var sections []string
	if strings.TrimSpace(htmlResult.Markdown) != "" {
		sections = append(sections, htmlResult.Markdown)
	}

	if card := QuickViewCard(input.Facet, input.QuickViewFields); card != "" {
		sections = append(sections, card)
	}

	if secrets := trailingSecrets(input.GMAnnotations, htmlResult.ConsumedAnnotationIDs); secrets != "" {
		sections = append(sections, secrets)
	}

	fm, err := yaml.Marshal(parse.Frontmatter{Name: input.Title, Tags: input.Tags})
	if err != nil {
		// yaml.Marshal on a plain struct of string/[]string cannot fail;
		// treat as effectively unreachable rather than partially handling it.
		panic(err)
	}

	body := "---\n" + string(fm) + "---"
	if len(sections) > 0 {
		body += "\n\n" + strings.Join(sections, "\n\n")
	}
	return body, htmlResult.Flags
}

// trailingSecrets appends a "## Secrets" section holding one :::secret block
// per GM annotation row not already consumed by an in-place mark — never
// dropping a row just because it had no matching mark in the body.
func trailingSecrets(annotations []Annotation, consumed []string) string {
	consumedSet := make(map[string]bool, len(consumed))
	for _, id := range consumed {
		consumedSet[id] = true
	}

	var blocks []string
	for _, a := range annotations {
		if consumedSet[a.ID] {
			continue
		}
		blocks = append(blocks, ":::secret\n"+a.Text+"\n:::")
	}
	if len(blocks) == 0 {
		return ""
	}
	return "## Secrets\n\n" + strings.Join(blocks, "\n\n")
}
