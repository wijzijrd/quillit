package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
)

var editCmd = &cobra.Command{
	Use:     "edit <path_to_entry>",
	Short:   "Open an entry folder in your editor ($VISUAL/$EDITOR, falling back to the OS default opener).",
	Args:    cobra.ExactArgs(1),
	Example: `  quillit edit characters/npcs/tom`,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, p, err := resolveCurrentProject()
		if err != nil {
			return err
		}

		entryPath := filepath.Join(p.Root, args[0])
		info, statErr := os.Stat(entryPath)
		if statErr != nil || !info.IsDir() {
			return fmt.Errorf("entry path not found: checked %s", entryPath)
		}

		name, editorArgs := resolveEditor()
		c := exec.Command(name, append(editorArgs, entryPath)...)
		c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
		return c.Run()
	},
}

// resolveEditor picks $VISUAL, then $EDITOR, then an OS-appropriate default
// opener, per docs/cli-spec.md §7 "edit".
func resolveEditor() (name string, args []string) {
	if v := os.Getenv("VISUAL"); v != "" {
		return v, nil
	}
	if e := os.Getenv("EDITOR"); e != "" {
		return e, nil
	}
	switch runtime.GOOS {
	case "darwin":
		return "open", nil
	case "windows":
		return "cmd", []string{"/c", "start", ""}
	default:
		return "xdg-open", nil
	}
}

func init() {
	rootCmd.AddCommand(editCmd)
}
