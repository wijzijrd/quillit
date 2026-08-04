package cmd

import (
	"fmt"

	"github.com/quillit/cli/internal/home"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init <project_name>",
	Short: "Create a project inside $QUILLIT_HOME (config + starter folders).",
	Long: `Create a project inside $QUILLIT_HOME (config + starter folders).

If $QUILLIT_HOME is unset, or set but its directory is missing, init
bootstraps the home directory first: it creates the directory, seeds
config.yaml with the default facet vocabulary, and prints the shell export
line you need to add to your profile (the CLI cannot persist environment
variables for your shell itself).

The new project is created regardless of your current directory, and is
set as the current project afterward (same effect as running
"quillit connect <project_name>").`,
	Args: cobra.ExactArgs(1),
	Example: `  quillit init curse-of-strahd`,
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		envPath, envSet := home.Locate()
		var h *home.Home
		if envSet && home.IsBootstrapped(envPath) {
			loaded, err := home.Load(envPath)
			if err != nil {
				return err
			}
			h = loaded
		} else {
			bootstrapped, exportLine, err := home.Bootstrap(envPath, envSet, stdinPrompt)
			if err != nil {
				return err
			}
			h = bootstrapped
			fmt.Println("Bootstrapped quillit home at", h.Path)
			fmt.Println("Add this to your shell profile so it persists across sessions:")
			fmt.Println(" ", exportLine)
		}

		p, err := home.CreateProject(h, name)
		if err != nil {
			return err
		}
		if err := h.SetCurrentProject(name); err != nil {
			return err
		}

		fmt.Printf("Created project %q at %s\n", name, p.Root)
		fmt.Printf("Connected to %q\n", name)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
