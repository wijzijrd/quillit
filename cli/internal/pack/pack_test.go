package pack

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func makeProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "quillit.yaml"), "name: test\nextra_facets: []\n")
	writeFile(t, filepath.Join(root, "characters/npcs/tom/tom.md"), "body")
	writeFile(t, filepath.Join(root, "characters/npcs/tom/tom.html"), "scaffold")
	writeFile(t, filepath.Join(root, "characters/npcs/tom/tom.css"), "scaffold")
	writeFile(t, filepath.Join(root, "characters/npcs/tom/tom.js"), "scaffold")
	writeFile(t, filepath.Join(root, "characters/npcs/tom/links.conf"), "links")
	writeFile(t, filepath.Join(root, "characters/npcs/tom/tom-portrait.png"), "PNG")
	writeFile(t, filepath.Join(root, "locations/inn/inn.md"), "inn")
	return root
}

func tarMembers(t *testing.T, data []byte) []string {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("gzip: %v", err)
	}
	tr := tar.NewReader(gz)
	var names []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar: %v", err)
		}
		names = append(names, hdr.Name)
	}
	sort.Strings(names)
	return names
}

func TestProject_PacksEntriesSkipsScaffolding(t *testing.T) {
	root := makeProject(t)
	var buf bytes.Buffer
	if err := Project(&buf, root, ""); err != nil {
		t.Fatalf("Project: %v", err)
	}
	got := tarMembers(t, buf.Bytes())
	want := []string{
		"characters/npcs/tom/tom-portrait.png",
		"characters/npcs/tom/tom.md",
		"locations/inn/inn.md",
	}
	if len(got) != len(want) {
		t.Fatalf("members = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("member %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestProject_SingleEntryKeepsRelativePath(t *testing.T) {
	root := makeProject(t)
	var buf bytes.Buffer
	if err := Project(&buf, root, "characters/npcs/tom"); err != nil {
		t.Fatalf("Project: %v", err)
	}
	got := tarMembers(t, buf.Bytes())
	want := []string{"characters/npcs/tom/tom-portrait.png", "characters/npcs/tom/tom.md"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("members = %v, want %v", got, want)
	}
}

func TestProject_MissingEntryFolderErrors(t *testing.T) {
	root := makeProject(t)
	var buf bytes.Buffer
	if err := Project(&buf, root, "characters/npcs/nobody"); err == nil {
		t.Error("missing --entry folder accepted, want error")
	}
}

func TestSelected_PacksOnlyGivenPaths(t *testing.T) {
	root := makeProject(t)
	var buf bytes.Buffer
	if err := Selected(&buf, root, []string{"characters/npcs/tom", "locations/inn"}); err != nil {
		t.Fatalf("Selected: %v", err)
	}
	got := tarMembers(t, buf.Bytes())
	want := []string{
		"characters/npcs/tom/tom-portrait.png",
		"characters/npcs/tom/tom.md",
		"locations/inn/inn.md",
	}
	if len(got) != len(want) {
		t.Fatalf("members = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("member %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSelected_OnePathMatchesProjectSingleEntryOutput(t *testing.T) {
	root := makeProject(t)
	var selBuf, projBuf bytes.Buffer
	if err := Selected(&selBuf, root, []string{"characters/npcs/tom"}); err != nil {
		t.Fatalf("Selected: %v", err)
	}
	if err := Project(&projBuf, root, "characters/npcs/tom"); err != nil {
		t.Fatalf("Project: %v", err)
	}
	selMembers := tarMembers(t, selBuf.Bytes())
	projMembers := tarMembers(t, projBuf.Bytes())
	if len(selMembers) != len(projMembers) {
		t.Fatalf("Selected members = %v, Project(--entry) members = %v — should match", selMembers, projMembers)
	}
	for i := range selMembers {
		if selMembers[i] != projMembers[i] {
			t.Errorf("member %d: Selected = %q, Project(--entry) = %q — should match", i, selMembers[i], projMembers[i])
		}
	}
}

func TestSelected_EmptyPathsProducesEmptyArchive(t *testing.T) {
	root := makeProject(t)
	var buf bytes.Buffer
	if err := Selected(&buf, root, nil); err != nil {
		t.Fatalf("Selected: %v", err)
	}
	if got := tarMembers(t, buf.Bytes()); len(got) != 0 {
		t.Errorf("members = %v, want none", got)
	}
}
