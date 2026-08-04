// Package scaffold renders the four working template files that make up a
// fresh entry folder, per docs/cli-spec.md §5 "Entry folder anatomy".
package scaffold

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

//go:embed templates/*.tmpl
var templatesFS embed.FS

// Data is the template context for a single entry's scaffold files.
type Data struct {
	// Name is the entry name: folder + file basename, and the value
	// written into the .md frontmatter and .html <title>.
	Name string
}

var files = []struct {
	tmpl string // filename under templates/
	ext  string // output file extension (without the dot)
}{
	{"entry.md.tmpl", "md"},
	{"entry.html.tmpl", "html"},
	{"entry.css.tmpl", "css"},
	{"entry.js.tmpl", "js"},
}

// Write renders the four working template files into dir as
// <name>.md/.html/.css/.js. dir must already exist. Does not create
// links.conf — that's only ever written by compile/render/export
// (CLI spec §5), never by create.
func Write(dir string, data Data) error {
	for _, f := range files {
		content, err := templatesFS.ReadFile("templates/" + f.tmpl)
		if err != nil {
			return fmt.Errorf("reading embedded template %s: %w", f.tmpl, err)
		}

		t, err := template.New(f.tmpl).Parse(string(content))
		if err != nil {
			return fmt.Errorf("parsing template %s: %w", f.tmpl, err)
		}

		outPath := filepath.Join(dir, fmt.Sprintf("%s.%s", data.Name, f.ext))
		out, err := os.Create(outPath)
		if err != nil {
			return fmt.Errorf("creating %s: %w", outPath, err)
		}
		err = t.Execute(out, data)
		closeErr := out.Close()
		if err != nil {
			return fmt.Errorf("rendering %s: %w", outPath, err)
		}
		if closeErr != nil {
			return fmt.Errorf("closing %s: %w", outPath, closeErr)
		}
	}
	return nil
}
