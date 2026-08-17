package render

import (
	"strings"
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
Round-faced, ale-stained apron, booming laugh.
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

// spyLinkRenderer records every LinkInfo it's called with, so tests can
// assert exactly which links (if any) went through the callback.
type spyLinkRenderer struct {
	calls []LinkInfo
}

func (s *spyLinkRenderer) render(info LinkInfo) string {
	s.calls = append(s.calls, info)
	return `<a data-quillit-spy-link href="` + info.Path + `">` + info.Label + `</a>`
}

// fakeResolver is a minimal in-memory Resolver for tests: path -> facet
// -> card content.
type fakeResolver map[string]map[string]string

func (r fakeResolver) Resolve(path string) (bool, func(string) (string, bool)) {
	facets, exists := r[path]
	if !exists {
		return false, nil
	}
	return true, func(facet string) (string, bool) {
		content, ok := facets[facet]
		return content, ok
	}
}

func TestRender_PlayerView_NoSecretLeakage(t *testing.T) {
	f := mustFilter(t, tomExample, filter.View{Kind: filter.ViewPlayer})
	spy := &spyLinkRenderer{}

	out, err := Render(f, filter.View{Kind: filter.ViewPlayer}, nil, spy.render, nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	if strings.Contains(out, "Crimson Hand") {
		t.Fatalf("player-view render leaked secret text, output: %s", out)
	}
	if strings.Contains(strings.ToLower(out), "secretly a spy") {
		t.Fatalf("player-view render leaked secret text, output: %s", out)
	}
	if len(spy.calls) != 0 {
		t.Errorf("player view invoked linkRenderer %d time(s), want 0 — links must render as plain text only", len(spy.calls))
	}
}

func TestRender_CardView_MotivationOnly(t *testing.T) {
	f := mustFilter(t, tomExample, filter.View{Kind: filter.ViewCard, Facet: "motivation"})
	spy := &spyLinkRenderer{}

	out, err := Render(f, filter.View{Kind: filter.ViewCard, Facet: "motivation"}, nil, spy.render, nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	if !strings.Contains(out, "Tom the Innkeeper") {
		t.Error("card-view render should still show the entry name header")
	}
	if !strings.Contains(out, "buy back his family farm") {
		t.Errorf("missing motivation card content, got: %s", out)
	}
	if strings.Contains(out, "Round-faced") {
		t.Errorf("card view leaked description card content, got: %s", out)
	}
	if strings.Contains(out, "Crimson Hand") {
		t.Errorf("card view leaked secret content, got: %s", out)
	}
}

func TestRender_CardExpansion_DepthOne(t *testing.T) {
	resolver := fakeResolver{
		"characters/npcs/mary": {
			"description": "Elegant, sharp-eyed shopkeeper. Owns [[locations/gilded-goose|the Gilded Goose]].",
		},
	}
	spy := &spyLinkRenderer{}
	view := filter.View{Kind: filter.ViewCard, Facet: "description"}
	f := mustFilter(t, tomExample, view)

	out, err := Render(f, view, resolver, spy.render, nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	if !strings.Contains(out, "Round-faced") {
		t.Errorf("missing Tom's own description card content, got: %s", out)
	}
	if !strings.Contains(out, "Elegant, sharp-eyed shopkeeper") {
		t.Errorf("missing Mary's expanded description card content, got: %s", out)
	}
	if !strings.Contains(out, "card-block--expanded") {
		t.Errorf("expanded card should carry the card-block--expanded class, got: %s", out)
	}

	// Tom's own inline link to Mary (depth 0, successfully expanded)
	// must NOT go through the callback — only Mary's own link (depth 1,
	// never expands further) should.
	if len(spy.calls) != 1 {
		t.Fatalf("got %d linkRenderer calls, want exactly 1 (Mary's depth-1 link): %+v", len(spy.calls), spy.calls)
	}
	call := spy.calls[0]
	if call.Path != "locations/gilded-goose" {
		t.Errorf("the one callback call should be for Mary's own link, got Path=%q", call.Path)
	}
	if call.Facet != "description" {
		t.Errorf("depth-1 link callback Facet = %q, want description (passed through from the expansion context)", call.Facet)
	}

	// The order matters: Tom's card, then Mary's expanded card.
	tomIdx := strings.Index(out, "Round-faced")
	maryIdx := strings.Index(out, "Elegant, sharp-eyed")
	if tomIdx == -1 || maryIdx == -1 || tomIdx > maryIdx {
		t.Errorf("expected Tom's card before Mary's expanded card in output, got: %s", out)
	}
}

func TestRender_CardExpansion_NoMatchingCard(t *testing.T) {
	resolver := fakeResolver{
		"characters/npcs/mary": {}, // exists, but has no "description" card
	}
	spy := &spyLinkRenderer{}
	view := filter.View{Kind: filter.ViewCard, Facet: "description"}
	f := mustFilter(t, tomExample, view)

	if _, err := Render(f, view, resolver, spy.render, nil); err != nil {
		t.Fatalf("Render: %v", err)
	}

	if len(spy.calls) != 1 {
		t.Fatalf("got %d linkRenderer calls, want 1 (Tom's link, not expanded)", len(spy.calls))
	}
	if !spy.calls[0].NoMatchingCard {
		t.Error("NoMatchingCard should be true: Mary exists but has no description card")
	}
}

func TestRender_LinkRendererCallCounts(t *testing.T) {
	t.Run("DM view calls it for non-expanded links", func(t *testing.T) {
		spy := &spyLinkRenderer{}
		f := mustFilter(t, tomExample, filter.View{Kind: filter.ViewDM})
		// No resolver: nothing can expand, so every link (the opening
		// paragraph's Mary reference, and the one inside the
		// description card) goes through the callback.
		if _, err := Render(f, filter.View{Kind: filter.ViewDM}, nil, spy.render, nil); err != nil {
			t.Fatalf("Render: %v", err)
		}
		if len(spy.calls) != 2 {
			t.Fatalf("got %d calls, want 2 (opening paragraph + description card, neither expands in DM view): %+v", len(spy.calls), spy.calls)
		}
		for _, c := range spy.calls {
			if c.Facet != "" {
				t.Errorf("DM-view link callback Facet = %q, want empty (no single active facet)", c.Facet)
			}
		}
	})

	t.Run("card-expanded view calls it for links beyond depth 1", func(t *testing.T) {
		resolver := fakeResolver{
			"characters/npcs/mary": {
				"description": "See also [[locations/gilded-goose]].",
			},
		}
		spy := &spyLinkRenderer{}
		view := filter.View{Kind: filter.ViewCard, Facet: "description"}
		f := mustFilter(t, tomExample, view)
		if _, err := Render(f, view, resolver, spy.render, nil); err != nil {
			t.Fatalf("Render: %v", err)
		}
		if len(spy.calls) != 1 {
			t.Fatalf("got %d calls, want 1 (only the depth-1 link inside Mary's expanded card)", len(spy.calls))
		}
		if spy.calls[0].Path != "locations/gilded-goose" {
			t.Errorf("call = %+v, want the depth-1 link", spy.calls[0])
		}
	})

	t.Run("player view never calls it", func(t *testing.T) {
		spy := &spyLinkRenderer{}
		f := mustFilter(t, tomExample, filter.View{Kind: filter.ViewPlayer})
		if _, err := Render(f, filter.View{Kind: filter.ViewPlayer}, nil, spy.render, nil); err != nil {
			t.Fatalf("Render: %v", err)
		}
		if len(spy.calls) != 0 {
			t.Fatalf("got %d calls, want 0 — player view must never invoke linkRenderer", len(spy.calls))
		}
	})
}

func TestRender_OutputIsFragment(t *testing.T) {
	f := mustFilter(t, tomExample, filter.View{Kind: filter.ViewDM})
	spy := &spyLinkRenderer{}
	out, err := Render(f, filter.View{Kind: filter.ViewDM}, nil, spy.render, nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	lower := strings.ToLower(out)
	for _, forbidden := range []string{"<!doctype", "<html", "<head", "<body"} {
		if strings.Contains(lower, forbidden) {
			t.Errorf("render output should be a fragment with no document wrapper, but found %q in: %s", forbidden, out)
		}
	}
}

func TestRender_ImageResolver_RewritesRelativeSrc(t *testing.T) {
	f := mustFilter(t, "---\nname: Tom\n---\n\n![Tom's portrait](tom-portrait.png)\n", filter.View{Kind: filter.ViewDM})
	spy := &spyLinkRenderer{}

	out, err := Render(f, filter.View{Kind: filter.ViewDM}, nil, spy.render, func(src string) string {
		return "/api/content/entries/e1/images/" + src
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, `src="/api/content/entries/e1/images/tom-portrait.png"`) {
		t.Errorf("relative image src not rewritten, got: %s", out)
	}
}

func TestRender_ImageResolver_LeavesAbsoluteAndExternalSrcAlone(t *testing.T) {
	src := "---\nname: Tom\n---\n\n![local](/already/absolute.png)\n\n![remote](https://example.com/x.png)\n"
	f := mustFilter(t, src, filter.View{Kind: filter.ViewDM})
	spy := &spyLinkRenderer{}
	called := false

	out, err := Render(f, filter.View{Kind: filter.ViewDM}, nil, spy.render, func(s string) string {
		called = true
		return "SHOULD-NOT-APPEAR"
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if called {
		t.Error("imageResolver should not be called for an absolute-path or external src")
	}
	if !strings.Contains(out, `src="/already/absolute.png"`) {
		t.Errorf("absolute-path src was altered, got: %s", out)
	}
	if !strings.Contains(out, `src="https://example.com/x.png"`) {
		t.Errorf("external src was altered, got: %s", out)
	}
}

func TestRender_NilImageResolver_MatchesPreviousOutput(t *testing.T) {
	f := mustFilter(t, "---\nname: Tom\n---\n\n![Tom's portrait](tom-portrait.png)\n", filter.View{Kind: filter.ViewDM})
	spy := &spyLinkRenderer{}

	out, err := Render(f, filter.View{Kind: filter.ViewDM}, nil, spy.render, nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, `src="tom-portrait.png"`) {
		t.Errorf("nil resolver should leave the raw markdown src untouched, got: %s", out)
	}
}

func TestRender_ImageResolver_NotAppliedInsideExpandedCard(t *testing.T) {
	resolver := fakeResolver{
		"characters/npcs/mary": {
			"description": "![mary](mary-portrait.png)",
		},
	}
	spy := &spyLinkRenderer{}
	view := filter.View{Kind: filter.ViewCard, Facet: "description"}
	f := mustFilter(t, tomExample, view)

	out, err := Render(f, view, resolver, spy.render, func(src string) string {
		return "/api/content/entries/tom/images/" + src
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, `src="mary-portrait.png"`) {
		t.Errorf("expanded card's image src should stay unresolved (no known entry id for it), got: %s", out)
	}
	if strings.Contains(out, "/api/content/entries/tom/images/mary-portrait.png") {
		t.Errorf("expanded card's image must not resolve against the wrong (top-level) entry id, got: %s", out)
	}
}

func TestRender_UnknownFacet_FullPipeline(t *testing.T) {
	entry, err := parse.Parse([]byte(tomExample))
	if err != nil {
		t.Fatalf("parse.Parse: %v", err)
	}
	view := filter.View{Kind: filter.ViewCard, Facet: "bogus-facet"}
	_, err = filter.Filter(entry, view, defaultVocab)
	if err == nil {
		t.Fatal("expected filter.UnknownFacet for the full parse->filter->render pipeline")
	}
	uf, ok := err.(filter.UnknownFacet)
	if !ok {
		t.Fatalf("expected filter.UnknownFacet, got %T: %v", err, err)
	}
	if uf.Name != "bogus-facet" {
		t.Errorf("Name = %q", uf.Name)
	}
	if len(uf.Vocabulary) == 0 {
		t.Error("Vocabulary should list the effective vocabulary")
	}
}
