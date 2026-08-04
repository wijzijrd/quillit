package home

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const ProjectConfigFileName = "quillit.yaml"

// ProjectConfig is a project's quillit.yaml.
type ProjectConfig struct {
	Name        string   `yaml:"name"`
	ExtraFacets []string `yaml:"extra_facets"`
}

// Project is a resolved project: its root directory plus parsed quillit.yaml.
type Project struct {
	Root   string
	Config ProjectConfig
}

func projectConfigPath(root string) string {
	return filepath.Join(root, ProjectConfigFileName)
}

// LoadProject reads quillit.yaml from root. root must already be known to
// contain one (see IsProjectRoot) — Load itself only fails on a missing or
// malformed file.
func LoadProject(root string) (*Project, error) {
	data, err := os.ReadFile(projectConfigPath(root))
	if err != nil {
		return nil, fmt.Errorf("reading project config at %s: %w", projectConfigPath(root), err)
	}
	var cfg ProjectConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing project config at %s: %w", projectConfigPath(root), err)
	}
	return &Project{Root: root, Config: cfg}, nil
}

// Save writes the project's quillit.yaml back to disk.
func (p *Project) Save() error {
	data, err := yaml.Marshal(p.Config)
	if err != nil {
		return fmt.Errorf("encoding project config: %w", err)
	}
	if err := os.WriteFile(projectConfigPath(p.Root), data, 0o644); err != nil {
		return fmt.Errorf("writing project config at %s: %w", projectConfigPath(p.Root), err)
	}
	return nil
}

// IsProjectRoot reports whether dir directly contains a quillit.yaml.
func IsProjectRoot(dir string) bool {
	info, err := os.Stat(projectConfigPath(dir))
	return err == nil && !info.IsDir()
}

// EffectiveFacets returns the effective facet vocabulary for the project:
// global facets followed by any project extra_facets not already present
// in the global list, per CLI spec §5 "Facet resolution" — projects add,
// never remove, global facets.
func (p *Project) EffectiveFacets(globalFacets []string) []string {
	seen := make(map[string]bool, len(globalFacets))
	out := make([]string, 0, len(globalFacets)+len(p.Config.ExtraFacets))
	for _, f := range globalFacets {
		if !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	for _, f := range p.Config.ExtraFacets {
		if !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	return out
}

// StarterFolders are the suggested (not enforced) organizational folders
// created inside a fresh project, per CLI spec §5 "Project layout".
var StarterFolders = []string{"characters", "locations", "sessions"}

// CreateProject creates a new project named name under h, per CLI spec §7
// "init": fails with ErrProjectExists if the directory already exists,
// otherwise creates quillit.yaml (empty extra_facets) plus the starter
// folders.
func CreateProject(h *Home, name string) (*Project, error) {
	root := filepath.Join(h.Path, name)
	if _, err := os.Stat(root); err == nil {
		return nil, ErrProjectExists{Name: name}
	}

	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("creating project directory at %s: %w", root, err)
	}
	for _, folder := range StarterFolders {
		if err := os.MkdirAll(filepath.Join(root, folder), 0o755); err != nil {
			return nil, fmt.Errorf("creating starter folder %s: %w", folder, err)
		}
	}

	p := &Project{Root: root, Config: ProjectConfig{Name: name, ExtraFacets: []string{}}}
	if err := p.Save(); err != nil {
		return nil, err
	}
	return p, nil
}

// Sentinel/typed errors for project resolution, per CLI spec §7 "Errors
// (all commands)". Errors() messages name the fix, per golden rule 6.

// ErrNoProject means neither the cwd-walk-up nor the current_project
// fallback resolved a project.
type ErrNoProject struct{}

func (ErrNoProject) Error() string {
	return "no quillit project resolved: run this command from inside a project directory, " +
		"or set one with `quillit connect <project_name>`"
}

// ErrDeadCurrentProject means the home's current_project pointer names a
// project that no longer exists on disk.
type ErrDeadCurrentProject struct {
	Name string
}

func (e ErrDeadCurrentProject) Error() string {
	return fmt.Sprintf(
		"connected project %q no longer exists at $QUILLIT_HOME; run `quillit connect <project_name>` "+
			"to pick another (see `quillit connect` with no argument for guidance)",
		e.Name,
	)
}

// ErrProjectNotFound means a project name given to a command (e.g.
// `connect <name>`) doesn't exist under $QUILLIT_HOME.
type ErrProjectNotFound struct {
	Name      string
	Available []string
}

func (e ErrProjectNotFound) Error() string {
	if len(e.Available) == 0 {
		return fmt.Sprintf("no project named %q, and no projects exist yet under $QUILLIT_HOME (try `quillit init %s`)", e.Name, e.Name)
	}
	return fmt.Sprintf("no project named %q; available projects: %v", e.Name, e.Available)
}

// ErrProjectExists means `init` was asked to create a project whose
// directory already exists.
type ErrProjectExists struct {
	Name string
}

func (e ErrProjectExists) Error() string {
	return fmt.Sprintf("project %q already exists under $QUILLIT_HOME", e.Name)
}

// ResolveProject implements the CLI spec §5 project-resolution order:
//  1. walk up from cwd to the nearest directory containing quillit.yaml;
//  2. else fall back to h's current_project pointer.
//
// h may be nil (e.g. $QUILLIT_HOME was never bootstrapped) — the walk-up
// path still works in that case; only the fallback is skipped.
func ResolveProject(cwd string, h *Home) (*Project, error) {
	if root, ok := findProjectRoot(cwd); ok {
		return LoadProject(root)
	}

	if h == nil || h.Config.CurrentProject == "" {
		return nil, ErrNoProject{}
	}

	root := filepath.Join(h.Path, h.Config.CurrentProject)
	if !IsProjectRoot(root) {
		return nil, ErrDeadCurrentProject{Name: h.Config.CurrentProject}
	}
	return LoadProject(root)
}

// findProjectRoot walks up from dir looking for the nearest ancestor
// (inclusive) containing quillit.yaml.
func findProjectRoot(dir string) (string, bool) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	for {
		if IsProjectRoot(dir) {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}
