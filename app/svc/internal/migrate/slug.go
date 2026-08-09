// Package migrate implements the one-time HTML-to-markdown content migration
// tool (issue #34): converts legacy entries into the CLI-aligned annotated
// markdown format and produces a dry-run report. It writes nothing permanent.
package migrate

import (
	"fmt"

	"github.com/quillit/svc/internal/db"
)

// Slugify derives a kebab-case slug from an entry title. Falls back to
// "untitled" for a title that kebab-cases to the empty string.
func Slugify(title string) string {
	s := db.KebabCase(title)
	if s == "" {
		return "untitled"
	}
	return s
}

// DirectoryPath derives a project-relative directory path from a legacy
// category name (e.g. "Characters" -> "characters"). Empty category maps to
// the project root ("").
func DirectoryPath(category string) string {
	return db.KebabCase(category)
}

// UniqueSlug returns base, or base suffixed with "-2", "-3", ... until it no
// longer collides with an entry already marked taken. Does not mutate taken.
func UniqueSlug(base string, taken map[string]bool) string {
	if !taken[base] {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !taken[candidate] {
			return candidate
		}
	}
}
