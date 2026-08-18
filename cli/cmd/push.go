package cmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/quillit/cli/internal/client"
	"github.com/quillit/cli/internal/entry"
	"github.com/quillit/cli/internal/pack"
)

var (
	pushApply        bool
	pushOnConflict   string
	pushCreateFacets bool
	pushOutput       string
	pushEntry        string
	pushDelta        bool
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

		if pushDelta && pushEntry != "" {
			return errors.New("--delta and --entry are mutually exclusive")
		}
		if pushDelta && pushOutput != "" {
			return errors.New("--delta and --output are mutually exclusive — delta needs a live comparison against a logged-in web project")
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

		if pushDelta && !cmd.Flags().Changed("on-conflict") {
			pushOnConflict = "overwrite"
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
		if pushDelta {
			local, err := localEntryHashes(p.Root)
			if err != nil {
				return err
			}
			remote, err := c.ListEntries(webProject.ID)
			if err != nil {
				return err
			}
			plan := classifyDelta(local, remote)
			fmt.Printf("%d changed, %d new, %d unchanged (skipped)\n", plan.changedCount, plan.newCount, plan.unchangedCount)
			if len(plan.toPush) == 0 {
				fmt.Printf("Nothing to push — %d entries all unchanged.\n", plan.unchangedCount)
				return nil
			}
			if err := pack.Selected(&buf, p.Root, plan.toPush); err != nil {
				return err
			}
		} else {
			if err := pack.Project(&buf, p.Root, pushEntry); err != nil {
				return err
			}
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
		fmt.Print(renderImportReport(resp, pushApply))
		return nil
	},
}

// entryPath joins a directoryPath/slug pair into the same project-root-
// relative path shape entry.WalkAll returns — content-svc's inverse,
// entryPathOf (app/content/internal/handler/importer.go), does the same
// join, so a remote EntryMeta's path and a local WalkAll path are
// directly comparable once both go through this.
func entryPath(dir, slug string) string {
	if dir == "" {
		return slug
	}
	return dir + "/" + slug
}

// hasDotSegment reports whether any "/"-separated segment of the
// slash-normalized relative path rel starts with ".". pack.walkOne skips
// dot-prefixed directories below its start point, so any entry path with
// a dot segment is one a plain push would never include — --delta must
// agree, or it can select paths pack.Selected then can't actually pack
// without content-svc's import validation rejecting the dot segment.
func hasDotSegment(rel string) bool {
	for _, seg := range strings.Split(filepath.ToSlash(rel), "/") {
		if strings.HasPrefix(seg, ".") {
			return true
		}
	}
	return false
}

// localEntryHashes walks every entry folder under root (entry.WalkAll)
// and returns a map of project-root-relative path -> SHA-256 hex hash of
// that entry's raw .md bytes — the exact bytes an unmodified push would
// upload, so directly comparable to content-svc's stored body_hash.
//
// entry.WalkAll doesn't skip dot-prefixed directories, but pack.walkOne
// does (below its start point) — so entries under a dot-prefixed
// directory (e.g. ".backup/tom") are filtered out here, keeping the set
// --delta ever considers identical to what a plain push would include.
func localEntryHashes(root string) (map[string]string, error) {
	paths, err := entry.WalkAll(root)
	if err != nil {
		return nil, err
	}
	hashes := make(map[string]string, len(paths))
	for _, rel := range paths {
		if hasDotSegment(rel) {
			continue
		}
		mdPath := filepath.Join(root, filepath.FromSlash(rel), filepath.Base(rel)+".md")
		data, err := os.ReadFile(mdPath)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(data)
		hashes[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
	}
	return hashes, nil
}

// deltaPlan is classifyDelta's result: which local entry paths need
// packing, and the counts for the user-facing summary line.
type deltaPlan struct {
	toPush                                 []string
	newCount, changedCount, unchangedCount int
}

// classifyDelta compares local entry hashes against the server's current
// state, one local entry at a time. A remote entry with an empty BodyHash
// (unset — either pre-#126-migration and never touched since, or the
// server just doesn't know) is always treated as changed, never skipped
// — see #126's design doc: "unknown is always changed."
func classifyDelta(local map[string]string, remote []client.EntryMeta) deltaPlan {
	remoteHash := make(map[string]string, len(remote))
	for _, e := range remote {
		remoteHash[entryPath(e.DirectoryPath, e.Slug)] = e.BodyHash
	}

	var plan deltaPlan
	for path, hash := range local {
		rh, exists := remoteHash[path]
		switch {
		case !exists:
			plan.newCount++
			plan.toPush = append(plan.toPush, path)
		case rh == "" || rh != hash:
			plan.changedCount++
			plan.toPush = append(plan.toPush, path)
		default:
			plan.unchangedCount++
		}
	}
	sort.Strings(plan.toPush)
	return plan
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

// renderImportReport formats the server's import report. requestedApply is
// the --apply flag the caller passed — distinct from resp.Applied, which can
// still be false under --apply when onConflict=fail hits a conflict; the two
// need different closing messages so a real apply attempt that applied
// nothing isn't mistaken for an ordinary dry run.
func renderImportReport(resp *client.ImportResponse, requestedApply bool) string {
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

	switch {
	case resp.Applied:
		fmt.Fprintf(&b, "Imported %d entries.\n", len(resp.Report))
	case requestedApply:
		fmt.Fprintf(&b, "Nothing imported — conflicts must be resolved (see rows above). Choose --on-conflict skip|overwrite|suffix or remove the conflicting entries.\n")
	default:
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
	pushCmd.Flags().BoolVar(&pushDelta, "delta", false, "Only pack and upload entries that are new or changed since the target project's current state (mutually exclusive with --entry and --output) — implies --on-conflict overwrite unless you pass --on-conflict explicitly")
	rootCmd.AddCommand(pushCmd)
}
