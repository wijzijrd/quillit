package cmd

import (
	"fmt"
	"os"

	"github.com/quillit/cli/internal/home"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Print global and project config, or manage the facet vocabulary.",
	Long: `With no subcommand, prints the global config, the resolved current
project's config, and the effective facet vocabulary (global union project
extras), clearly labeled.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		h, err := requireHome()
		if err != nil {
			return err
		}

		fmt.Println("Global config ($QUILLIT_HOME/config.yaml):")
		fmt.Printf("  facets: %v\n", h.Config.Facets)
		if h.Config.CurrentProject == "" {
			fmt.Println("  current_project: (none)")
		} else {
			fmt.Printf("  current_project: %s\n", h.Config.CurrentProject)
		}

		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		p, err := home.ResolveProject(cwd, h)
		if err != nil {
			fmt.Println()
			fmt.Println("No project resolved (not inside one, and none connected).")
			return nil
		}

		fmt.Println()
		fmt.Printf("Project config (%s):\n", p.Root)
		fmt.Printf("  name: %s\n", p.Config.Name)
		fmt.Printf("  extra_facets: %v\n", p.Config.ExtraFacets)
		fmt.Println()
		fmt.Printf("Effective facet vocabulary: %v\n", p.EffectiveFacets(h.Config.Facets))
		return nil
	},
}

// configAddCmd/configRmCmd establish the final command-tree shape now;
// full add/rm behavior (kebab-case validation, scope handling, no-op-on-
// duplicate) lands in a later issue.
var configAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a facet to the global or a project's vocabulary (not yet implemented).",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("quillit config add: not yet implemented")
		return nil
	},
}

var configRmCmd = &cobra.Command{
	Use:   "rm",
	Short: "Remove a facet from the global or a project's vocabulary (not yet implemented).",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("quillit config rm: not yet implemented")
		return nil
	},
}

func init() {
	for _, c := range []*cobra.Command{configAddCmd, configRmCmd} {
		c.Flags().String("facet", "", "facet name (kebab-case)")
		c.Flags().String("scope", "global", `scope: "global" or a project name`)
	}
	configCmd.AddCommand(configAddCmd, configRmCmd)
	rootCmd.AddCommand(configCmd)
}
