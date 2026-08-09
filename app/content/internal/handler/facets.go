package handler

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/quillit/contentengine/parse"
)

// UnknownFacetError is returned when an entry body's :::card block names a
// facet outside the effective vocabulary (global facets ∪ this project's
// facets). Fail loud (docs/web-refactor-spec.md §4.3 golden rule 6): never
// guess or silently accept — name the bad facet and the full vocabulary so
// the caller can fix it.
type UnknownFacetError struct {
	Facet      string
	Vocabulary []string
}

func (e UnknownFacetError) Error() string {
	return fmt.Sprintf("unknown facet %q (effective vocabulary: %v)", e.Facet, e.Vocabulary)
}

// effectiveFacetVocabulary returns the union of the global facet vocabulary
// and projectID's own facets (docs/web-refactor-spec.md §4.3: "Effective
// vocabulary = global ∪ project"), as both a lookup set and a sorted slice
// for error messages.
func effectiveFacetVocabulary(ctx context.Context, db *sql.DB, projectID string) (map[string]bool, []string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT name FROM facets
		UNION
		SELECT name FROM project_facets WHERE project_id = ?
		ORDER BY name
	`, projectID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	set := map[string]bool{}
	var list []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, nil, err
		}
		set[name] = true
		list = append(list, name)
	}
	return set, list, rows.Err()
}

// validateFacets walks entry's entire block tree (including blocks nested
// inside a :::secret, matching linkindex.Extract's traversal) and returns
// UnknownFacetError for the first :::card block whose facet isn't in
// vocabulary.
func validateFacets(entry *parse.Entry, vocabulary map[string]bool, sortedVocabulary []string) error {
	return validateFacetBlocks(entry.Body, vocabulary, sortedVocabulary)
}

func validateFacetBlocks(blocks []parse.Block, vocabulary map[string]bool, sortedVocabulary []string) error {
	for _, b := range blocks {
		if b.Kind == parse.BlockCard && !vocabulary[b.Facet] {
			return UnknownFacetError{Facet: b.Facet, Vocabulary: sortedVocabulary}
		}
		if err := validateFacetBlocks(b.Blocks, vocabulary, sortedVocabulary); err != nil {
			return err
		}
	}
	return nil
}
