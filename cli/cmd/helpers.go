package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/quillit/cli/internal/home"
	"github.com/quillit/cli/internal/resolver"
	"github.com/quillit/contentengine/filter"
	"github.com/quillit/contentengine/parse"
)

// requireHome loads $QUILLIT_HOME for commands that need global config but
// must not bootstrap it themselves — every command except `init`.
func requireHome() (*home.Home, error) {
	path, set := home.Locate()
	if !set || !home.IsBootstrapped(path) {
		return nil, home.ErrHomeNotBootstrapped{}
	}
	return home.Load(path)
}

// resolveCurrentProject loads the home and resolves the project for the
// current working directory — the pattern every entry-touching command
// (create, assign, edit, and later render/compile/export) needs first.
func resolveCurrentProject() (*home.Home, *home.Project, error) {
	h, err := requireHome()
	if err != nil {
		return nil, nil, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, nil, err
	}
	p, err := home.ResolveProject(cwd, h)
	if err != nil {
		return nil, nil, err
	}
	return h, p, nil
}

// promptForDirectory is used by `create -A` when -A is given with no
// directory value, per CLI spec §7 "create".
func promptForDirectory() (string, error) {
	fmt.Print("Assign to directory: ")
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	dir := strings.TrimSpace(line)
	if dir == "" {
		return "", fmt.Errorf("no directory given")
	}
	return dir, nil
}

// loadFilteredEntry reads and parses an entry's .md, applies
// filter.Filter for view, and recompiles the entry's links.conf first if
// it's stale (CLI spec §7 "compile"'s lazy auto-refresh) — the shared
// first half of both render's and export's pipelines.
func loadFilteredEntry(projectRoot, entryPath string, view filter.View, vocabulary []string) (*filter.FilteredEntry, error) {
	entryDir := filepath.Join(projectRoot, entryPath)
	name := filepath.Base(entryPath)
	mdPath := filepath.Join(entryDir, name+".md")

	data, err := os.ReadFile(mdPath)
	if err != nil {
		return nil, fmt.Errorf("entry path not found: checked %s", mdPath)
	}
	entry, err := parse.Parse(data)
	if err != nil {
		return nil, err
	}

	filtered, err := filter.Filter(entry, view, vocabulary)
	if err != nil {
		return nil, err
	}

	confPath := filepath.Join(entryDir, "links.conf")
	stale, err := resolver.IsStale(mdPath, confPath)
	if err != nil {
		return nil, err
	}
	if stale {
		if _, err := compileOne(projectRoot, entryPath); err != nil {
			return nil, err
		}
	}

	return filtered, nil
}

// stdinPrompt is the interactive home.Prompter used by `init` when
// $QUILLIT_HOME is unset. An empty response (including EOF, e.g. in a
// non-interactive test) accepts defaultPath.
func stdinPrompt(defaultPath string) (string, error) {
	fmt.Printf("Quillit home location [%s]: ", defaultPath)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
