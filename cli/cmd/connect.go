package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/quillit/cli/internal/home"
	"github.com/spf13/cobra"
)

var connectCmd = &cobra.Command{
	Use:   "connect [project_name]",
	Short: "Set the current project, or print which project is connected.",
	Long: `Set the current project pointer in the home config, so commands work
from any directory (e.g. after closing your terminal).

With no argument, prints the currently connected project (or "none").

Being inside a project directory always takes priority over the connected
project (see project resolution order) — connect is a fallback for working
from elsewhere.`,
	Args:    cobra.MaximumNArgs(1),
	Example: `  quillit connect curse-of-strahd`,
	RunE: func(cmd *cobra.Command, args []string) error {
		h, err := requireHome()
		if err != nil {
			return err
		}

		if len(args) == 0 {
			if h.Config.CurrentProject == "" {
				fmt.Println("none")
			} else {
				fmt.Println(h.Config.CurrentProject)
			}
			return nil
		}

		name := args[0]
		if !home.IsProjectRoot(filepath.Join(h.Path, name)) {
			available, listErr := h.ListProjects()
			if listErr != nil {
				return listErr
			}
			return home.ErrProjectNotFound{Name: name, Available: available}
		}

		if err := h.SetCurrentProject(name); err != nil {
			return err
		}
		fmt.Printf("Connected to %q\n", name)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(connectCmd)
}
