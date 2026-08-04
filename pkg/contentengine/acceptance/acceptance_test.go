package acceptance

import "testing"

func TestValidate_MixedCorpus(t *testing.T) {
	vocabulary := []string{"motivation", "description"}
	entries := map[string][]byte{
		"good":      []byte("---\nname: Fine\n---\n:::card motivation\nOK.\n:::\n"),
		"bad-facet": []byte(":::card not-declared\ncontent\n:::\n"),
		"unclosed":  []byte(":::secret\nnever closed\n"),
		"plain":     []byte("Just prose, no directives at all.\n"),
	}

	results := Validate(entries, vocabulary)
	if len(results) != len(entries) {
		t.Fatalf("got %d results, want %d", len(results), len(entries))
	}

	byName := map[string]EntryResult{}
	for _, r := range results {
		byName[r.Name] = r
	}

	if err := byName["good"].Err; err != nil {
		t.Errorf("\"good\" should validate clean, got: %v", err)
	}
	if err := byName["plain"].Err; err != nil {
		t.Errorf("\"plain\" should validate clean, got: %v", err)
	}
	if byName["bad-facet"].Err == nil {
		t.Error("\"bad-facet\" should fail validation (undeclared facet)")
	}
	if byName["unclosed"].Err == nil {
		t.Error("\"unclosed\" should fail validation (malformed directive)")
	}
}

func TestValidate_DeterministicOrder(t *testing.T) {
	entries := map[string][]byte{
		"c": []byte("prose\n"),
		"a": []byte("prose\n"),
		"b": []byte("prose\n"),
	}
	results := Validate(entries, nil)
	want := []string{"a", "b", "c"}
	for i, name := range want {
		if results[i].Name != name {
			t.Errorf("results[%d].Name = %q, want %q (results should be sorted by name)", i, results[i].Name, name)
		}
	}
}

func TestValidate_EmptyEntries(t *testing.T) {
	results := Validate(map[string][]byte{}, nil)
	if len(results) != 0 {
		t.Errorf("got %d results, want 0", len(results))
	}
}
