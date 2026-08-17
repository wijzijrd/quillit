package home

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocate_EnvVarWins(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(EnvVar, "/env/path")

	if err := writeMarker("/marker/path"); err != nil {
		t.Fatal(err)
	}

	path, ok := Locate()
	if !ok || path != "/env/path" {
		t.Errorf("Locate() = (%q, %v), want (/env/path, true)", path, ok)
	}
}

func TestLocate_FallsBackToMarker(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(EnvVar, "")

	if err := writeMarker("/marker/path"); err != nil {
		t.Fatal(err)
	}

	path, ok := Locate()
	if !ok || path != "/marker/path" {
		t.Errorf("Locate() = (%q, %v), want (/marker/path, true)", path, ok)
	}
}

func TestLocate_NeitherEnvNorMarker(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(EnvVar, "")

	path, ok := Locate()
	if ok || path != "" {
		t.Errorf("Locate() = (%q, %v), want (\"\", false)", path, ok)
	}
}

// TestSave_TightensPreexistingLoosePermissions guards against a home
// bootstrapped by an older CLI (config.yaml left at 0644) later having
// `quillit login` write session_token into it without tightening the mode —
// Save must chmod to 0600 every time, not just on initial creation.
func TestSave_TightensPreexistingLoosePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigFileName)
	if err := os.WriteFile(path, []byte("facets: []\n"), 0o644); err != nil {
		t.Fatalf("pre-creating config.yaml: %v", err)
	}

	h := &Home{Path: dir, Config: Config{Facets: []string{"motivation"}}}
	if err := h.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config.yaml: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("config.yaml mode after Save = %o, want 0600", got)
	}
}

func TestBootstrap_WritesMarkerForLaterLocate(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv(EnvVar, "")

	target := filepath.Join(fakeHome, "quillit")
	prompt := func(defaultPath string) (string, error) { return defaultPath, nil }

	if _, _, err := Bootstrap("", false, prompt); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	path, ok := Locate()
	if !ok || path != target {
		t.Errorf("Locate() after Bootstrap = (%q, %v), want (%q, true)", path, ok, target)
	}
}
