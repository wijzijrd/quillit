// Package entry implements entry-folder operations shared by the create,
// assign, and edit commands.
package entry

import (
	"fmt"
	"os"
	"path/filepath"
)

// Assign moves the entry folder named name from the project root into
// projectRoot/directory (created if missing), per docs/cli-spec.md §7
// "assign". Fails clearly, with no partial move, if the entry isn't at the
// project root or an entry of the same name already exists at the
// destination.
func Assign(projectRoot, name, directory string) (newPath string, err error) {
	src := filepath.Join(projectRoot, name)
	if info, statErr := os.Stat(src); statErr != nil || !info.IsDir() {
		return "", fmt.Errorf("entry %q not found at project root (checked %s)", name, src)
	}

	destDir := filepath.Join(projectRoot, directory)
	dest := filepath.Join(destDir, name)
	if _, statErr := os.Stat(dest); statErr == nil {
		return "", fmt.Errorf("entry %q already exists at %s", name, dest)
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("creating directory %s: %w", destDir, err)
	}
	if err := os.Rename(src, dest); err != nil {
		return "", fmt.Errorf("moving entry %q to %s: %w", name, dest, err)
	}
	return dest, nil
}
