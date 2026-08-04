package home

import "regexp"

// facetNameRe matches CLI spec §7 "config": facet names are lowercase,
// digits, hyphens (kebab-case) — keeps directive parsing unambiguous.
var facetNameRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidFacetName reports whether name is valid kebab-case: lowercase
// letters, digits, and hyphens, with no leading/trailing/doubled hyphen.
func ValidFacetName(name string) bool {
	return facetNameRe.MatchString(name)
}
