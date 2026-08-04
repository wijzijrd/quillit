package filter

import (
	"strings"
	"testing"

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
Round-faced, ale-stained apron, booming laugh.
Spouse: [[characters/npcs/mary|Mary]]
:::
`

var defaultVocab = []string{"motivation", "description", "history"}

func mustParse(t *testing.T, src string) *parse.Entry {
	t.Helper()
	e, err := parse.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse.Parse: %v", err)
	}
	return e
}

func TestFilter_DMView_Unchanged(t *testing.T) {
	entry := mustParse(t, tomExample)
	f, err := Filter(entry, View{Kind: ViewDM}, defaultVocab)
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	if len(f.Blocks) != len(entry.Body) {
		t.Fatalf("DM view block count = %d, want %d (unchanged)", len(f.Blocks), len(entry.Body))
	}

	var sawSecret, sawMotivation, sawDescription bool
	for _, b := range f.Blocks {
		switch b.Kind {
		case parse.BlockSecret:
			sawSecret = true
		case parse.BlockCard:
			if b.Facet == "motivation" {
				sawMotivation = true
			}
			if b.Facet == "description" {
				sawDescription = true
			}
		}
	}
	if !sawSecret || !sawMotivation || !sawDescription {
		t.Errorf("DM view missing content: secret=%v motivation=%v description=%v", sawSecret, sawMotivation, sawDescription)
	}
}

func TestFilter_PlayerView_StripsSecrets(t *testing.T) {
	entry := mustParse(t, tomExample)
	f, err := Filter(entry, View{Kind: ViewPlayer}, defaultVocab)
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	for _, b := range f.Blocks {
		if b.Kind == parse.BlockSecret {
			t.Fatal("player view still contains a secret block")
		}
		if strings.Contains(b.Content, "Crimson Hand") {
			t.Fatal("player view content contains secret text")
		}
	}
	// Non-secret content (top-level cards, prose) survives player view
	// per CLI spec §4: "Everything except :::secret block."
	var sawMotivation bool
	for _, b := range f.Blocks {
		if b.Kind == parse.BlockCard && b.Facet == "motivation" {
			sawMotivation = true
		}
	}
	if !sawMotivation {
		t.Error("player view should still show non-secret top-level card blocks")
	}
}

func TestFilter_PlayerView_StripsNestedCardInSecret(t *testing.T) {
	src := `:::secret
DM only.

:::card motivation
Secret-nested card — must not leak into player view.
:::
:::
`
	entry := mustParse(t, src)
	f, err := Filter(entry, View{Kind: ViewPlayer}, defaultVocab)
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	if len(f.Blocks) != 0 {
		t.Fatalf("player view should have nothing left (the only content was inside a secret), got %+v", f.Blocks)
	}
}

func TestFilter_CardView_MotivationOnly(t *testing.T) {
	entry := mustParse(t, tomExample)
	f, err := Filter(entry, View{Kind: ViewCard, Facet: "motivation"}, defaultVocab)
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	if len(f.Blocks) != 1 {
		t.Fatalf("got %d blocks, want exactly 1 (motivation card): %+v", len(f.Blocks), f.Blocks)
	}
	if f.Blocks[0].Kind != parse.BlockCard || f.Blocks[0].Facet != "motivation" {
		t.Fatalf("block = %+v, want the motivation card", f.Blocks[0])
	}
	if !strings.Contains(f.Blocks[0].Content, "buy back his family farm") {
		t.Errorf("content = %q", f.Blocks[0].Content)
	}
	if f.Name != "Tom the Innkeeper" {
		t.Errorf("Name = %q, want frontmatter name preserved for the header", f.Name)
	}
}

func TestFilter_CardView_FindsNestedCard(t *testing.T) {
	src := `:::secret
Wrapper.

:::card motivation
Nested card — card view is a DM tool, secret-nested cards still reachable.
:::
:::
`
	entry := mustParse(t, src)
	f, err := Filter(entry, View{Kind: ViewCard, Facet: "motivation"}, defaultVocab)
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	if len(f.Blocks) != 1 {
		t.Fatalf("got %d blocks, want 1: %+v", len(f.Blocks), f.Blocks)
	}
	if !strings.Contains(f.Blocks[0].Content, "Nested card") {
		t.Errorf("content = %q", f.Blocks[0].Content)
	}
}

func TestFilter_UnknownRequestedFacet(t *testing.T) {
	entry := mustParse(t, tomExample)
	_, err := Filter(entry, View{Kind: ViewCard, Facet: "bogus-facet"}, defaultVocab)
	if err == nil {
		t.Fatal("expected UnknownFacet error")
	}
	uf, ok := err.(UnknownFacet)
	if !ok {
		t.Fatalf("expected UnknownFacet, got %T: %v", err, err)
	}
	if uf.Name != "bogus-facet" {
		t.Errorf("Name = %q, want bogus-facet", uf.Name)
	}
	if len(uf.Vocabulary) != len(defaultVocab) {
		t.Errorf("Vocabulary = %v, want %v", uf.Vocabulary, defaultVocab)
	}
}

func TestFilter_EntryReferencesUndeclaredFacet_ErrorsEvenInDMView(t *testing.T) {
	entry := mustParse(t, ":::card not-a-real-facet\ncontent\n:::\n")
	_, err := Filter(entry, View{Kind: ViewDM}, defaultVocab)
	if err == nil {
		t.Fatal("expected UnknownFacet error even for a plain DM-view render, per CLI spec §4: \".md containing undeclared facet → error\"")
	}
	if _, ok := err.(UnknownFacet); !ok {
		t.Fatalf("expected UnknownFacet, got %T: %v", err, err)
	}
}

func TestFilter_QuickView_TruncatesToFirstBlock(t *testing.T) {
	entry := mustParse(t, tomExample)
	f, err := Filter(entry, View{Kind: ViewDM, Quick: true}, defaultVocab)
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	if len(f.Blocks) != 1 {
		t.Fatalf("quick view got %d blocks, want 1", len(f.Blocks))
	}
	if f.Blocks[0].Kind != parse.BlockProse {
		t.Errorf("quick view's first block should be the opening prose, got Kind=%v", f.Blocks[0].Kind)
	}
}

func TestFilter_QuickView_ComposesWithCard(t *testing.T) {
	entry := mustParse(t, tomExample)
	f, err := Filter(entry, View{Kind: ViewCard, Facet: "motivation", Quick: true}, defaultVocab)
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	if len(f.Blocks) != 1 || f.Blocks[0].Facet != "motivation" {
		t.Fatalf("got %+v", f.Blocks)
	}
}
