package cmd

import (
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
