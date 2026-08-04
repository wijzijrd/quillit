// Package entry implements entry-folder operations shared by the create,
// assign, and edit commands.
package entry

import (
	"fmt"
	"io/fs"
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

// WalkAll returns the project-root-relative path of every entry folder
// under projectRoot, at any depth — any directory containing a file
// named "<its own basename>.md", per CLI spec §5's entry folder anatomy.
// Used by `compile --all` and (later) `export` with no path.
func WalkAll(projectRoot string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(projectRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == projectRoot || !d.IsDir() {
			return nil
		}
		name := filepath.Base(p)
		if _, statErr := os.Stat(filepath.Join(p, name+".md")); statErr == nil {
			rel, relErr := filepath.Rel(projectRoot, p)
			if relErr != nil {
				return relErr
			}
			paths = append(paths, rel)
		}
		return nil
	})
	return paths, err
}
