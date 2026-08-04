package home

import "testing"

func TestValidFacetName(t *testing.T) {
	valid := []string{"motivation", "stat-block", "loot", "history2", "a-b-c-1-2-3"}
	for _, name := range valid {
		if !ValidFacetName(name) {
			t.Errorf("ValidFacetName(%q) = false, want true", name)
		}
	}

	invalid := []string{"", "Motivation", "stat_block", "-leading", "trailing-", "double--hyphen", "has space", "café"}
	for _, name := range invalid {
		if ValidFacetName(name) {
			t.Errorf("ValidFacetName(%q) = true, want false", name)
		}
	}
}
