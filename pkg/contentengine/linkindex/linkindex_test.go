package linkindex

import (
	"reflect"
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

func mustParse(t *testing.T, src string) *parse.Entry {
	t.Helper()
	e, err := parse.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse.Parse: %v", err)
	}
	return e
}

func TestExtract_TomExample(t *testing.T) {
	entry := mustParse(t, tomExample)
	records := Extract(entry)

	if len(records) != 2 {
		t.Fatalf("got %d records, want 2 (opening paragraph + description card): %+v", len(records), records)
	}

	prose := records[0]
	if prose.TargetPath != "characters/npcs/mary" || prose.Label != "Mary" {
		t.Errorf("prose link record = %+v", prose)
	}
	if prose.CardFacet != "" {
		t.Errorf("opening-paragraph link CardFacet = %q, want empty (not inside a card)", prose.CardFacet)
	}

	card := records[1]
	if card.CardFacet != "description" {
		t.Errorf("description-card link CardFacet = %q, want description", card.CardFacet)
	}
	if card.TargetPath != "characters/npcs/mary" {
		t.Errorf("card link TargetPath = %q", card.TargetPath)
	}
}

func TestExtract_LinkInCardNestedInSecret(t *testing.T) {
	src := `:::secret
Wrapper.

:::card motivation
Nested card linking to [[locations/gilded-goose]].
:::
:::
`
	entry := mustParse(t, src)
	records := Extract(entry)

	if len(records) != 1 {
		t.Fatalf("got %d records, want 1: %+v", len(records), records)
	}
	if records[0].TargetPath != "locations/gilded-goose" {
		t.Errorf("TargetPath = %q", records[0].TargetPath)
	}
	if records[0].CardFacet != "motivation" {
		t.Errorf("CardFacet = %q, want motivation even though the card is nested inside a secret", records[0].CardFacet)
	}
}

func TestExtract_LinkDirectlyInSecret_NoFacet(t *testing.T) {
	src := ":::secret\nSee [[characters/npcs/mary]].\n:::\n"
	entry := mustParse(t, src)
	records := Extract(entry)

	if len(records) != 1 {
		t.Fatalf("got %d records, want 1: %+v", len(records), records)
	}
	if records[0].CardFacet != "" {
		t.Errorf("CardFacet = %q, want empty — this link is in the secret's own prose, not inside a card", records[0].CardFacet)
	}
}

func TestExtract_NoLinks(t *testing.T) {
	entry := mustParse(t, "Just some plain prose, nothing linked.\n")
	records := Extract(entry)
	if len(records) != 0 {
		t.Errorf("got %d records, want 0: %+v", len(records), records)
	}
}

func TestEncodeDecode_RoundTrip(t *testing.T) {
	original := []Record{
		{TargetPath: "characters/npcs/mary", Label: "Mary", CardFacet: "description", Resolved: true},
		{TargetPath: "locations/nowhere", Label: "Nowhere", Resolved: false},
	}

	data, err := Encode(original)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if !reflect.DeepEqual(original, decoded) {
		t.Errorf("round-trip mismatch:\noriginal: %+v\ndecoded:  %+v", original, decoded)
	}
}

func TestEncode_NilRecords_ProducesEmptyArray(t *testing.T) {
	data, err := Encode(nil)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(decoded) != 0 {
		t.Errorf("got %d records, want 0", len(decoded))
	}
}
