package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/quillit/cli/internal/home"
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
