package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/quillit/cli/internal/client"
)

func TestRenderImportReport_DryRun(t *testing.T) {
	resp := &client.ImportResponse{
		Applied: false,
		Report: []client.ImportReportRow{
			{Path: "characters/npcs/tom", Action: "create"},
			{Path: "locations/inn", Action: "conflict", Detail: "an entry already exists at this path"},
		},
	}
	out := renderImportReport(resp, false)
	for _, want := range []string{"characters/npcs/tom", "create", "locations/inn", "conflict", "--apply"} {
		if !strings.Contains(out, want) {
			t.Errorf("report output missing %q:\n%s", want, out)
		}
	}
}

func TestRenderImportReport_Applied(t *testing.T) {
	resp := &client.ImportResponse{
		Applied: true,
		Report:  []client.ImportReportRow{{Path: "a/b", Action: "create"}},
	}
	out := renderImportReport(resp, true)
	if strings.Contains(out, "--apply") {
		t.Errorf("applied report still suggests --apply:\n%s", out)
	}
}

// TestRenderImportReport_ApplyRequestedButNotApplied covers --apply hitting
// conflicts under onConflict=fail: the server reports Applied=false even
// though the user asked for a real apply, so the ordinary dry-run message
// ("Re-run with --apply") would be actively misleading — the user already
// did that.
func TestRenderImportReport_ApplyRequestedButNotApplied(t *testing.T) {
	resp := &client.ImportResponse{
		Applied: false,
		Report: []client.ImportReportRow{
			{Path: "locations/inn", Action: "conflict", Detail: "an entry already exists at this path"},
		},
	}
	out := renderImportReport(resp, true)
	if strings.Contains(out, "Dry run") {
		t.Errorf("apply-requested report still shows dry-run text:\n%s", out)
	}
	for _, want := range []string{"Nothing imported", "conflict", "--on-conflict"} {
		if !strings.Contains(out, want) {
			t.Errorf("report output missing %q:\n%s", want, out)
		}
	}
}

func TestClassifyDelta(t *testing.T) {
	local := map[string]string{
		"characters/npcs/tom": "hash-tom-new",
		"locations/inn":       "hash-inn-same",
		"characters/npcs/new": "hash-new-entry",
	}
	remote := []client.EntryMeta{
		{Slug: "tom", DirectoryPath: "characters/npcs", BodyHash: "hash-tom-old"},
		{Slug: "inn", DirectoryPath: "locations", BodyHash: "hash-inn-same"},
		{Slug: "unknown-hash-entry", DirectoryPath: "characters/npcs", BodyHash: ""},
	}
	// Add a local entry matching the unknown-hash remote entry, to prove
	// an empty remote hash is always treated as changed.
	local["characters/npcs/unknown-hash-entry"] = "hash-anything"

	plan := classifyDelta(local, remote)

	if plan.newCount != 1 {
		t.Errorf("newCount = %d, want 1 (characters/npcs/new)", plan.newCount)
	}
	if plan.changedCount != 2 {
		t.Errorf("changedCount = %d, want 2 (tom's hash differs, unknown-hash-entry's remote hash is empty)", plan.changedCount)
	}
	if plan.unchangedCount != 1 {
		t.Errorf("unchangedCount = %d, want 1 (locations/inn)", plan.unchangedCount)
	}
	wantPush := []string{"characters/npcs/new", "characters/npcs/tom", "characters/npcs/unknown-hash-entry"}
	if len(plan.toPush) != len(wantPush) {
		t.Fatalf("toPush = %v, want %v", plan.toPush, wantPush)
	}
	for i := range wantPush {
		if plan.toPush[i] != wantPush[i] {
			t.Errorf("toPush[%d] = %q, want %q", i, plan.toPush[i], wantPush[i])
		}
	}
}

func TestClassifyDelta_NoRemoteEntriesEverythingIsNew(t *testing.T) {
	local := map[string]string{"a": "h1", "b/c": "h2"}
	plan := classifyDelta(local, nil)
	if plan.newCount != 2 || plan.changedCount != 0 || plan.unchangedCount != 0 {
		t.Errorf("plan = %+v, want newCount=2, changedCount=0, unchangedCount=0", plan)
	}
}

func TestClassifyDelta_AllUnchangedProducesEmptyToPush(t *testing.T) {
	local := map[string]string{"a": "h1"}
	remote := []client.EntryMeta{{Slug: "a", DirectoryPath: "", BodyHash: "h1"}}
	plan := classifyDelta(local, remote)
	if len(plan.toPush) != 0 {
		t.Errorf("toPush = %v, want empty", plan.toPush)
	}
	if plan.unchangedCount != 1 {
		t.Errorf("unchangedCount = %d, want 1", plan.unchangedCount)
	}
}

func TestLocalEntryHashes(t *testing.T) {
	root := t.TempDir()
	writeFileForTest(t, filepath.Join(root, "characters/npcs/tom/tom.md"), "hello tom")
	writeFileForTest(t, filepath.Join(root, "locations/inn/inn.md"), "hello inn")

	hashes, err := localEntryHashes(root)
	if err != nil {
		t.Fatalf("localEntryHashes: %v", err)
	}
	if len(hashes) != 2 {
		t.Fatalf("got %d entries, want 2: %v", len(hashes), hashes)
	}
	sum := sha256.Sum256([]byte("hello tom"))
	want := hex.EncodeToString(sum[:])
	if hashes["characters/npcs/tom"] != want {
		t.Errorf("hash for characters/npcs/tom = %q, want %q", hashes["characters/npcs/tom"], want)
	}
}

func writeFileForTest(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMatchWebProject(t *testing.T) {
	ps := []client.Project{{ID: "p1", Name: "curse-of-strahd"}, {ID: "p2", Name: "one-shots"}}
	if p, err := matchWebProject(ps, "curse-of-strahd"); err != nil || p.ID != "p1" {
		t.Errorf("by name: %+v, %v", p, err)
	}
	if p, err := matchWebProject(ps, "p2"); err != nil || p.Name != "one-shots" {
		t.Errorf("by id: %+v, %v", p, err)
	}
	if _, err := matchWebProject(ps, "nope"); err == nil {
		t.Error("unknown project matched")
	} else if !strings.Contains(err.Error(), "curse-of-strahd") {
		t.Errorf("error should list available projects, got: %v", err)
	}
}
