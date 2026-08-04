package cmd

import (
	"fmt"
	"os"
	"path/filepath"

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

var configAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a facet to the global or a project's facet vocabulary.",
	Long: `Add a facet to the global vocabulary, or to a project's extra_facets
with --scope <project_name>. --scope global is the default.

Facet names must be kebab-case: lowercase letters, digits, and hyphens
only. Adding a facet already present in the effective vocabulary at the
target scope is a no-op, not an error.`,
	Example: `  quillit config add --facet stat-block --scope curse-of-strahd`,
	RunE: func(cmd *cobra.Command, args []string) error {
		name, scope, err := facetFlags(cmd)
		if err != nil {
			return err
		}
		if !home.ValidFacetName(name) {
			return fmt.Errorf("invalid facet name %q: must be kebab-case (lowercase letters, digits, hyphens)", name)
		}

		h, err := requireHome()
		if err != nil {
			return err
		}

		if scope == "global" {
			return addGlobalFacet(h, name)
		}
		return addProjectFacet(h, scope, name)
	},
}

var configRmCmd = &cobra.Command{
	Use:   "rm",
	Short: "Remove a facet from the global or a project's facet vocabulary.",
	Long: `Remove a facet from the chosen scope (--scope global by default, or a
project name). This never touches any .md files — entries still using
the facet will fail loud at a later render (golden rule 6); a reminder
is printed after a successful removal.`,
	Example: `  quillit config rm --facet stat-block --scope curse-of-strahd`,
	RunE: func(cmd *cobra.Command, args []string) error {
		name, scope, err := facetFlags(cmd)
		if err != nil {
			return err
		}

		h, err := requireHome()
		if err != nil {
			return err
		}

		if scope == "global" {
			return rmGlobalFacet(h, name)
		}
		return rmProjectFacet(h, scope, name)
	},
}

// facetFlags reads --facet/--scope, defaulting scope to "global".
func facetFlags(cmd *cobra.Command) (name, scope string, err error) {
	name, err = cmd.Flags().GetString("facet")
	if err != nil {
		return "", "", err
	}
	scope, err = cmd.Flags().GetString("scope")
	if err != nil {
		return "", "", err
	}
	if scope == "" {
		scope = "global"
	}
	return name, scope, nil
}

func addGlobalFacet(h *home.Home, name string) error {
	if contains(h.Config.Facets, name) {
		fmt.Printf("Facet %q already in the global vocabulary; nothing to do.\n", name)
		return nil
	}
	h.Config.Facets = append(h.Config.Facets, name)
	if err := h.Save(); err != nil {
		return err
	}
	fmt.Printf("Added %q to the global facet vocabulary.\n", name)
	return nil
}

func addProjectFacet(h *home.Home, projectName, name string) error {
	p, err := loadNamedProject(h, projectName)
	if err != nil {
		return err
	}
	if contains(p.EffectiveFacets(h.Config.Facets), name) {
		fmt.Printf("Facet %q already in %q's effective vocabulary; nothing to do.\n", name, projectName)
		return nil
	}
	p.Config.ExtraFacets = append(p.Config.ExtraFacets, name)
	if err := p.Save(); err != nil {
		return err
	}
	fmt.Printf("Added %q to %q's extra facets.\n", name, projectName)
	return nil
}

func rmGlobalFacet(h *home.Home, name string) error {
	idx := indexOf(h.Config.Facets, name)
	if idx == -1 {
		fmt.Printf("Facet %q not found in the global vocabulary; nothing to remove.\n", name)
		return nil
	}
	h.Config.Facets = append(h.Config.Facets[:idx], h.Config.Facets[idx+1:]...)
	if err := h.Save(); err != nil {
		return err
	}
	fmt.Printf("Removed %q from the global facet vocabulary.\n", name)
	printFacetRemovalReminder(name)
	return nil
}

func rmProjectFacet(h *home.Home, projectName, name string) error {
	p, err := loadNamedProject(h, projectName)
	if err != nil {
		return err
	}
	idx := indexOf(p.Config.ExtraFacets, name)
	if idx == -1 {
		fmt.Printf("Facet %q not found in %q's extra facets; nothing to remove.\n", name, projectName)
		return nil
	}
	p.Config.ExtraFacets = append(p.Config.ExtraFacets[:idx], p.Config.ExtraFacets[idx+1:]...)
	if err := p.Save(); err != nil {
		return err
	}
	fmt.Printf("Removed %q from %q's extra facets.\n", name, projectName)
	printFacetRemovalReminder(name)
	return nil
}

func printFacetRemovalReminder(name string) {
	fmt.Printf("Note: entries still using the %q facet will fail loud at render until updated.\n", name)
}

// loadNamedProject resolves a project by name under h — used for
// --scope <project_name>, distinct from resolveCurrentProject's cwd-based
// resolution.
func loadNamedProject(h *home.Home, name string) (*home.Project, error) {
	root := filepath.Join(h.Path, name)
	if !home.IsProjectRoot(root) {
		available, err := h.ListProjects()
		if err != nil {
			return nil, err
		}
		return nil, home.ErrProjectNotFound{Name: name, Available: available}
	}
	return home.LoadProject(root)
}

func contains(list []string, s string) bool {
	return indexOf(list, s) != -1
}

func indexOf(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return -1
}

func init() {
	for _, c := range []*cobra.Command{configAddCmd, configRmCmd} {
		c.Flags().String("facet", "", "facet name (kebab-case)")
		c.Flags().String("scope", "global", `scope: "global" or a project name`)
		_ = c.MarkFlagRequired("facet")
	}
	configCmd.AddCommand(configAddCmd, configRmCmd)
	rootCmd.AddCommand(configCmd)
}
