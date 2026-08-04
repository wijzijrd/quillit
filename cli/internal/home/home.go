// Package home implements $QUILLIT_HOME bootstrap and project resolution
// per docs/cli-spec.md §5 and §7.
package home

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	EnvVar         = "QUILLIT_HOME"
	ConfigFileName = "config.yaml"
	DefaultHomeDir = "quillit"
)

// DefaultFacets are seeded into a freshly bootstrapped home's config.yaml.
var DefaultFacets = []string{"motivation", "description", "history"}

// Config is the home-level config.yaml: global facet vocabulary plus the
// current-project pointer written by `connect`.
type Config struct {
	Facets         []string `yaml:"facets"`
	CurrentProject string   `yaml:"current_project"`
}

// Home wraps a resolved $QUILLIT_HOME directory and its parsed config.yaml.
type Home struct {
	Path   string
	Config Config
}

// ErrHomeNotBootstrapped means $QUILLIT_HOME is unset or its directory is
// missing, for a command that needs global config but must not bootstrap
// it implicitly (everything except `init`), per CLI spec §7 "Errors".
type ErrHomeNotBootstrapped struct{}

func (ErrHomeNotBootstrapped) Error() string {
	return "$QUILLIT_HOME is not set up yet; run `quillit init <project_name>` to bootstrap it"
}

// Locate reads $QUILLIT_HOME. ok is false if the env var is unset.
func Locate() (path string, ok bool) {
	v, set := os.LookupEnv(EnvVar)
	if !set || v == "" {
		return "", false
	}
	return v, true
}

// Exists reports whether path is an existing directory.
func Exists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// IsBootstrapped reports whether path is a directory containing a
// config.yaml — i.e. a fully (not partially) bootstrapped home. A
// directory that exists but lacks config.yaml (e.g. created by hand, or
// left behind by an interrupted bootstrap) is treated the same as a
// missing home: callers should bootstrap or report ErrHomeNotBootstrapped
// rather than surfacing a raw "file not found" from Load.
func IsBootstrapped(path string) bool {
	if !Exists(path) {
		return false
	}
	info, err := os.Stat(configPath(path))
	return err == nil && !info.IsDir()
}

// configPath returns the config.yaml path inside a home directory.
func configPath(homePath string) string {
	return filepath.Join(homePath, ConfigFileName)
}

// Load reads an existing home directory's config.yaml. Callers should have
// already verified the directory exists (see Exists) — Load itself only
// fails if config.yaml is missing or malformed, which indicates a broken
// (not merely absent) home.
func Load(path string) (*Home, error) {
	data, err := os.ReadFile(configPath(path))
	if err != nil {
		return nil, fmt.Errorf("reading home config at %s: %w", configPath(path), err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing home config at %s: %w", configPath(path), err)
	}
	return &Home{Path: path, Config: cfg}, nil
}

// Save writes the home's config.yaml back to disk.
func (h *Home) Save() error {
	data, err := yaml.Marshal(h.Config)
	if err != nil {
		return fmt.Errorf("encoding home config: %w", err)
	}
	if err := os.WriteFile(configPath(h.Path), data, 0o644); err != nil {
		return fmt.Errorf("writing home config at %s: %w", configPath(h.Path), err)
	}
	return nil
}

// Prompter asks the user for a home directory location when $QUILLIT_HOME
// is unset, offering defaultPath as the default. Implementations return the
// path the user chose (which may just be defaultPath).
type Prompter func(defaultPath string) (string, error)

// Bootstrap performs first-time $QUILLIT_HOME setup per CLI spec §7 "init":
//   - $QUILLIT_HOME set but its directory missing: use that value directly,
//     no prompt.
//   - $QUILLIT_HOME unset: ask via prompt, defaulting to ~/quillit.
//
// It creates the directory and seeds config.yaml with DefaultFacets, then
// returns the resulting Home plus the shell export line the caller should
// print (the CLI cannot persist environment variables for the user's shell
// itself).
func Bootstrap(envValue string, envSet bool, prompt Prompter) (h *Home, exportLine string, err error) {
	var target string
	switch {
	case envSet && envValue != "":
		target = envValue
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, "", fmt.Errorf("resolving default home directory: %w", err)
		}
		defaultPath := filepath.Join(home, DefaultHomeDir)
		target, err = prompt(defaultPath)
		if err != nil {
			return nil, "", fmt.Errorf("prompting for quillit home location: %w", err)
		}
		if target == "" {
			target = defaultPath
		}
	}

	if err := os.MkdirAll(target, 0o755); err != nil {
		return nil, "", fmt.Errorf("creating quillit home at %s: %w", target, err)
	}

	h = &Home{Path: target, Config: Config{Facets: append([]string(nil), DefaultFacets...)}}
	if err := h.Save(); err != nil {
		return nil, "", err
	}

	return h, fmt.Sprintf("export %s=%q", EnvVar, target), nil
}

// SetCurrentProject writes name as the connected project and persists it,
// per `quillit connect`'s behavior (CLI spec §7).
func (h *Home) SetCurrentProject(name string) error {
	h.Config.CurrentProject = name
	return h.Save()
}

// ListProjects returns the names of every project (subdirectory containing
// a quillit.yaml) directly inside the home directory.
func (h *Home) ListProjects() ([]string, error) {
	entries, err := os.ReadDir(h.Path)
	if err != nil {
		return nil, fmt.Errorf("listing quillit home at %s: %w", h.Path, err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(h.Path, e.Name(), ProjectConfigFileName)); err == nil {
			names = append(names, e.Name())
		}
	}
	return names, nil
}
