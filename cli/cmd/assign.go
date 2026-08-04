package cmd

import (
	"fmt"

	"github.com/quillit/cli/internal/entry"
	"github.com/spf13/cobra"
)

var assignCmd = &cobra.Command{
	Use:     "assign <entry> <directory>",
	Short:   "Move an entry folder from the project root into an organizational directory.",
	Args:    cobra.ExactArgs(2),
	Example: `  quillit assign tom characters/npcs`,
	RunE: func(cmd *cobra.Command, args []string) error {
		name, directory := args[0], args[1]

		_, p, err := resolveCurrentProject()
		if err != nil {
			return err
		}

		dest, err := entry.Assign(p.Root, name, directory)
		if err != nil {
			return err
		}
		fmt.Printf("Assigned %q to %s\n", name, dest)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(assignCmd)
}
