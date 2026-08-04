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
