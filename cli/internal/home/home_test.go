package home

import (
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
