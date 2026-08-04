package resolver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeEntry creates a minimal entry folder (just the .md, which is all
// this package reads) at root/relPath.
func writeEntry(t *testing.T, root, relPath, body string) {
	t.Helper()
	dir := filepath.Join(root, relPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	name := filepath.Base(relPath)
	if err := os.WriteFile(filepath.Join(dir, name+".md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fixtureProject builds a small project: Tom links to Mary; Mary has a
// description card and links onward to the Gilded Goose; the Gilded
// Goose has no cards at all.
func fixtureProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	writeEntry(t, root, "characters/npcs/tom", `# Tom
See [[characters/npcs/mary]].

:::card description
Round-faced innkeeper.
:::
`)
	writeEntry(t, root, "characters/npcs/mary", `# Mary
:::card description
Sharp-eyed shopkeeper. Owns [[locations/gilded-goose]].
:::

:::card motivation
Wants Tom to notice her.
:::
`)
	writeEntry(t, root, "locations/gilded-goose", `# The Gilded Goose
A cozy inn with no cards of its own.
`)

	return root
}

func TestFS_Resolve_ExistingEntryWithCard(t *testing.T) {
	root := fixtureProject(t)
	r := FS{ProjectRoot: root}

	exists, cardForFacet := r.Resolve("characters/npcs/mary")
	if !exists {
		t.Fatal("expected characters/npcs/mary to exist")
	}
	content, ok := cardForFacet("description")
	if !ok {
		t.Fatal("expected a description card for Mary")
	}
	if !strings.Contains(content, "Sharp-eyed shopkeeper") {
		t.Errorf("card content = %q", content)
	}
}

func TestFS_Resolve_ExistingEntryWithoutMatchingCard(t *testing.T) {
	root := fixtureProject(t)
	r := FS{ProjectRoot: root}

	exists, cardForFacet := r.Resolve("locations/gilded-goose")
	if !exists {
		t.Fatal("expected locations/gilded-goose to exist")
	}
	_, ok := cardForFacet("description")
	if ok {
		t.Error("the Gilded Goose has no cards at all; cardForFacet should report ok=false")
	}
}

func TestFS_Resolve_NonExistentEntry(t *testing.T) {
	root := fixtureProject(t)
	r := FS{ProjectRoot: root}

	exists, cardForFacet := r.Resolve("characters/npcs/ghost")
	if exists {
		t.Error("expected a non-existent entry to resolve as not existing")
	}
	if cardForFacet != nil {
		t.Error("cardForFacet should be nil when the entry doesn't exist")
	}
}

func TestFS_Resolve_MultipleFacetsOnSameEntry(t *testing.T) {
	root := fixtureProject(t)
	r := FS{ProjectRoot: root}

	_, cardForFacet := r.Resolve("characters/npcs/mary")
	if _, ok := cardForFacet("motivation"); !ok {
		t.Error("expected Mary's motivation card to resolve too")
	}
	if _, ok := cardForFacet("history"); ok {
		t.Error("Mary has no history card; expected ok=false")
	}
}

func TestIsStale(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "tom.md")
	conf := filepath.Join(dir, "links.conf")

	if err := os.WriteFile(md, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	stale, err := IsStale(md, conf)
	if err != nil {
		t.Fatalf("IsStale: %v", err)
	}
	if !stale {
		t.Error("links.conf doesn't exist yet: expected stale=true")
	}

	if err := os.WriteFile(conf, []byte("[]"), 0o644); err != nil {
		t.Fatal(err)
	}
	stale, err = IsStale(md, conf)
	if err != nil {
		t.Fatalf("IsStale: %v", err)
	}
	if stale {
		t.Error("links.conf just written after .md: expected stale=false")
	}

	// Touch the .md again, later, and confirm staleness flips.
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(md, []byte("content, edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	stale, err = IsStale(md, conf)
	if err != nil {
		t.Fatalf("IsStale: %v", err)
	}
	if !stale {
		t.Error(".md edited after links.conf was written: expected stale=true")
	}
}
