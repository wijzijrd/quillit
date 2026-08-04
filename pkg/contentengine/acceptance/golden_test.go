package acceptance

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/quillit/contentengine/filter"
	"github.com/quillit/contentengine/linkindex"
	"github.com/quillit/contentengine/parse"
	"github.com/quillit/contentengine/render"
)

// update regenerates every golden file this suite checks against,
// instead of comparing to them — run once after a deliberate pipeline
// change, inspect the diff, then commit the result:
//
//	go test ./... -run TestGolden -update
var update = flag.Bool("update", false, "regenerate golden files instead of checking against them")

var defaultVocabulary = []string{"motivation", "description", "history"}

func loadFixture(t *testing.T, path string) *parse.Entry {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading fixture %s: %v", path, err)
	}
	entry, err := parse.Parse(data)
	if err != nil {
		t.Fatalf("parsing fixture %s: %v", path, err)
	}
	return entry
}

// checkGolden compares actual against the file at goldenPath, or (with
// -update) overwrites it. actual is always newline-terminated before
// writing/comparing so golden files end cleanly.
func checkGolden(t *testing.T, goldenPath string, actual []byte) {
	t.Helper()
	if len(actual) == 0 || actual[len(actual)-1] != '\n' {
		actual = append(actual, '\n')
	}

	if *update {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, actual, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("reading golden %s (run `go test ./... -run TestGolden -update` to generate it): %v", goldenPath, err)
	}
	if string(want) != string(actual) {
		t.Errorf("golden mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", goldenPath, want, actual)
	}
}

func marshalJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshaling: %v", err)
	}
	return data
}

// renderView filters entry for view and renders it, failing the test on
// any error (every golden fixture except the malformed one is expected
// to render cleanly).
func renderView(t *testing.T, entry *parse.Entry, view filter.View, resolver render.Resolver) string {
	t.Helper()
	filtered, err := filter.Filter(entry, view, defaultVocabulary)
	if err != nil {
		t.Fatalf("filter.Filter: %v", err)
	}
	html, err := render.Render(filtered, view, resolver, plainLinkRenderer)
	if err != nil {
		t.Fatalf("render.Render: %v", err)
	}
	return html
}

// resolvedLinks runs linkindex.Extract and resolves each record's
// existence against resolver — the same two-step compileOne performs
// (see cli/cmd/compile.go), reimplemented here since this package can't
// import cli/'s module.
func resolvedLinks(entry *parse.Entry, resolver render.Resolver) []linkindex.Record {
	records := linkindex.Extract(entry)
	for i := range records {
		exists, _ := resolver.Resolve(records[i].TargetPath)
		records[i].Resolved = exists
	}
	return records
}

func TestGolden_TomExample(t *testing.T) {
	dir := "testdata/tom"
	tom := loadFixture(t, filepath.Join(dir, "tom.md"))
	mary := loadFixture(t, filepath.Join(dir, "mary.md"))
	resolver := memResolver{"characters/npcs/mary": mary}

	checkGolden(t, filepath.Join(dir, "parse.golden.json"), marshalJSON(t, tom))

	for _, tc := range []struct {
		name string
		view filter.View
	}{
		{"dm", filter.View{Kind: filter.ViewDM}},
		{"player", filter.View{Kind: filter.ViewPlayer}},
		{"card-motivation", filter.View{Kind: filter.ViewCard, Facet: "motivation"}},
		{"card-description", filter.View{Kind: filter.ViewCard, Facet: "description"}},
	} {
		html := renderView(t, tom, tc.view, resolver)
		checkGolden(t, filepath.Join(dir, tc.name+".golden.html"), []byte(html))
	}

	checkGolden(t, filepath.Join(dir, "links.golden.json"), marshalJSON(t, resolvedLinks(tom, resolver)))
}

func TestGolden_NoDirectives(t *testing.T) {
	dir := "testdata/no-directives"
	entry := loadFixture(t, filepath.Join(dir, "entry.md"))

	checkGolden(t, filepath.Join(dir, "parse.golden.json"), marshalJSON(t, entry))
	checkGolden(t, filepath.Join(dir, "dm.golden.html"), []byte(renderView(t, entry, filter.View{Kind: filter.ViewDM}, nil)))
	checkGolden(t, filepath.Join(dir, "links.golden.json"), marshalJSON(t, resolvedLinks(entry, memResolver{})))
}

func TestGolden_NestedCardInSecret(t *testing.T) {
	dir := "testdata/nested-card-in-secret"
	entry := loadFixture(t, filepath.Join(dir, "entry.md"))

	checkGolden(t, filepath.Join(dir, "parse.golden.json"), marshalJSON(t, entry))
	checkGolden(t, filepath.Join(dir, "dm.golden.html"), []byte(renderView(t, entry, filter.View{Kind: filter.ViewDM}, nil)))
	checkGolden(t, filepath.Join(dir, "player.golden.html"), []byte(renderView(t, entry, filter.View{Kind: filter.ViewPlayer}, nil)))
	checkGolden(t, filepath.Join(dir, "card-motivation.golden.html"), []byte(renderView(t, entry, filter.View{Kind: filter.ViewCard, Facet: "motivation"}, nil)))
	checkGolden(t, filepath.Join(dir, "links.golden.json"), marshalJSON(t, resolvedLinks(entry, memResolver{})))
}

func TestGolden_DepthChain(t *testing.T) {
	dir := "testdata/depth-chain"
	a := loadFixture(t, filepath.Join(dir, "a.md"))
	b := loadFixture(t, filepath.Join(dir, "b.md"))
	c := loadFixture(t, filepath.Join(dir, "c.md"))
	resolver := memResolver{"chain-b": b, "chain-c": c}

	// Rendering A's history card must expand B (depth 1) but never reach
	// C (depth 2) — C's content must not appear anywhere in the output;
	// asserted directly, not just implied by the golden file, so this
	// specific regression is loud even to someone skimming a diff.
	html := renderView(t, a, filter.View{Kind: filter.ViewCard, Facet: "history"}, resolver)
	checkGolden(t, filepath.Join(dir, "card-history.golden.html"), []byte(html))
	if strings.Contains(html, "C ended the order") {
		t.Fatal("depth-1 expansion boundary violated: Chain C's content appeared in a render of A")
	}

	checkGolden(t, filepath.Join(dir, "links.golden.json"), marshalJSON(t, resolvedLinks(a, resolver)))
}

func TestGolden_DanglingLink(t *testing.T) {
	dir := "testdata/dangling-link"
	entry := loadFixture(t, filepath.Join(dir, "entry.md"))
	resolver := memResolver{} // deliberately empty: the link never resolves

	checkGolden(t, filepath.Join(dir, "parse.golden.json"), marshalJSON(t, entry))
	checkGolden(t, filepath.Join(dir, "dm.golden.html"), []byte(renderView(t, entry, filter.View{Kind: filter.ViewDM}, resolver)))

	records := resolvedLinks(entry, resolver)
	if len(records) != 1 || records[0].Resolved {
		t.Fatalf("expected exactly one unresolved link record, got %+v", records)
	}
	checkGolden(t, filepath.Join(dir, "links.golden.json"), marshalJSON(t, records))
}

func TestGolden_MalformedDirective(t *testing.T) {
	data, err := os.ReadFile("testdata/malformed/entry.md")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	_, err = parse.Parse(data)
	if err == nil {
		t.Fatal("expected a parse error for the unclosed :::secret block")
	}
	if _, ok := err.(parse.MalformedDirective); !ok {
		t.Fatalf("expected parse.MalformedDirective, got %T: %v", err, err)
	}
}
