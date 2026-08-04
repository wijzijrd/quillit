package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/quillit/cli/internal/entry"
	"github.com/quillit/cli/internal/home"
	"github.com/quillit/contentengine/export"
	"github.com/quillit/contentengine/filter"
	"github.com/quillit/contentengine/linkindex"
	"github.com/spf13/cobra"
)

var (
	exportView         string
	exportCard         string
	exportWithLinks    bool
	exportWithLinksAll bool
	exportWith         string
)

var exportCmd = &cobra.Command{
	Use:   "export [path_to_entry]",
	Short: "Render an entry (or every entry) to PDF.",
	Long: `Same pipeline as render, but outputs a PDF next to the entry instead of
opening a browser. With no path, exports every entry in the project to a
project-level exports/ directory, applying the given view flags to each
(bundling flags are invalid in that bulk mode).

PDFs never expand wikilinks or show clipboard-link affordances — those
are browser-only. A link's plain text label is all that ever appears,
regardless of --view.

Bundling a single-entry export combines linked entries into one PDF,
main entry first: --with-links prompts interactively (reading the
entry's links.conf); --with-links-all bundles every resolved outgoing
link with no prompt; --with <entries> takes an explicit comma-separated
list. (CLI spec §7 documents this as one flag, "--with-links [all]" —
split here into --with-links/--with-links-all instead, the same fix
applied to create's -A in #22: pflag's optional-value flags only parse
--flag=value unambiguously, not the space-separated form the spec's
own examples use.)`,
	Args:    cobra.MaximumNArgs(1),
	Example: `  quillit export characters/npcs/tom --view player --with-links-all`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Flags().Changed("view") && cmd.Flags().Changed("card") {
			return fmt.Errorf("--view and --card are mutually exclusive")
		}
		view, err := buildExportView(cmd)
		if err != nil {
			return err
		}

		h, p, err := resolveCurrentProject()
		if err != nil {
			return err
		}

		bundling := exportWithLinks || exportWithLinksAll || exportWith != ""

		if len(args) == 0 {
			if bundling {
				return fmt.Errorf("--with-links/--with-links-all/--with are not valid for bulk export (no path given)")
			}
			return exportAll(p, h, view)
		}

		return exportOne(p, h, args[0], view, bundling)
	},
}

// buildExportView mirrors render's buildView, using export's own flag
// vars — export has no -Q per CLI spec §6's command table.
func buildExportView(cmd *cobra.Command) (filter.View, error) {
	if cmd.Flags().Changed("card") {
		return filter.View{Kind: filter.ViewCard, Facet: exportCard}, nil
	}
	switch exportView {
	case "", "dm":
		return filter.View{Kind: filter.ViewDM}, nil
	case "player":
		return filter.View{Kind: filter.ViewPlayer}, nil
	default:
		return filter.View{}, fmt.Errorf(`unknown --view %q: must be "dm" or "player"`, exportView)
	}
}

// exportOne exports a single entry, optionally bundled with linked
// entries, to <entryDir>/<name>.pdf.
func exportOne(p *home.Project, h *home.Home, entryPath string, view filter.View, bundling bool) error {
	vocabulary := p.EffectiveFacets(h.Config.Facets)

	mainFiltered, err := loadFilteredEntry(p.Root, entryPath, view, vocabulary)
	if err != nil {
		return err
	}
	filtered := []*filter.FilteredEntry{mainFiltered}

	if bundling {
		targets, err := resolveBundleTargets(p.Root, entryPath)
		if err != nil {
			return err
		}
		for _, t := range targets {
			f, err := loadFilteredEntry(p.Root, t, view, vocabulary)
			if err != nil {
				return err
			}
			filtered = append(filtered, f)
		}
	}

	data, err := export.Bundle(filtered)
	if err != nil {
		return err
	}

	outPath := filepath.Join(p.Root, entryPath, filepath.Base(entryPath)+".pdf")
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", outPath, err)
	}
	fmt.Println("Exported", outPath)
	return nil
}

