// Command migrate-content is the one-time HTML-to-markdown content migration
// dry-run tool (issue #34, docs/web-refactor-spec.md §5). It reads a copy of
// quillit.db, converts every entry to CLI-aligned annotated markdown, and
// writes a full report for review. It never writes back to the database or
// to blob storage — see internal/migrate.Run's doc comment.
//
// Usage:
//
//	go run ./cmd/migrate-content -db /path/to/copy-of-quillit.db -out report.json
//
// Run it against a COPY of the production database (and, if any entry has a
// blob-backed body, a reachable MinIO snapshot) — never the live database.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/quillit/svc/internal/db"
	"github.com/quillit/svc/internal/migrate"
	"github.com/quillit/svc/internal/storage"
)

func main() {
	dbPath := flag.String("db", "", "path to a copy of quillit.db to migrate (required; never point this at the live database)")
	outPath := flag.String("out", "migration-report.json", "path to write the JSON dry-run report to")
	useMinio := flag.Bool("minio", true, "resolve body_key-backed entry bodies from MinIO (set false for a database with no blob-backed bodies)")
	flag.Parse()

	if *dbPath == "" {
		fmt.Fprintln(os.Stderr, "migrate-content: -db is required")
		flag.Usage()
		os.Exit(2)
	}

	if err := run(*dbPath, *outPath, *useMinio); err != nil {
		fmt.Fprintf(os.Stderr, "migrate-content: %v\n", err)
		os.Exit(1)
	}
}

func run(dbPath, outPath string, useMinio bool) error {
	database, err := db.OpenLegacy(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	var blobs migrate.BlobFetcher
	if useMinio {
		store, err := storage.NewMinio()
		if err != nil {
			return fmt.Errorf("configure MinIO: %w", err)
		}
		blobs = minioFetcher{store}
	}

	report, err := migrate.Run(context.Background(), database, blobs)
	if err != nil {
		return fmt.Errorf("run migration: %w", err)
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return fmt.Errorf("write report to %s: %w", outPath, err)
	}

	var clean, flagged, failed int
	for _, e := range report.Entries {
		switch e.Status {
		case "clean":
			clean++
		case "flagged":
			flagged++
		case "failed":
			failed++
		}
	}
	fmt.Printf("migrate-content: %d entries — %d clean, %d flagged, %d failed. Report written to %s.\n",
		len(report.Entries), clean, flagged, failed, outPath)
	if failed > 0 {
		fmt.Println("migrate-content: hard failures present — resolve them before any cutover sign-off.")
	}
	return nil
}

type minioFetcher struct{ store *storage.MinioStore }

func (m minioFetcher) FetchBody(ctx context.Context, key string) (string, error) {
	data, err := m.store.Get(ctx, key)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
