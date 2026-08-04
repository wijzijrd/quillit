package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/quillit/cli/internal/entry"
	"github.com/quillit/cli/internal/scaffold"
	"github.com/spf13/cobra"
)

var createAssign bool

var createCmd = &cobra.Command{
	Use:   "create <entry_name> [directory]",
	Short: "Create an entry (folder + templated files) in the project root.",
	Long: `Create an entry: a folder named <entry_name> in the project root
containing four templated files (<entry_name>.md/.html/.css/.js) that are
usable immediately, without editing.

Use "quillit assign" afterward to file the entry into an organizational
directory, or pass -A here to do both in one step:

  quillit create tom -A characters/npcs   assign straight to a directory
  quillit create tom -A                   prompt for the directory
  quillit create tom                      leave it at the project root

[directory] is only meaningful together with -A; it's an error to pass it
without -A.`,
	Args:    cobra.RangeArgs(1, 2),
	Example: `  quillit create tom -A characters/npcs`,
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if len(args) == 2 && !createAssign {
			return fmt.Errorf("directory argument %q is only valid together with --assign/-A", args[1])
		}

		_, p, err := resolveCurrentProject()
		if err != nil {
			return err
		}

		entryDir := filepath.Join(p.Root, name)
		if _, statErr := os.Stat(entryDir); statErr == nil {
			return fmt.Errorf("entry %q already exists at %s", name, entryDir)
		}
		if err := os.MkdirAll(entryDir, 0o755); err != nil {
			return fmt.Errorf("creating entry directory: %w", err)
		}
		if err := scaffold.Write(entryDir, scaffold.Data{Name: name}); err != nil {
			return err
		}
		fmt.Printf("Created entry %q at %s\n", name, entryDir)

		if !createAssign {
			return nil
		}

		dir := ""
		if len(args) == 2 {
			dir = args[1]
		} else {
			dir, err = promptForDirectory()
			if err != nil {
				return err
			}
		}

		dest, err := entry.Assign(p.Root, name, dir)
		if err != nil {
			return err
		}
		fmt.Printf("Assigned %q to %s\n", name, dest)
		return nil
	},
}

func init() {
	createCmd.Flags().BoolVarP(&createAssign, "assign", "A", false, "assign into the given directory immediately; omit the directory to be prompted")
	rootCmd.AddCommand(createCmd)
}
