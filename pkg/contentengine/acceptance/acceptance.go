// Package acceptance is two things sharing one fixture corpus:
//
//  1. A golden-file regression suite (golden_test.go, testdata/) proving
//     parse, filter, render, and linkindex behave correctly on a fixed
//     set of entries covering every scenario docs/cli-spec.md §4
//     describes — run via a normal `go test ./...`.
//
//  2. Validate, below: a reusable, corpus-agnostic check with no golden
//     files involved at all, for validating *arbitrary* entries that
//     have no recorded expected output. docs/web-refactor-spec.md §5
//     designates this as the Phase 2 HTML->markdown migration's
//     acceptance test — a migrated entry is only "done" once Validate
//     reports no error for it.
//
// To point the golden suite itself at fixtures beyond this package's
// own testdata/, copy golden_test.go's pattern (loadFixture + a
// checkGolden call per view/output you care about) against a different
// directory — the harness makes no assumption about which corpus it's
// pointed at beyond "a directory of .md files organized however the
// caller likes." To validate an arbitrary corpus without any golden
// files at all, use Validate directly:
//
//	entries := map[string][]byte{"characters/npcs/tom": data, ...}
//	for _, r := range acceptance.Validate(entries, vocabulary) {
//		if r.Err != nil {
//			// report r.Name, r.Err
//		}
//	}
package acceptance

import (
	"sort"

	"github.com/quillit/contentengine/filter"
	"github.com/quillit/contentengine/parse"
)

// EntryResult is the outcome of validating one entry.
type EntryResult struct {
	// Name identifies the entry for reporting — whatever the caller used
	// as the key in the entries map passed to Validate (a file path, a
	// project-relative entry path, ...).
	Name string
	Err  error
}

// Validate runs each entry (name -> raw .md bytes) through the same
// parse + facet-validation pipeline compile/render/export use, checked
// against vocabulary, and reports the result per entry, sorted by name
// for deterministic output. There's no golden-file comparison here —
// arbitrary content has no pre-recorded expected output, only a
// pass/fail: does the content engine accept it.
func Validate(entries map[string][]byte, vocabulary []string) []EntryResult {
	results := make([]EntryResult, 0, len(entries))
	for name, data := range entries {
		results = append(results, EntryResult{Name: name, Err: validateOne(data, vocabulary)})
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Name < results[j].Name })
	return results
}

// validateOne parses data and validates every facet it references
// (anywhere in the tree, per filter.Filter's whole-entry check) against
// vocabulary. A plain DM view is the cheapest way to trigger that
// check — it doesn't narrow to any one facet, so every :::card block's
// facet gets validated regardless of what a caller might eventually
// render.
func validateOne(data []byte, vocabulary []string) error {
	entry, err := parse.Parse(data)
	if err != nil {
		return err
	}
	_, err = filter.Filter(entry, filter.View{Kind: filter.ViewDM}, vocabulary)
	return err
}
