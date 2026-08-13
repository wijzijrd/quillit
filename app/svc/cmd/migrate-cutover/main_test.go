package main

import (
	"slices"
	"testing"

	"github.com/quillit/svc/internal/migrate"
)

func TestCleanupTargets(t *testing.T) {
	tests := []struct {
		name     string
		input    []migrate.CutoverResult
		expected []string
	}{
		{
			name:     "empty input",
			input:    []migrate.CutoverResult{},
			expected: nil,
		},
		{
			name: "all non-imported",
			input: []migrate.CutoverResult{
				{EntryID: "e1", Status: "skipped-failed"},
				{EntryID: "e2", Status: "error"},
				{EntryID: "e3", Status: "skipped-failed"},
			},
			expected: nil,
		},
		{
			name: "mix of statuses",
			input: []migrate.CutoverResult{
				{EntryID: "e1", Status: "imported"},
				{EntryID: "e2", Status: "skipped-failed"},
				{EntryID: "e3", Status: "imported"},
				{EntryID: "e4", Status: "error"},
				{EntryID: "e5", Status: "imported"},
			},
			expected: []string{"e1", "e3", "e5"},
		},
		{
			name: "all imported",
			input: []migrate.CutoverResult{
				{EntryID: "e1", Status: "imported"},
				{EntryID: "e2", Status: "imported"},
				{EntryID: "e3", Status: "imported"},
			},
			expected: []string{"e1", "e2", "e3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanupTargets(tt.input)
			if !slices.Equal(result, tt.expected) {
				t.Errorf("cleanupTargets(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}
