// Command migrate-cutover performs the write phase of the content migration
// (issue #35, docs/web-refactor-spec.md §5 "cutover"). It reruns the exact
// same conversion pipeline as migrate-content (#34, cmd/migrate-content) —
// nothing about source data or conversion rules differs here — and this
// time actually creates each successfully-converted entry in the
// already-deployed quillit/content service (POST
// /content/projects/{id}/entries), which owns writing the markdown body to
// MinIO and validating/compiling it. See internal/migrate/cutover.go's doc
// comment for why this calls content's HTTP API instead of writing to its
// database directly.
//
// It never modifies entry or content-migration data in svc's own database, and
// never deletes anything from MinIO (except via explicit -cleanup, which is
// scoped to confirmed-imported entries only). It opens svc's database via
// db.OpenLegacy(), which caps migration at schema v7 — the last version with
// the legacy entries/annotations/etc. tables intact — instead of db.Open(),
// which would migrate on to v8 and drop those tables before this tool ever
// reads a row from them. Against an already-v7-or-later production database,
// OpenLegacy() runs no migration at all: every migration step is a no-op once
// the schema is already at or past the version it would apply. svc's legacy
// entries table (and the handlers/tables issue #35 lists for removal) are
// dropped separately, by schema migration v8 (internal/db/db.go toV8) and the
// handler-removal changes in this same plan's Part 2/3 — deploy those only
// after this tool has been run against production and the result has been
// spot-checked against content's API (e.g. GET /content/projects/{id}/entries
// for a few known projects).
//
// Usage:
//
//	go run ./cmd/migrate-cutover -svc-db /path/to/quillit.db -content-url http://localhost:3004 -apply
//
// Run it against the LIVE svc database with a reachable MinIO and a running
// content service. The tool does not write to entry/migration data — it opens
// the database with db.OpenLegacy(), which performs no migration at all
// against an already-current-version database. Omit -apply to print the plan
// without creating anything.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/quillit/svc/internal/db"
	"github.com/quillit/svc/internal/migrate"
	"github.com/quillit/svc/internal/storage"
)

func main() {
	dbPath := flag.String("svc-db", "", "path to svc's LIVE quillit.db (required; this tool never modifies entry/migration data — it opens the database with db.OpenLegacy(), which performs no migration at all against an already-current-version database)")
	contentURL := flag.String("content-url", "http://localhost:3004", "base URL of the running quillit/content service")
	useMinio := flag.Bool("minio", true, "resolve body_key-backed entry bodies from MinIO (set false only if no entry has a body_key)")
	apply := flag.Bool("apply", false, "actually create entries in content (default: dry-run, prints the plan only)")
	force := flag.Bool("force", false, "proceed even if the conversion pass reports hard failures (default: refuse)")
	cleanup := flag.Bool("cleanup", false, "issue #35 step 7: after cutover is verified, delete legacy entries/{id}/body.html MinIO objects for every entry confirmed imported (requires -apply; a no-op dry-run without it)")
	flag.Parse()

	if *dbPath == "" {
		fmt.Fprintln(os.Stderr, "migrate-cutover: -svc-db is required")
		flag.Usage()
		os.Exit(2)
	}

	if err := run(*dbPath, *contentURL, *useMinio, *apply, *force, *cleanup); err != nil {
		fmt.Fprintf(os.Stderr, "migrate-cutover: %v\n", err)
		os.Exit(1)
	}
}

func run(dbPath, contentURL string, useMinio, apply, force, cleanup bool) error {
	database, err := db.OpenLegacy(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	var minioStore *storage.MinioStore
	var blobs migrate.BlobFetcher
	if useMinio {
		store, err := storage.NewMinio()
		if err != nil {
			return fmt.Errorf("configure MinIO: %w", err)
		}
		minioStore = store
		blobs = minioFetcher{store}
	}

	report, err := migrate.Run(context.Background(), database, blobs)
	if err != nil {
		return fmt.Errorf("conversion pass: %w", err)
	}
	var failed int
	for _, e := range report.Entries {
		if e.Status == "failed" {
			failed++
		}
	}
	if failed > 0 && !force {
		return fmt.Errorf("%d entries failed conversion — resolve them (rerun cmd/migrate-content for the full report) or pass -force to skip them", failed)
	}

	if !apply {
		fmt.Printf("migrate-cutover: dry run — %d entries would be created in content at %s (%d would be skipped as failed). Pass -apply to actually run.\n",
			len(report.Entries)-failed, contentURL, failed)
		return nil
	}

	importer := &migrate.ContentClient{BaseURL: contentURL, HTTP: &http.Client{Timeout: 30 * time.Second}}
	results, err := migrate.Cutover(context.Background(), database, blobs, importer)
	if err != nil {
		return fmt.Errorf("cutover: %w", err)
	}

	var imported, skipped, errored int
	for _, r := range results {
		switch r.Status {
		case "imported":
			imported++
		case "skipped-failed":
			skipped++
		case "error":
			errored++
			fmt.Fprintf(os.Stderr, "migrate-cutover: entry %s (%s): %s\n", r.EntryID, r.Path, r.Err)
		}
	}
	fmt.Printf("migrate-cutover: %d imported, %d skipped (failed conversion), %d errored.\n", imported, skipped, errored)
	if errored > 0 {
		return fmt.Errorf("%d entries failed to import — fix and rerun with -apply (safe: already-imported entries return 409 and are counted as imported, not retried destructively)", errored)
	}

	if cleanup {
		if minioStore == nil {
			return fmt.Errorf("-cleanup requires -minio (need a MinIO client to delete legacy body.html objects)")
		}
		targets := cleanupTargets(results)
		var deleted, deleteErrs int
		for _, id := range targets {
			key := fmt.Sprintf("entries/%s/body.html", id)
			if err := minioStore.Delete(context.Background(), key); err != nil {
				deleteErrs++
				fmt.Fprintf(os.Stderr, "migrate-cutover: delete %s: %v\n", key, err)
				continue
			}
			deleted++
		}
		fmt.Printf("migrate-cutover: cleanup — deleted %d legacy body.html objects, %d errors.\n", deleted, deleteErrs)
		if deleteErrs > 0 {
			return fmt.Errorf("%d legacy body.html deletions failed", deleteErrs)
		}
	}
	return nil
}

// cleanupTargets returns the entry ids whose legacy body.html is safe to
// delete: only entries this exact run's Cutover confirmed as "imported" —
// never derived from the earlier dry-run report alone, since that doesn't
// confirm content actually has the entry.
func cleanupTargets(results []migrate.CutoverResult) []string {
	var ids []string
	for _, r := range results {
		if r.Status == "imported" {
			ids = append(ids, r.EntryID)
		}
	}
	return ids
}

type minioFetcher struct{ store *storage.MinioStore }

func (m minioFetcher) FetchBody(ctx context.Context, key string) (string, error) {
	data, err := m.store.Get(ctx, key)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
