// Package linkindex extracts and (de)serializes an entry's outgoing
// wikilinks — the content of a project's per-entry links.conf file
// (docs/cli-spec.md §3 "Link index", §5). It has no filesystem coupling:
// Extract takes a *parse.Entry and returns Records; Encode/Decode work on
// plain bytes. Something with filesystem access (cli/internal/resolver)
// is responsible for reading/writing the actual links.conf file and for
// filling in each Record's Resolved field — this package doesn't know
// what a project tree looks like, let alone check one.
package linkindex

import (
	"encoding/json"
	"fmt"

	"github.com/quillit/contentengine/parse"
)

// Record is one outgoing wikilink, ready to serialize into links.conf.
type Record struct {
	TargetPath string `json:"target_path"`
	Label      string `json:"label"`

	// CardFacet is set when the link occurs inside a :::card block (at
	// any nesting depth, including inside a :::secret): the facet it
	// belongs to. Empty for links elsewhere — plain prose, or directly
	// inside a :::secret with no card wrapper.
	CardFacet string `json:"card_facet,omitempty"`

	// Resolved is always false on Records returned by Extract — this
	// package never checks the filesystem. The caller (typically the
	// CLI's `compile` command) fills it in after checking TargetPath
	// against the actual project. CLI spec §7 "compile": links to
	// non-existent entries are recorded, not dropped, and reported as
	// warnings — Resolved is what lets a later reader tell the
	// difference without re-checking the filesystem itself.
	Resolved bool `json:"resolved"`
}

// Extract walks entry's entire block tree — including blocks nested
// inside a :::secret — collecting every outgoing wikilink in document
// order.
func Extract(entry *parse.Entry) []Record {
	return extractBlocks(entry.Body, "")
}

// extractBlocks recurses through blocks, carrying facet — the enclosing
// card's facet, if any block on the path from the root down to here was
// a :::card — down into nested blocks so a link inside a card nested
// inside a secret still gets attributed to that card's facet.
func extractBlocks(blocks []parse.Block, facet string) []Record {
	var out []Record
	for _, b := range blocks {
		blockFacet := facet
		if b.Kind == parse.BlockCard {
			blockFacet = b.Facet
		}
		for _, link := range b.Links {
			out = append(out, Record{TargetPath: link.Path, Label: link.Label, CardFacet: blockFacet})
		}
		out = append(out, extractBlocks(b.Blocks, blockFacet)...)
	}
	return out
}

// Encode serializes records as the links.conf file format: an indented
// JSON array. links.conf is generated and never hand-edited (CLI spec
// §5), so the format's only real requirement is parsing reliably — JSON
// is the simplest choice that satisfies that; the indentation is purely
// for a curious DM who opens the file, not a parsing requirement.
func Encode(records []Record) ([]byte, error) {
	if records == nil {
		records = []Record{}
	}
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encoding links.conf: %w", err)
	}
	return data, nil
}

// Decode parses links.conf content previously written by Encode.
func Decode(data []byte) ([]Record, error) {
	var records []Record
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("decoding links.conf: %w", err)
	}
	return records, nil
}
