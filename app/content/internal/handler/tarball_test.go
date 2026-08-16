package handler

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"strconv"
	"strings"
	"testing"
)

// makeTarball builds a gzipped tar from name→content pairs, in order.
func makeTarball(t *testing.T, files map[string]string) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatalf("tar header: %v", err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("tar write: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return &buf
}

func TestReadImportTarball_ExtractsEntries(t *testing.T) {
	buf := makeTarball(t, map[string]string{
		"characters/npcs/tom/tom.md":           "---\nname: Tom\n---\nBody",
		"characters/npcs/tom/tom.html":         "<html>",   // ignored scaffolding
		"characters/npcs/tom/tom.css":          "css",      // ignored
		"characters/npcs/tom/tom.js":           "js",       // ignored
		"characters/npcs/tom/links.conf":       "links",    // ignored
		"characters/npcs/tom/tom-portrait.png": "PNGDATA",  // asset
		"quillit.yaml":                         "name: p",  // ignored
		"inn/inn.md":                           "---\nname: Inn\n---\nInn body",
	})
	items, err := readImportTarball(buf)
	if err != nil {
		t.Fatalf("readImportTarball: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	byPath := map[string]importItem{}
	for _, it := range items {
		byPath[it.DirectoryPath+"/"+it.Slug] = it
	}
	tom := byPath["characters/npcs/tom"]
	if string(tom.Body) != "---\nname: Tom\n---\nBody" {
		t.Errorf("tom body = %q", tom.Body)
	}
	if len(tom.Assets) != 1 || tom.Assets[0].Name != "tom-portrait.png" {
		t.Errorf("tom assets = %+v, want [tom-portrait.png]", tom.Assets)
	}
	inn := byPath["/inn"] // root entry: DirectoryPath ""
	if inn.Slug != "inn" || inn.DirectoryPath != "" {
		t.Errorf("inn = %+v, want slug inn at root", inn)
	}
}

func TestReadImportTarball_RejectsTraversal(t *testing.T) {
	for _, name := range []string{"../evil/evil.md", "/abs/abs.md", "a/../../b/b.md"} {
		buf := makeTarball(t, map[string]string{name: "x"})
		if _, err := readImportTarball(buf); err == nil {
			t.Errorf("tar member %q accepted, want error", name)
		}
	}
}

func TestReadImportTarball_RejectsTooManyFiles(t *testing.T) {
	files := map[string]string{}
	for i := 0; i < 1001; i++ {
		files[strings.Repeat("a", 1)+"/"+fmtInt(i)+".txt"] = "x"
	}
	buf := makeTarball(t, files)
	if _, err := readImportTarball(buf); err == nil {
		t.Error("1001 files accepted, want error")
	}
}

func TestReadImportTarball_RejectsOversizeAsset(t *testing.T) {
	big := strings.Repeat("x", 10<<20+1) // 10 MB + 1
	buf := makeTarball(t, map[string]string{
		"tom/tom.md":  "body",
		"tom/big.png": big,
	})
	if _, err := readImportTarball(buf); err == nil {
		t.Error("oversize asset accepted, want error")
	}
}

func TestReadImportTarball_NotGzip(t *testing.T) {
	if _, err := readImportTarball(strings.NewReader("plain garbage")); err == nil {
		t.Error("non-gzip input accepted, want error")
	}
}

func fmtInt(i int) string { return strconv.Itoa(i) }
