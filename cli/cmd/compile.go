package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/quillit/cli/internal/entry"
	"github.com/quillit/cli/internal/resolver"
	"github.com/quillit/contentengine/linkindex"
	"github.com/quillit/contentengine/parse"
	"github.com/spf13/cobra"
)

var compileAll bool

var compileCmd = &cobra.Command{
	Use:   "compile <path_to_entry>",
	Short: "Scan an entry's .md for wikilinks and write its links.conf index.",
	Long: `Scan an entry's .md for wikilinks, writing links.conf (target path,
label, containing card facet if any) into the entry folder.

render/export compare an entry's .md mtime against its links.conf and
recompile automatically when stale — compile is never required before
them, it just forces or warms the index (--all, e.g. before a session).

Links to non-existent entries are recorded, not dropped, and reported as
warnings, not errors.`,
	Args:    cobra.MaximumNArgs(1),
	Example: `  quillit compile characters/npcs/tom`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if compileAll && len(args) == 1 {
			return fmt.Errorf("give either <path_to_entry> or --all, not both")
		}
		if !compileAll && len(args) == 0 {
			return fmt.Errorf("give <path_to_entry>, or --all to compile every entry")
		}

		_, p, err := resolveCurrentProject()
		if err != nil {
			return err
		}

		var targets []string
		if compileAll {
			targets, err = entry.WalkAll(p.Root)
			if err != nil {
				return err
			}
		} else {
			targets = []string{args[0]}
		}

		var warnings []string
		for _, target := range targets {
			w, err := compileOne(p.Root, target)
			if err != nil {
				return err
			}
			warnings = append(warnings, w...)
		}

		label := "entries"
		if len(targets) == 1 {
			label = "entry"
		}
		fmt.Printf("Compiled %d %s.\n", len(targets), label)
		for _, w := range warnings {
			fmt.Println("warning:", w)
		}
		return nil
	},
}

// compileOne parses one entry's .md, extracts its wikilinks, resolves
// each against the project tree, and writes the result as links.conf.
func compileOne(projectRoot, entryPath string) (warnings []string, err error) {
	entryDir := filepath.Join(projectRoot, entryPath)
	name := filepath.Base(entryPath)
	mdPath := filepath.Join(entryDir, name+".md")

	data, err := os.ReadFile(mdPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", mdPath, err)
	}
	parsed, err := parse.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", mdPath, err)
	}

	records := linkindex.Extract(parsed)
	fsResolver := resolver.FS{ProjectRoot: projectRoot}
	for i := range records {
		exists, _ := fsResolver.Resolve(records[i].TargetPath)
		records[i].Resolved = exists
		if !exists {
			warnings = append(warnings, fmt.Sprintf("%s: link to non-existent entry %q", entryPath, records[i].TargetPath))
		}
	}

	encoded, err := linkindex.Encode(records)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(entryDir, "links.conf"), encoded, 0o644); err != nil {
		return nil, fmt.Errorf("writing links.conf for %s: %w", entryPath, err)
	}
	return warnings, nil
}

func init() {
	compileCmd.Flags().BoolVar(&compileAll, "all", false, "compile every entry in the current project")
	rootCmd.AddCommand(compileCmd)
}
