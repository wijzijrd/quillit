package cmd

import (
	"strings"
	"testing"

	"github.com/quillit/contentengine/filter"
	"github.com/quillit/contentengine/render"
	"github.com/spf13/cobra"
)

func TestClipboardLinkRenderer_DMView_NoFacet(t *testing.T) {
	out := clipboardLinkRenderer(render.LinkInfo{Path: "characters/npcs/mary", Label: "Mary"})

	wantCommand := "quillit render characters/npcs/mary"
	if !strings.Contains(out, `data-quillit-command="`+wantCommand+`"`) {
		t.Errorf("output = %q, want data-quillit-command=%q", out, wantCommand)
	}
	if !strings.Contains(out, "clipboard-link") {
		t.Errorf("output missing clipboard-link class: %q", out)
	}
	if strings.Contains(out, "clipboard-link--no-card") {
		t.Errorf("output should not carry the no-card class here: %q", out)
	}
	if !strings.Contains(out, ">Mary<") {
		t.Errorf("output missing visible label: %q", out)
	}
}

func TestClipboardLinkRenderer_CardView_IncludesFacet(t *testing.T) {
	out := clipboardLinkRenderer(render.LinkInfo{Path: "characters/npcs/mary", Label: "Mary", Facet: "description"})

	wantCommand := "quillit render characters/npcs/mary --card description"
	if !strings.Contains(out, `data-quillit-command="`+wantCommand+`"`) {
		t.Errorf("output = %q, want data-quillit-command=%q", out, wantCommand)
	}
}

func TestClipboardLinkRenderer_NoMatchingCard_AddsNote(t *testing.T) {
	out := clipboardLinkRenderer(render.LinkInfo{
		Path: "locations/gilded-goose", Label: "the Gilded Goose",
		Facet: "history", NoMatchingCard: true,
	})

	if !strings.Contains(out, "clipboard-link--no-card") {
		t.Errorf("output missing no-card class: %q", out)
	}
	if !strings.Contains(out, "no history card") {
		t.Errorf("output missing the (no <facet> card) note: %q", out)
	}
	// Still a valid, clickable command — CLI spec §4: "render doesn't
	// fail" for this case.
	if !strings.Contains(out, `data-quillit-command="quillit render locations/gilded-goose --card history"`) {
		t.Errorf("output = %q, want a working command even without a matching card", out)
	}
}

func TestClipboardLinkRenderer_EscapesHTML(t *testing.T) {
	out := clipboardLinkRenderer(render.LinkInfo{Path: "a/b", Label: `<script>alert(1)</script>`})
	if strings.Contains(out, "<script>") {
		t.Errorf("label was not HTML-escaped: %q", out)
	}
}

// newRenderTestCmd builds a fresh, isolated *cobra.Command carrying just
// the render flags, bound to the (reset) package-level renderView/
// renderCard/renderQuick vars buildView reads. A fresh command per test
// means Flags().Changed(...) starts false for every case, unlike reusing
// the real renderCmd singleton (whose Changed state would leak across
// tests, and across a real invocation, once set).
func newRenderTestCmd() *cobra.Command {
	renderView, renderCard, renderQuick = "dm", "", false
	c := &cobra.Command{}
	c.Flags().StringVar(&renderView, "view", "dm", "")
	c.Flags().StringVar(&renderCard, "card", "", "")
	c.Flags().BoolVarP(&renderQuick, "quick-view", "Q", false, "")
	return c
}

func TestBuildView_DefaultIsDM(t *testing.T) {
	c := newRenderTestCmd()
	view, err := buildView(c)
	if err != nil {
		t.Fatalf("buildView: %v", err)
	}
	if view.Kind != filter.ViewDM {
		t.Errorf("Kind = %v, want ViewDM", view.Kind)
	}
}

func TestBuildView_ViewPlayer(t *testing.T) {
	c := newRenderTestCmd()
	if err := c.Flags().Set("view", "player"); err != nil {
		t.Fatal(err)
	}
	view, err := buildView(c)
	if err != nil {
		t.Fatalf("buildView: %v", err)
	}
	if view.Kind != filter.ViewPlayer {
		t.Errorf("Kind = %v, want ViewPlayer", view.Kind)
	}
}

func TestBuildView_CardFlagSelectsCardView(t *testing.T) {
	c := newRenderTestCmd()
	if err := c.Flags().Set("card", "motivation"); err != nil {
		t.Fatal(err)
	}
	view, err := buildView(c)
	if err != nil {
		t.Fatalf("buildView: %v", err)
	}
	if view.Kind != filter.ViewCard || view.Facet != "motivation" {
		t.Errorf("view = %+v, want Kind=ViewCard Facet=motivation", view)
	}
}

func TestBuildView_QuickComposesWithPlayer(t *testing.T) {
	c := newRenderTestCmd()
	if err := c.Flags().Set("view", "player"); err != nil {
		t.Fatal(err)
	}
	if err := c.Flags().Set("quick-view", "true"); err != nil {
		t.Fatal(err)
	}
	view, err := buildView(c)
	if err != nil {
		t.Fatalf("buildView: %v", err)
	}
	if view.Kind != filter.ViewPlayer || !view.Quick {
		t.Errorf("view = %+v, want Kind=ViewPlayer Quick=true", view)
	}
}

func TestBuildView_QuickComposesWithCard(t *testing.T) {
	c := newRenderTestCmd()
	if err := c.Flags().Set("card", "history"); err != nil {
		t.Fatal(err)
	}
	if err := c.Flags().Set("quick-view", "true"); err != nil {
		t.Fatal(err)
	}
	view, err := buildView(c)
	if err != nil {
		t.Fatalf("buildView: %v", err)
	}
	if view.Kind != filter.ViewCard || view.Facet != "history" || !view.Quick {
		t.Errorf("view = %+v, want Kind=ViewCard Facet=history Quick=true", view)
	}
}

func TestBuildView_UnknownViewValue(t *testing.T) {
	c := newRenderTestCmd()
	if err := c.Flags().Set("view", "bogus"); err != nil {
		t.Fatal(err)
	}
	if _, err := buildView(c); err == nil {
		t.Fatal("expected an error for an unrecognized --view value")
	}
}
