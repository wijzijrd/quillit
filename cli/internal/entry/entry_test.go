package entry

import (
	"os"
	"path/filepath"
	"testing"
)

func mkEntry(t *testing.T, root, name string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".md"), []byte("---\nname: "+name+"\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAssign_Success(t *testing.T) {
	root := t.TempDir()
	mkEntry(t, root, "tom")

	dest, err := Assign(root, "tom", "characters/npcs")
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}

	wantDest := filepath.Join(root, "characters", "npcs", "tom")
	if dest != wantDest {
		t.Errorf("dest = %q, want %q", dest, wantDest)
	}
	if _, err := os.Stat(filepath.Join(dest, "tom.md")); err != nil {
		t.Errorf("expected tom.md to have moved with the folder: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "tom")); err == nil {
		t.Error("entry still present at project root after assign")
	}
}

func TestAssign_MissingSource(t *testing.T) {
	root := t.TempDir()

	if _, err := Assign(root, "ghost", "characters/npcs"); err == nil {
		t.Fatal("expected error for missing entry")
	}
}

func TestAssign_DestinationCollision(t *testing.T) {
	root := t.TempDir()
	mkEntry(t, root, "tom")
	mkEntry(t, filepath.Join(root, "characters", "npcs"), "tom") // already occupies the destination

	_, err := Assign(root, "tom", "characters/npcs")
	if err == nil {
		t.Fatal("expected error for destination collision")
	}

	// No partial move: source must still be intact.
	if _, statErr := os.Stat(filepath.Join(root, "tom", "tom.md")); statErr != nil {
		t.Errorf("source entry should be untouched after a failed assign: %v", statErr)
	}
}

func TestAssign_RunningTwiceFailsSecondTime(t *testing.T) {
	root := t.TempDir()
	mkEntry(t, root, "tom")

	if _, err := Assign(root, "tom", "characters/npcs"); err != nil {
		t.Fatalf("first Assign: %v", err)
	}
	if _, err := Assign(root, "tom", "characters/npcs"); err == nil {
		t.Fatal("expected second Assign (entry no longer at root) to fail")
	}
}
