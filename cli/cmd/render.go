package cmd

import (
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/quillit/cli/internal/resolver"
	"github.com/quillit/contentengine/filter"
	"github.com/quillit/contentengine/parse"
	"github.com/quillit/contentengine/render"
	"github.com/spf13/cobra"
)

const entryContentMarker = `<main id="quillit-entry-content">`

var (
	renderView  string
	renderCard  string
	renderQuick bool
)

var renderCmd = &cobra.Command{
	Use:   "render <path_to_entry>",
	Short: "Build an entry into HTML and open it in your browser.",
	Long: `Build an entry's .md into HTML using its own scaffolding files, and
open the result in your default browser. Default view is "dm".

--view and --card are mutually exclusive; --card implies DM audience —
secrets inside a matching card block are still included, since card view
is a DM tool.`,
	Args:    cobra.ExactArgs(1),
	Example: `  quillit render characters/npcs/tom --card motivation`,
	RunE: func(cmd *cobra.Command, args []string) error {
		entryPath := args[0]

		if cmd.Flags().Changed("view") && cmd.Flags().Changed("card") {
			return fmt.Errorf("--view and --card are mutually exclusive")
		}
		view, err := buildView(cmd)
		if err != nil {
			return err
		}

		h, p, err := resolveCurrentProject()
		if err != nil {
			return err
		}

		entryDir := filepath.Join(p.Root, entryPath)
		name := filepath.Base(entryPath)
		mdPath := filepath.Join(entryDir, name+".md")

		data, err := os.ReadFile(mdPath)
		if err != nil {
			return fmt.Errorf("entry path not found: checked %s", mdPath)
		}
		entry, err := parse.Parse(data)
		if err != nil {
			return err
		}

		vocabulary := p.EffectiveFacets(h.Config.Facets)
		filtered, err := filter.Filter(entry, view, vocabulary)
		if err != nil {
			return err
		}

		// Lazy auto-refresh (CLI spec §7 "compile"): recompile links.conf
		// first if the .md is newer (or links.conf doesn't exist yet).
		confPath := filepath.Join(entryDir, "links.conf")
		stale, err := resolver.IsStale(mdPath, confPath)
		if err != nil {
			return err
		}
		if stale {
			if _, err := compileOne(p.Root, entryPath); err != nil {
				return err
			}
		}

		fragment, err := render.Render(filtered, view, resolver.FS{ProjectRoot: p.Root}, clipboardLinkRenderer)
		if err != nil {
			return err
		}

		htmlPath := filepath.Join(entryDir, name+".html")
		if err := injectFragment(htmlPath, fragment); err != nil {
			return err
		}

		return openInBrowser(htmlPath)
	},
}

// buildView translates the --view/--card/-Q flags into a filter.View,
// per CLI spec §7 "render".
func buildView(cmd *cobra.Command) (filter.View, error) {
	if cmd.Flags().Changed("card") {
		return filter.View{Kind: filter.ViewCard, Facet: renderCard, Quick: renderQuick}, nil
	}
	switch renderView {
	case "", "dm":
		return filter.View{Kind: filter.ViewDM, Quick: renderQuick}, nil
	case "player":
		return filter.View{Kind: filter.ViewPlayer, Quick: renderQuick}, nil
	default:
		return filter.View{}, fmt.Errorf(`unknown --view %q: must be "dm" or "player"`, renderView)
	}
}

// injectFragment replaces the content of the entry's .html scaffolding's
// #quillit-entry-content marker (see internal/scaffold's entry.html.tmpl)
// with fragment, and writes the result back to the same file — the
// entry's own .html is the render target CLI spec §5's entry folder
// anatomy describes: "render scaffolding... the render command injects
// into", overwritten fresh on every render rather than accumulating
// separate per-view output files.
func injectFragment(htmlPath, fragment string) error {
	data, err := os.ReadFile(htmlPath)
	if err != nil {
		return fmt.Errorf("reading entry scaffolding %s: %w", htmlPath, err)
	}
	tmpl := string(data)

	start := strings.Index(tmpl, entryContentMarker)
	if start == -1 {
		return fmt.Errorf("%s: missing %s marker (entry scaffolding may be corrupted)", htmlPath, entryContentMarker)
	}
	contentStart := start + len(entryContentMarker)
	rest := tmpl[contentStart:]

	end := strings.Index(rest, "</main>")
	if end == -1 {
		return fmt.Errorf(`%s: unclosed <main id="quillit-entry-content">`, htmlPath)
	}

	out := tmpl[:contentStart] + "\n" + fragment + rest[end:]
	return os.WriteFile(htmlPath, []byte(out), 0o644)
}

// clipboardLinkRenderer is the CLI's presentation of a non-expanded
// wikilink (render.LinkRenderer, see contentengine/render): a span whose
// data-quillit-command attribute holds the "quillit render ..." command
// to copy, wired up by the click handler already in every entry's .js
// scaffolding (see internal/scaffold's entry.js.tmpl). CLI spec §4: a
// link the resolver found but which has no card for the requested facet
// gets a small note instead of failing.
func clipboardLinkRenderer(info render.LinkInfo) string {
	command := "quillit render " + info.Path
	if info.Facet != "" {
		command += " --card " + info.Facet
	}

	class := "clipboard-link"
	var note string
	if info.NoMatchingCard {
		class += " clipboard-link--no-card"
		note = fmt.Sprintf(` <span class="clipboard-link-note">(no %s card)</span>`, html.EscapeString(info.Facet))
	}

	return fmt.Sprintf(
		`<span class="%s" data-quillit-command="%s" title="%s">%s</span>%s`,
		class,
		html.EscapeString(command),
		html.EscapeString("Click to copy: "+command),
		html.EscapeString(info.Label),
		note,
	)
}

// openInBrowser opens path with the OS's default handler for its file
// type — for an .html file, the default web browser. Unlike `edit`,
// this deliberately ignores $VISUAL/$EDITOR: those name a text editor,
// not a browser.
func openInBrowser(path string) error {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name = "open"
	case "windows":
		name, args = "cmd", []string{"/c", "start", ""}
	default:
		name = "xdg-open"
	}
	c := exec.Command(name, append(args, path)...)
	return c.Run()
}

func init() {
	renderCmd.Flags().StringVar(&renderView, "view", "dm", `view: "dm" or "player"`)
	renderCmd.Flags().StringVar(&renderCard, "card", "", "render only the flash card for this facet")
	renderCmd.Flags().BoolVarP(&renderQuick, "quick-view", "Q", false, "quick view: frontmatter summary + first section only")
	rootCmd.AddCommand(renderCmd)
}
