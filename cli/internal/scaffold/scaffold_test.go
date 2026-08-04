package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWrite(t *testing.T) {
	dir := t.TempDir()

	if err := Write(dir, Data{Name: "tom"}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	wantFiles := map[string][]string{
		"tom.md":   {"name: tom", ":::secret", ":::card motivation"},
		"tom.html": {"<title>tom</title>", "tom.css", "tom.js", "quillit-entry-content"},
		"tom.css":  {"clipboard-link"},
		"tom.js":   {"clipboard-link", "data-quillit-command"},
	}

	for name, mustContain := range wantFiles {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("expected file %s to exist: %v", name, err)
		}
		if len(data) == 0 {
			t.Errorf("%s is empty", name)
		}
		content := string(data)
		for _, substr := range mustContain {
			if !strings.Contains(content, substr) {
				t.Errorf("%s missing expected content %q", name, substr)
			}
		}
	}
}

func TestWrite_DifferentNamesProduceDifferentBasenames(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, Data{Name: "mary"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	for _, ext := range []string{"md", "html", "css", "js"} {
		if _, err := os.Stat(filepath.Join(dir, "mary."+ext)); err != nil {
			t.Errorf("expected mary.%s to exist: %v", ext, err)
		}
	}
}
