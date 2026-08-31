-- +goose Up
-- Baseline schema for content, replacing the hand-rolled PRAGMA user_version
-- migration chain (toV1..toV5) that this service ran until now.
--
-- This reproduces the *end state* of toV1..toV5 — derived by reading
-- internal/db/db.go's toV1 through toV5 directly (see the deleted functions
-- in git history for the full trail), not a step-by-step replay. Unlike
-- svc's toV8, none of content's later migrations drop or reshape anything
-- an earlier migration created: toV2 creates entries/entry_links/facets/
-- project_facets and seeds facets; toV3 adds the entries_fts FTS5 virtual
-- table; toV4 and toV5 each ALTER TABLE entries to add one nullable column
-- (orphaned_at, body_hash). Every table and column created along the way
-- survives to the final schema, so this baseline is simply their union,
-- with entries already carrying its final orphaned_at/body_hash columns.
--
-- entries_fts is a virtual table (FTS5). CREATE VIRTUAL TABLE IF NOT EXISTS
-- is standard SQLite syntax (not goose-specific) and was verified directly
-- against modernc.org/sqlite in this service's test suite (see db_test.go)
-- to be idempotent — running it twice does not error and does not reset
-- the index's contents.
--
-- seedFacets' three static rows (motivation, description, history) are
-- reproduced below as INSERT OR IGNORE. Unlike svc's seedFacets, content's
-- version (internal/db/db.go, now-deleted) has no dynamic backfill step —
-- its own doc comment says so explicitly ("matching svc's pre-migration
-- seed... minus the quick_view_templates backfill — content starts fresh,
-- with no legacy quick-view data to carry forward"), so there is nothing
-- dynamic to trace or omit here.
--
-- Every statement is IF NOT EXISTS / OR IGNORE so this migration has zero
-- effect against a database that already reached toV5 under the old system —
-- goose will still record it as applied, but it changes nothing there.
--
-- PRECONDITION: because every statement below is IF NOT EXISTS / OR IGNORE,
-- this migration only produces a correct schema against (a) an empty
-- database, or (b) one already at the old hand-rolled system's v5 end
-- state. It must never run against a database still at that old system's
-- user_version 1-4 (never cut over to v5) — at those versions `entries`
-- already exists without orphaned_at/body_hash (added by toV4/toV5 via
-- ALTER TABLE, not CREATE), and CREATE TABLE IF NOT EXISTS would silently
-- no-op rather than adding those columns, leaving a wrong-but-internally-
-- consistent schema. db.go's migrate() enforces this precondition at
-- runtime by checking PRAGMA user_version before goose.Up ever runs.

CREATE TABLE IF NOT EXISTS schema_meta (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    created_at INTEGER NOT NULL
);

INSERT OR IGNORE INTO schema_meta (id, created_at) VALUES (1, strftime('%s','now'));

CREATE TABLE IF NOT EXISTS entries (
    id             TEXT PRIMARY KEY,
    project_id     TEXT NOT NULL,
    slug           TEXT NOT NULL
        CHECK (slug <> '' AND slug NOT GLOB '*[^a-z0-9-]*'),
    directory_path TEXT NOT NULL DEFAULT '',
    title          TEXT NOT NULL DEFAULT '',
    tags           TEXT NOT NULL DEFAULT '[]',
    owner_user_id  TEXT,
    created_at     INTEGER NOT NULL,
    updated_at     INTEGER NOT NULL,
    orphaned_at    INTEGER,
    body_hash      TEXT,
    UNIQUE(project_id, directory_path, slug)
);

CREATE INDEX IF NOT EXISTS idx_entries_project ON entries(project_id);

CREATE TABLE IF NOT EXISTS entry_links (
    entry_id        TEXT    NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
    target_path     TEXT    NOT NULL,
    target_entry_id TEXT,
    label           TEXT    NOT NULL DEFAULT '',
    card_facet      TEXT,
    resolved        INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_entry_links_entry ON entry_links(entry_id);

CREATE TABLE IF NOT EXISTS facets (
    name TEXT PRIMARY KEY
        CHECK (name <> '' AND name NOT GLOB '*[^a-z0-9-]*')
);

CREATE TABLE IF NOT EXISTS project_facets (
    project_id TEXT NOT NULL,
    name       TEXT NOT NULL
        CHECK (name <> '' AND name NOT GLOB '*[^a-z0-9-]*'),
    UNIQUE(project_id, name)
);

INSERT OR IGNORE INTO facets (name) VALUES ('motivation');
INSERT OR IGNORE INTO facets (name) VALUES ('description');
INSERT OR IGNORE INTO facets (name) VALUES ('history');

-- entries_fts (toV3): FTS5 search index over title/tags/body. entry_id is
-- UNINDEXED (a lookup key, not searchable text); project_id/slug/
-- directory_path live only on entries and are joined in at query time. No
-- triggers keep this in sync — writers refresh it explicitly alongside
-- entry_links recompilation (see internal/handler/search.go,
-- internal/handler/links.go), matching this codebase's existing convention.
CREATE VIRTUAL TABLE IF NOT EXISTS entries_fts USING fts5(
    entry_id UNINDEXED,
    title,
    tags,
    body,
    tokenize = 'unicode61'
);

-- +goose Down
DROP TABLE IF EXISTS entries_fts;
DROP TABLE IF EXISTS project_facets;
DROP TABLE IF EXISTS facets;
DROP TABLE IF EXISTS entry_links;
DROP TABLE IF EXISTS entries;
DROP TABLE IF EXISTS schema_meta;
