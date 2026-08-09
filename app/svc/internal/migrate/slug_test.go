package migrate

import "testing"

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Mary the Innkeeper": "mary-the-innkeeper",
		"  Trimmed  ":        "trimmed",
		"Tom's Tavern!":      "tom-s-tavern",
		"":                   "untitled",
	}
	for input, want := range cases {
		if got := Slugify(input); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestDirectoryPath(t *testing.T) {
	cases := map[string]string{
		"Characters": "characters",
		"Lore":       "lore",
		"":           "",
	}
	for input, want := range cases {
		if got := DirectoryPath(input); got != want {
			t.Errorf("DirectoryPath(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestUniqueSlug_NoCollision(t *testing.T) {
	taken := map[string]bool{}
	if got := UniqueSlug("mary", taken); got != "mary" {
		t.Errorf("UniqueSlug with no collision = %q, want %q", got, "mary")
	}
}

func TestUniqueSlug_ResolvesCollisionsBySuffix(t *testing.T) {
	taken := map[string]bool{"mary": true, "mary-2": true}
	got := UniqueSlug("mary", taken)
	if got != "mary-3" {
		t.Errorf("UniqueSlug with 2 collisions = %q, want %q", got, "mary-3")
	}
	if taken[got] {
		t.Errorf("UniqueSlug returned a slug that was already marked taken: %q", got)
	}
}