// exportAll exports every entry in the project to <projectRoot>/exports/,
// one PDF per entry, applying view to all of them. No bundling — CLI
// spec §7: "--with-links invalid there."
func exportAll(p *home.Project, h *home.Home, view filter.View) error {
	targets, err := entry.WalkAll(p.Root)
	if err != nil {
		return err
	}
	vocabulary := p.EffectiveFacets(h.Config.Facets)

	exportsDir := filepath.Join(p.Root, "exports")
	if err := os.MkdirAll(exportsDir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", exportsDir, err)
	}

	for _, target := range targets {
		filtered, err := loadFilteredEntry(p.Root, target, view, vocabulary)
		if err != nil {
			return err
		}
		data, err := export.Bundle([]*filter.FilteredEntry{filtered})
		if err != nil {
			return err
		}
		outName := strings.ReplaceAll(target, string(filepath.Separator), "-") + ".pdf"
		outPath := filepath.Join(exportsDir, outName)
		if err := os.WriteFile(outPath, data, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", outPath, err)
		}
		fmt.Println("Exported", outPath)
	}
	return nil
}

// resolveBundleTargets figures out which linked entries to bundle,
// per the three flags: explicit --with list, --with-links-all (every
// resolved outgoing link, no prompt), or --with-links (interactive
// checklist). Assumes the main entry's links.conf is already fresh
// (loadFilteredEntry, called before this in exportOne, ensures that).
func resolveBundleTargets(projectRoot, entryPath string) ([]string, error) {
	if exportWith != "" {
		var targets []string
		for _, part := range strings.Split(exportWith, ",") {
			if t := strings.TrimSpace(part); t != "" {
				targets = append(targets, t)
			}
		}
		return targets, nil
	}

	confPath := filepath.Join(projectRoot, entryPath, "links.conf")
	data, err := os.ReadFile(confPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", confPath, err)
	}
	records, err := linkindex.Decode(data)
	if err != nil {
		return nil, err
	}

	var candidates []string
	seen := map[string]bool{}
	for _, r := range records {
		if r.Resolved && !seen[r.TargetPath] {
			seen[r.TargetPath] = true
			candidates = append(candidates, r.TargetPath)
		}
	}

	if exportWithLinksAll {
		return candidates, nil
	}
	return promptChecklist(candidates)
}

// promptChecklist reads a comma-separated selection of 1-based indices
// from stdin against candidates — --with-links' interactive mode.
func promptChecklist(candidates []string) ([]string, error) {
	if len(candidates) == 0 {
		fmt.Println("No resolved outgoing links to bundle.")
		return nil, nil
	}

	fmt.Println("Select entries to bundle (comma-separated numbers, or blank for none):")
	for i, c := range candidates {
		fmt.Printf("  %d) %s\n", i+1, c)
	}
	fmt.Print("> ")

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return nil, err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, nil
	}

	var selected []string
	for _, tok := range strings.Split(line, ",") {
		tok = strings.TrimSpace(tok)
		idx, err := strconv.Atoi(tok)
		if err != nil || idx < 1 || idx > len(candidates) {
			return nil, fmt.Errorf("invalid selection %q", tok)
		}
		selected = append(selected, candidates[idx-1])
	}
	return selected, nil
}

func init() {
	exportCmd.Flags().StringVar(&exportView, "view", "dm", `view: "dm" or "player"`)
	exportCmd.Flags().StringVar(&exportCard, "card", "", "export only the flash card for this facet")
	exportCmd.Flags().BoolVar(&exportWithLinks, "with-links", false, "interactively choose linked entries to bundle into one PDF")
	exportCmd.Flags().BoolVar(&exportWithLinksAll, "with-links-all", false, "bundle every resolved outgoing link into one PDF, no prompt")
	exportCmd.Flags().StringVar(&exportWith, "with", "", "comma-separated entry paths to bundle into one PDF")
	rootCmd.AddCommand(exportCmd)
}
