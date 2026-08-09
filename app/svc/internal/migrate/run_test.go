package migrate

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/quillit/svc/internal/db"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func insertEntry(t *testing.T, database *sql.DB, id, title, category, body, bodyKey, campaignIDs, tags, quickView string) {
	t.Helper()
	now := time.Now().Unix()
	var bodyKeyVal interface{}
	if bodyKey != "" {
		bodyKeyVal = bodyKey
	}
	if _, err := database.Exec(
		`INSERT INTO entries (id, title, category, body, body_key, visibility, campaign_ids, linked_entries, tags, quick_view_data, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 'private', ?, '[]', ?, ?, ?, ?)`,
		id, title, category, body, bodyKeyVal, campaignIDs, tags, quickView, now, now,
	); err != nil {
		t.Fatalf("insert entry %s: %v", id, err)
	}
}

func insertGMAnnotation(t *testing.T, database *sql.DB, id, entryID, text string) {
	t.Helper()
	now := time.Now().Unix()
	if _, err := database.Exec(
		`INSERT INTO annotations (id, entry_id, text, visibility, shared_with, created_at, updated_at)
		 VALUES (?, ?, ?, 'gm', '[]', ?, ?)`,
		id, entryID, text, now, now,
	); err != nil {
		t.Fatalf("insert annotation %s: %v", id, err)
	}
}

type stubBlobs map[string]string

func (s stubBlobs) FetchBody(ctx context.Context, key string) (string, error) {
	body, ok := s[key]
	if !ok {
		return "", sql.ErrNoRows
	}
	return body, nil
}

func TestRun_ConvertsInlineBodyEntry(t *testing.T) {
	database := openTestDB(t)
	insertEntry(t, database, "e1", "Mary the Innkeeper", "Characters", "<p>Runs the tavern.</p>", "", "[\"proj1\"]", "[\"npc\"]", "{}")

	report, err := Run(context.Background(), database, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.Entries) != 1 {
		t.Fatalf("expected 1 entry in report, got %d", len(report.Entries))
	}
	e := report.Entries[0]
	if e.Status != "clean" {
		t.Errorf("Status = %q, want %q (flags=%v, parseError=%q)", e.Status, "clean", e.Flags, e.ParseError)
	}
	if e.Slug != "mary-the-innkeeper" {
		t.Errorf("Slug = %q, want %q", e.Slug, "mary-the-innkeeper")
	}
	if e.DirectoryPath != "characters" {
		t.Errorf("DirectoryPath = %q, want %q", e.DirectoryPath, "characters")
	}
	if e.ProjectID != "proj1" {
		t.Errorf("ProjectID = %q, want %q", e.ProjectID, "proj1")
	}
}

func TestRun_ResolvesBodyFromBlobStore(t *testing.T) {
	database := openTestDB(t)
	insertEntry(t, database, "e1", "Mary", "Characters", "", "entries/e1/body.html", "[\"proj1\"]", "[]", "{}")
	blobs := stubBlobs{"entries/e1/body.html": "<p>From blob storage.</p>"}

	report, err := Run(context.Background(), database, blobs)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(report.Entries))
	}
	if report.Entries[0].Status != "clean" {
		t.Errorf("Status = %q, want clean", report.Entries[0].Status)
	}
}

