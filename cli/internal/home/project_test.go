package home

import (
	"os"
	"path/filepath"
	"testing"
)

func writeProject(t *testing.T, root string, cfg ProjectConfig) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	p := &Project{Root: root, Config: cfg}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
}

func TestResolveProject_CwdWalkUp(t *testing.T) {
	base := t.TempDir()
	projectRoot := filepath.Join(base, "curse-of-strahd")
	writeProject(t, projectRoot, ProjectConfig{Name: "curse-of-strahd"})

	deep := filepath.Join(projectRoot, "characters", "npcs")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}

	// No home at all: walk-up alone must resolve.
	p, err := ResolveProject(deep, nil)
	if err != nil {
		t.Fatalf("ResolveProject: %v", err)
	}
	if p.Root != projectRoot {
		t.Errorf("Root = %q, want %q", p.Root, projectRoot)
	}
	if p.Config.Name != "curse-of-strahd" {
		t.Errorf("Name = %q, want curse-of-strahd", p.Config.Name)
	}
}

func TestResolveProject_CurrentProjectFallback(t *testing.T) {
	homePath := t.TempDir()
	writeProject(t, filepath.Join(homePath, "one-shots"), ProjectConfig{Name: "one-shots"})

	h := &Home{Path: homePath, Config: Config{CurrentProject: "one-shots"}}

	outside := t.TempDir() // not inside any project
	p, err := ResolveProject(outside, h)
	if err != nil {
		t.Fatalf("ResolveProject: %v", err)
	}
	if p.Config.Name != "one-shots" {
		t.Errorf("Name = %q, want one-shots", p.Config.Name)
	}
}

func TestResolveProject_WalkUpBeatsCurrentProject(t *testing.T) {
	homePath := t.TempDir()
	writeProject(t, filepath.Join(homePath, "wrong-project"), ProjectConfig{Name: "wrong-project"})

	other := t.TempDir()
	rightRoot := filepath.Join(other, "right-project")
	writeProject(t, rightRoot, ProjectConfig{Name: "right-project"})

	h := &Home{Path: homePath, Config: Config{CurrentProject: "wrong-project"}}

	p, err := ResolveProject(rightRoot, h)
	if err != nil {
		t.Fatalf("ResolveProject: %v", err)
	}
	if p.Config.Name != "right-project" {
		t.Errorf("cwd walk-up should win over current_project fallback; got %q", p.Config.Name)
	}
}

func TestResolveProject_NoneResolvable(t *testing.T) {
	outside := t.TempDir()

	if _, err := ResolveProject(outside, nil); err == nil {
		t.Fatal("expected error with nil home and no project in cwd chain")
	} else if _, ok := err.(ErrNoProject); !ok {
		t.Errorf("expected ErrNoProject, got %T: %v", err, err)
	}

	homePath := t.TempDir()
	h := &Home{Path: homePath, Config: Config{}} // no current_project set
	if _, err := ResolveProject(outside, h); err == nil {
		t.Fatal("expected error with empty current_project")
	} else if _, ok := err.(ErrNoProject); !ok {
		t.Errorf("expected ErrNoProject, got %T: %v", err, err)
	}
}

func TestResolveProject_DeadCurrentProject(t *testing.T) {
	homePath := t.TempDir() // "ghost" project directory never created
	h := &Home{Path: homePath, Config: Config{CurrentProject: "ghost"}}

	outside := t.TempDir()
	_, err := ResolveProject(outside, h)
	if err == nil {
		t.Fatal("expected error for dead current_project")
	}
	if _, ok := err.(ErrDeadCurrentProject); !ok {
		t.Errorf("expected ErrDeadCurrentProject, got %T: %v", err, err)
	}
}

func TestEffectiveFacets(t *testing.T) {
	p := &Project{Config: ProjectConfig{ExtraFacets: []string{"stat-block", "motivation"}}}
	got := p.EffectiveFacets([]string{"motivation", "description", "history"})
	want := []string{"motivation", "description", "history", "stat-block"}

	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v", got, want)
			break
		}
	}
}
