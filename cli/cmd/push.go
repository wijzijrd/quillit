package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/quillit/cli/internal/client"
	"github.com/quillit/cli/internal/pack"
)

var (
	pushApply        bool
	pushOnConflict   string
	pushCreateFacets bool
	pushOutput       string
	pushEntry        string
)

var pushCmd = &cobra.Command{
	Use:   "push [web_project]",
	Short: "Import this project into the web app (dry-run by default).",
	Long: `Pack the current project (or one entry via --entry) as a tarball and
import it into a web app project. Without --apply this is a dry run: the
report shows what would happen and nothing changes. web_project is the web
project's name or id; it defaults to the local project's name.

With --output the tarball is written to a file instead of uploaded — no
login needed.`,
	Example: `  quillit push --output backup.tgz
  quillit push curse-of-strahd
  quillit push curse-of-strahd --apply --on-conflict skip`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		h, p, err := resolveCurrentProject()
		if err != nil {
			return err
		}

		if pushOutput != "" {
			f, err := os.Create(pushOutput)
			if err != nil {
				return err
			}
			defer f.Close()
			if err := pack.Project(f, p.Root, pushEntry); err != nil {
				return err
			}
			fmt.Printf("Wrote %s\n", pushOutput)
			return nil
		}

		switch pushOnConflict {
		case "fail", "skip", "overwrite", "suffix":
		default:
			return fmt.Errorf("invalid --on-conflict %q: want fail, skip, overwrite, or suffix", pushOnConflict)
		}

		if h.Config.Server == "" || h.Config.SessionToken == "" {
			return errors.New("not logged in — run `quillit login --server <url>` first")
		}
		c := &client.Client{Server: h.Config.Server, Token: h.Config.SessionToken}

		target := p.Config.Name
		if len(args) == 1 {
			target = args[0]
		}
		projects, err := c.ListProjects()
		if err != nil {
			return err
		}
		webProject, err := matchWebProject(projects, target)
		if err != nil {
			return err
		}

		var buf bytes.Buffer
		if err := pack.Project(&buf, p.Root, pushEntry); err != nil {
			return err
		}

		mode := "dry-run"
		if pushApply {
			mode = "apply"
		}
		resp, err := c.Import(webProject.ID, &buf, client.ImportOptions{
			Mode: mode, OnConflict: pushOnConflict, CreateFacets: pushCreateFacets,
		})
		if err != nil {
			var ve *client.ValidationError
			if errors.As(err, &ve) {
				return fmt.Errorf("%s", renderValidationError(ve))
			}
			return err
		}
		fmt.Print(renderImportReport(resp))
		return nil
	},
}

// matchWebProject finds the target web project by exact name or id,
// failing loud with the available names (spec §6).
func matchWebProject(projects []client.Project, target string) (client.Project, error) {
	var names []string
	for _, p := range projects {
		if p.Name == target || p.ID == target {
			return p, nil
		}
		names = append(names, p.Name)
	}
	return client.Project{}, fmt.Errorf("no web project named %q — available: %s", target, strings.Join(names, ", "))
}

func renderImportReport(resp *client.ImportResponse) string {
	var b strings.Builder
	tw := tabwriter.NewWriter(&b, 2, 4, 2, ' ', 0)
	for _, row := range resp.Report {
		detail := row.Detail
		fmt.Fprintf(tw, "%s\t%s\t%s\n", row.Action, row.Path, detail)
	}
	tw.Flush()

	if len(resp.Facets.Created) > 0 {
		verb := "created"
		if !resp.Applied {
			verb = "would create"
		}
		fmt.Fprintf(&b, "facets %s: %s\n", verb, strings.Join(resp.Facets.Created, ", "))
	}
	uploaded, skipped := 0, 0
	for _, img := range resp.Images {
		if img.Uploaded {
			uploaded++
		} else if img.Detail != "" {
			skipped++
		}
	}
	if uploaded > 0 || skipped > 0 {
		fmt.Fprintf(&b, "images: %d uploaded, %d skipped\n", uploaded, skipped)
	}

	if resp.Applied {
		fmt.Fprintf(&b, "Imported %d entries.\n", len(resp.Report))
	} else {
		fmt.Fprintf(&b, "Dry run — nothing imported. Re-run with --apply to import.\n")
	}
	return b.String()
}

func renderValidationError(ve *client.ValidationError) string {
	var b strings.Builder
	b.WriteString("import rejected:\n")
	for _, e := range ve.Entries {
		fmt.Fprintf(&b, "  %s: %s\n", e.Path, e.Error)
	}
	if len(ve.MissingFacets) > 0 {
		fmt.Fprintf(&b, "  missing facets: %s\n", strings.Join(ve.MissingFacets, ", "))
		b.WriteString("  add them to the web project first, or re-run with --create-facets\n")
	}
	return b.String()
}

func init() {
	pushCmd.Flags().BoolVar(&pushApply, "apply", false, "Apply the import (default is a dry run)")
	pushCmd.Flags().StringVar(&pushOnConflict, "on-conflict", "fail", "Existing-entry handling: fail, skip, overwrite, or suffix")
	pushCmd.Flags().BoolVar(&pushCreateFacets, "create-facets", false, "Add facets missing from the web project's vocabulary instead of rejecting")
	pushCmd.Flags().StringVar(&pushOutput, "output", "", "Write the tarball to this file instead of uploading")
	pushCmd.Flags().StringVar(&pushEntry, "entry", "", "Push only this entry folder (path relative to project root)")
	rootCmd.AddCommand(pushCmd)
}
