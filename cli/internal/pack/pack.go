// Package pack builds the tar.gz import wire format (spec §2 of
// docs/superpowers/specs/2026-08-16-cli-project-import-design.md) from a
// local project directory.
package pack

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// scaffolding files the server would ignore anyway — skipping them here
// keeps tarballs small (same list as content-svc's ignoredImportFile).
func skipFile(base string) bool {
	if strings.HasPrefix(base, ".") {
		return true
	}
	if base == "links.conf" || base == "quillit.yaml" {
		return true
	}
	switch filepath.Ext(base) {
	case ".html", ".css", ".js":
		return true
	}
	return false
}

// Project writes a gzipped tar of the project directory at root to w.
// only, when non-empty, restricts the archive to that single entry folder
// (path relative to root, e.g. "characters/npcs/tom") — archive member
// paths stay relative to the project root either way, so the server sees
// the same shape. Scaffolding files (.html/.css/.js, links.conf,
// quillit.yaml, dotfiles) are skipped.
func Project(w io.Writer, root string, only string) error {
	start := root
	if only != "" {
		start = filepath.Join(root, filepath.FromSlash(only))
		info, err := os.Stat(start)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("entry folder not found: %s", only)
		}
	}

	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	if err := walkOne(tw, root, start); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

// Selected writes a gzipped tar containing only the given entry folders
// (each relative to root, e.g. "characters/npcs/tom") — used by `push
// --delta` (#126), where the caller has already computed which entries
// are new or changed. Archive member paths stay relative to root, the
// same shape Project always produces, so the server's import pipeline
// sees no difference between a --delta push and an ordinary one.
func Selected(w io.Writer, root string, paths []string) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	for _, p := range paths {
		start := filepath.Join(root, filepath.FromSlash(p))
		info, err := os.Stat(start)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("entry folder not found: %s", p)
		}
		if err := walkOne(tw, root, start); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

// walkOne tars every non-scaffolding file under start into tw, with
// member paths relative to root (not start) — the shared body of both
// Project (a single start, possibly root itself) and Selected (one call
// per given path).
func walkOne(tw *tar.Writer, root, start string) error {
	return filepath.WalkDir(start, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && p != start {
				return filepath.SkipDir
			}
			return nil
		}
		if skipFile(d.Name()) {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if err := tw.WriteHeader(&tar.Header{
			Name:     filepath.ToSlash(rel),
			Mode:     0o644,
			Size:     int64(len(data)),
			Typeflag: tar.TypeReg,
		}); err != nil {
			return err
		}
		_, err = tw.Write(data)
		return err
	})
}