func TestRun_MissingBlobStoreFlagsEntryAsFailed(t *testing.T) {
	database := openTestDB(t)
	insertEntry(t, database, "e1", "Mary", "Characters", "", "entries/e1/body.html", "[]", "[]", "{}")

	report, err := Run(context.Background(), database, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Entries[0].Status != "failed" {
		t.Errorf("Status = %q, want failed (no blob store configured for a blob-backed body)", report.Entries[0].Status)
	}
}

func TestRun_DeduplicatesSlugsWithinSameProjectAndDirectory(t *testing.T) {
	database := openTestDB(t)
	insertEntry(t, database, "e1", "Mary", "Characters", "<p>a</p>", "", "[\"proj1\"]", "[]", "{}")
	insertEntry(t, database, "e2", "Mary", "Characters", "<p>b</p>", "", "[\"proj1\"]", "[]", "{}")

	report, err := Run(context.Background(), database, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	slugs := map[string]bool{}
	for _, e := range report.Entries {
		if slugs[e.Slug] {
			t.Errorf("duplicate slug %q assigned within the same project+directory", e.Slug)
		}
		slugs[e.Slug] = true
	}
}

func TestRun_MultiProjectEntryPicksFirstAndReportsAll(t *testing.T) {
	database := openTestDB(t)
	insertEntry(t, database, "e1", "Mary", "Characters", "<p>a</p>", "", "[\"proj1\",\"proj2\"]", "[]", "{}")

	report, err := Run(context.Background(), database, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	e := report.Entries[0]
	if e.ProjectID != "proj1" {
		t.Errorf("ProjectID = %q, want primary %q", e.ProjectID, "proj1")
	}
	if len(e.MultiProjectIDs) != 2 || e.MultiProjectIDs[0] != "proj1" || e.MultiProjectIDs[1] != "proj2" {
		t.Errorf("MultiProjectIDs = %v, want [proj1 proj2]", e.MultiProjectIDs)
	}
}

func TestRun_MentionResolvesToAssignedPath(t *testing.T) {
	database := openTestDB(t)
	insertEntry(t, database, "e1", "Mary", "Characters", "<p>Innkeeper.</p>", "", "[\"proj1\"]", "[]", "{}")
	insertEntry(t, database, "e2", "Tom", "Characters",
		`<p>Talk to <span data-type="mention" class="entry-mention" data-id="e1">[[Mary]]</span>.</p>`,
		"", "[\"proj1\"]", "[]", "{}")

	report, err := Run(context.Background(), database, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var tom EntryReport
	for _, e := range report.Entries {
		if e.EntryID == "e2" {
			tom = e
		}
	}
	if !strings.Contains(tom.Markdown, "[[characters/mary|Mary]]") {
		t.Errorf("expected Tom's body to contain a resolved wikilink to Mary, got:\n%s", tom.Markdown)
	}
}

func TestRun_UnconsumedGMAnnotationAppearsAsTrailingSecret(t *testing.T) {
	database := openTestDB(t)
	insertEntry(t, database, "e1", "Mary", "Characters", "<p>An innkeeper.</p>", "", "[]", "[]", "{}")
	insertGMAnnotation(t, database, "a1", "e1", "Secretly a dragon.")

	report, err := Run(context.Background(), database, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(report.Entries[0].Markdown, "Secretly a dragon.") {
		t.Errorf("expected the GM annotation text in the converted body, got:\n%s", report.Entries[0].Markdown)
	}
}

func TestRun_QuickViewDataBecomesCardBlock(t *testing.T) {
	database := openTestDB(t)
	insertEntry(t, database, "e1", "Mary", "Characters", "<p>An innkeeper.</p>", "", "[]", "[]", `{"Role":"innkeeper"}`)

	report, err := Run(context.Background(), database, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(report.Entries[0].Markdown, ":::card characters\n**Role:** innkeeper\n:::") {
		t.Errorf("expected a quick-view card block, got:\n%s", report.Entries[0].Markdown)
	}
}

func TestRun_IsIdempotent(t *testing.T) {
	database := openTestDB(t)
	insertEntry(t, database, "e1", "Mary", "Characters", "<p>a</p>", "", "[\"proj1\"]", "[]", "{}")
	insertEntry(t, database, "e2", "Tom", "Lore", "<p>b</p>", "", "[\"proj1\"]", "[]", "{}")

	r1, err := Run(context.Background(), database, nil)
	if err != nil {
		t.Fatalf("Run (1st): %v", err)
	}
	r2, err := Run(context.Background(), database, nil)
	if err != nil {
		t.Fatalf("Run (2nd): %v", err)
	}
	if len(r1.Entries) != len(r2.Entries) {
		t.Fatalf("entry count differs between runs: %d vs %d", len(r1.Entries), len(r2.Entries))
	}
	for i := range r1.Entries {
		if r1.Entries[i].Slug != r2.Entries[i].Slug || r1.Entries[i].Markdown != r2.Entries[i].Markdown {
			t.Errorf("entry %d differs between runs:\n1st: %+v\n2nd: %+v", i, r1.Entries[i], r2.Entries[i])
		}
	}
}

func TestRun_MalformedBodyProducesHardFailureNotSilentPass(t *testing.T) {
	database := openTestDB(t)
	// A card facet not in the seeded vocabulary must fail validation, not
	// silently pass, per the acceptance-test requirement.
	insertEntry(t, database, "e1", "Bad Entry", "Characters", `<p>hi</p>`, "", "[]", "[]", "{}")
	if _, err := database.Exec(`UPDATE entries SET body = '<p>' || ':::card nonexistent-facet' || char(10) || 'x' || char(10) || ':::' || '</p>' WHERE id = 'e1'`); err != nil {
		t.Fatalf("seed malformed body: %v", err)
	}

	report, err := Run(context.Background(), database, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Entries[0].Status != "failed" {
		t.Errorf("Status = %q, want failed for an undeclared-facet card block", report.Entries[0].Status)
	}
	if report.Entries[0].ParseError == "" {
		t.Error("expected a non-empty ParseError for the hard failure")
	}
}
