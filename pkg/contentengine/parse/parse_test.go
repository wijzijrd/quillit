package parse

import (
	"reflect"
	"strings"
	"testing"
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
Round-faced, ale-stained apron, booming laugh.
Spouse: [[characters/npcs/mary|Mary]]
:::
`

func TestParse_TomExample(t *testing.T) {
	e, err := Parse([]byte(tomExample))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if e.Frontmatter.Name != "Tom the Innkeeper" {
		t.Errorf("Frontmatter.Name = %q, want %q", e.Frontmatter.Name, "Tom the Innkeeper")
	}
	wantTags := []string{"npc", "waterdeep"}
	if !reflect.DeepEqual(e.Frontmatter.Tags, wantTags) {
		t.Errorf("Frontmatter.Tags = %v, want %v", e.Frontmatter.Tags, wantTags)
	}

	var prose, secrets, cards []Block
	for _, b := range e.Body {
		switch b.Kind {
		case BlockProse:
			prose = append(prose, b)
		case BlockSecret:
			secrets = append(secrets, b)
		case BlockCard:
			cards = append(cards, b)
		}
	}

	if len(prose) != 1 {
		t.Fatalf("got %d prose blocks, want 1", len(prose))
	}
	if !strings.Contains(prose[0].Content, "Tom runs the Gilded Goose inn") {
		t.Errorf("prose block missing expected opening paragraph, got: %q", prose[0].Content)
	}

	if len(secrets) != 1 {
		t.Fatalf("got %d secret blocks, want 1", len(secrets))
	}
	if !strings.Contains(secrets[0].Content, "Tom is secretly a spy for the Crimson Hand") {
		t.Errorf("secret block content = %q, want it to contain the spy sentence", secrets[0].Content)
	}

	if len(cards) != 2 {
		t.Fatalf("got %d card blocks, want 2 (motivation, description)", len(cards))
	}
	byFacet := map[string]Block{}
	for _, c := range cards {
		byFacet[c.Facet] = c
	}
	motivation, ok := byFacet["motivation"]
	if !ok {
		t.Fatal("no card block with facet \"motivation\"")
	}
	if !strings.Contains(motivation.Content, "buy back his family farm") {
		t.Errorf("motivation card content = %q", motivation.Content)
	}

	description, ok := byFacet["description"]
	if !ok {
		t.Fatal("no card block with facet \"description\"")
	}
	if !strings.Contains(description.Content, "Round-faced") {
		t.Errorf("description card content = %q", description.Content)
	}
	if len(description.Links) != 1 {
		t.Fatalf("description card: got %d links, want 1", len(description.Links))
	}
	if description.Links[0].Path != "characters/npcs/mary" {
		t.Errorf("description card link Path = %q, want characters/npcs/mary", description.Links[0].Path)
	}
	if description.Links[0].Label != "Mary" {
		t.Errorf("description card link Label = %q, want Mary", description.Links[0].Label)
	}
}

func TestParse_NoDirectives(t *testing.T) {
	e, err := Parse([]byte("---\nname: Plain\n---\n\nJust some prose, nothing fancy.\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(e.Body) != 1 || e.Body[0].Kind != BlockProse {
		t.Fatalf("got %+v, want a single prose block", e.Body)
	}
	if !strings.Contains(e.Body[0].Content, "Just some prose") {
		t.Errorf("prose content = %q", e.Body[0].Content)
	}
}

func TestParse_OnlySecrets(t *testing.T) {
	e, err := Parse([]byte(":::secret\nHidden.\n:::\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(e.Body) != 1 || e.Body[0].Kind != BlockSecret {
		t.Fatalf("got %+v, want a single secret block", e.Body)
	}
	if e.Body[0].Content != "Hidden." {
		t.Errorf("secret content = %q, want %q", e.Body[0].Content, "Hidden.")
	}
}

func TestParse_OnlyCards(t *testing.T) {
	e, err := Parse([]byte(":::card history\nOnce upon a time.\n:::\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(e.Body) != 1 || e.Body[0].Kind != BlockCard {
		t.Fatalf("got %+v, want a single card block", e.Body)
	}
	if e.Body[0].Facet != "history" {
		t.Errorf("Facet = %q, want history", e.Body[0].Facet)
	}
}

func TestParse_SecretsAndCards(t *testing.T) {
	src := ":::secret\nS1\n:::\n\nProse between.\n\n:::card loot\nC1\n:::\n"
	e, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(e.Body) != 3 {
		t.Fatalf("got %d blocks, want 3 (secret, prose, card): %+v", len(e.Body), e.Body)
	}
	if e.Body[0].Kind != BlockSecret || e.Body[1].Kind != BlockProse || e.Body[2].Kind != BlockCard {
		t.Fatalf("block order/kinds = %v, %v, %v", e.Body[0].Kind, e.Body[1].Kind, e.Body[2].Kind)
	}
}

func TestParse_CardNestedInSecret(t *testing.T) {
	src := `:::secret
Only the DM should know this much.

:::card motivation
Nested motivation card, DM-only, still a valid card view target.
:::
:::
`
	e, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(e.Body) != 1 || e.Body[0].Kind != BlockSecret {
		t.Fatalf("got %+v, want a single top-level secret block", e.Body)
	}
	secret := e.Body[0]
	if !strings.Contains(secret.Content, "Only the DM should know this much") {
		t.Errorf("secret direct content = %q", secret.Content)
	}
	if strings.Contains(secret.Content, "Nested motivation card") {
		t.Errorf("secret direct content should not duplicate the nested card's raw text, got %q", secret.Content)
	}
	if len(secret.Blocks) != 1 || secret.Blocks[0].Kind != BlockCard {
		t.Fatalf("secret.Blocks = %+v, want a single nested card block", secret.Blocks)
	}
	if secret.Blocks[0].Facet != "motivation" {
		t.Errorf("nested card Facet = %q, want motivation", secret.Blocks[0].Facet)
	}
	if !strings.Contains(secret.Blocks[0].Content, "Nested motivation card") {
		t.Errorf("nested card content = %q", secret.Blocks[0].Content)
	}
}

func TestParse_MalformedUnclosedSecret(t *testing.T) {
	_, err := Parse([]byte(":::secret\nNever closed.\n"))
	if err == nil {
		t.Fatal("expected error for unclosed :::secret block")
	}
	if _, ok := err.(MalformedDirective); !ok {
		t.Errorf("expected MalformedDirective, got %T: %v", err, err)
	}
}

func TestParse_MalformedUnclosedCard(t *testing.T) {
	_, err := Parse([]byte(":::card loot\nNever closed.\n"))
	if err == nil {
		t.Fatal("expected error for unclosed :::card block")
	}
	if _, ok := err.(MalformedDirective); !ok {
		t.Errorf("expected MalformedDirective, got %T: %v", err, err)
	}
}

func TestParse_MalformedStrayCloseFence(t *testing.T) {
	_, err := Parse([]byte("Some prose.\n:::\nMore prose.\n"))
	if err == nil {
		t.Fatal("expected error for a stray closing ::: with nothing open")
	}
	if _, ok := err.(MalformedDirective); !ok {
		t.Errorf("expected MalformedDirective, got %T: %v", err, err)
	}
}

func TestParse_WikilinkWithoutLabel(t *testing.T) {
	e, err := Parse([]byte("See [[characters/npcs/mary]] for details.\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(e.Body) != 1 || len(e.Body[0].Links) != 1 {
		t.Fatalf("got %+v", e.Body)
	}
	link := e.Body[0].Links[0]
	if link.Path != "characters/npcs/mary" || link.Label != "characters/npcs/mary" {
		t.Errorf("link = %+v, want Path==Label==characters/npcs/mary", link)
	}
}

func TestParse_NoFrontmatter(t *testing.T) {
	e, err := Parse([]byte("Just prose, no frontmatter at all.\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if e.Frontmatter.Name != "" || e.Frontmatter.Tags != nil {
		t.Errorf("Frontmatter = %+v, want zero value", e.Frontmatter)
	}
	if len(e.Body) != 1 || !strings.Contains(e.Body[0].Content, "Just prose") {
		t.Errorf("Body = %+v", e.Body)
	}
}
