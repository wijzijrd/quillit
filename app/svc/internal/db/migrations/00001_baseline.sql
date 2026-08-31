-- +goose Up
-- Baseline schema for svc, replacing the hand-rolled PRAGMA user_version
-- migration chain (toV1..toV8) that this project ran until now.
--
-- This is NOT a step-by-step replay of toV1..toV8. It reproduces only their
-- *end state* — the schema Open() produces today after running every step in
-- order — derived by reading internal/db/db.go's toV1 through toV8
-- directly (see the deleted functions in git history for the full trail).
--
-- toV8 (issues #34/#35, the content-service cutover) drops every legacy
-- entry-domain table that earlier steps created: entries, annotations,
-- categories, category_default_tags, project_global_categories,
-- quick_view_templates, players, player_notes, campaigns, entry_shares,
-- entry_relations, member_folders, member_folder_entries, member_entry_meta,
-- and — notably — facets, project_facets, and entry_links, all three of
-- which toV7 had *just* created (and, for facets, seeded via seedFacets)
-- one migration step earlier. None of those eleven-plus tables, or
-- seedFacets' seed rows, exist in the schema Open() produces today
-- (confirmed by TestOpen_FreshDatabase and TestToV8_DropsLegacyEntryDomainTables
-- in the old db_test.go), so none of them appear below. seedFacets' dynamic
-- backfill (seeding facets from quick_view_templates' current rows) is
-- correctly omitted for the same reason: its target table doesn't survive
-- to the schema this baseline reproduces.
--
-- Every statement is IF NOT EXISTS / OR IGNORE so this migration has zero
-- effect against a database that already reached toV8 under the old system —
-- goose will still record it as applied, but it changes nothing there.
--
-- PRECONDITION: because every statement below is IF NOT EXISTS / OR IGNORE,
-- this migration only produces a correct schema against (a) an empty
-- database, or (b) one already at the old hand-rolled system's v8 end state.
-- It must never run against a database still at that old system's
-- user_version 1-7 (i.e. one that was never cut over to v8) — some of these
-- tables/columns already exist there in an older shape, and the IF NOT
-- EXISTS statements would silently no-op instead of bringing them up to
-- date, leaving a wrong-but-internally-consistent schema. db.go's migrate()
-- enforces this precondition at runtime by checking PRAGMA user_version
-- before goose.Up ever runs.

CREATE TABLE IF NOT EXISTS sessions (
    id         TEXT    PRIMARY KEY,
    jwt        TEXT    NOT NULL,
    expires_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS projects (
    id         TEXT    PRIMARY KEY,
    name       TEXT    NOT NULL,
    type       TEXT    NOT NULL DEFAULT 'campaign',
    created_by TEXT    NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS project_members (
    id         TEXT    PRIMARY KEY,
    project_id TEXT    NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id    TEXT    NOT NULL,
    role       TEXT    NOT NULL,
    joined_at  INTEGER NOT NULL,
    username   TEXT    NOT NULL DEFAULT '',
    UNIQUE(project_id, user_id)
);

CREATE TABLE IF NOT EXISTS project_invites (
    id         TEXT    PRIMARY KEY,
    token      TEXT    NOT NULL UNIQUE,
    project_id TEXT    NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    role       TEXT    NOT NULL,
    created_by TEXT    NOT NULL,
    expires_at INTEGER NOT NULL,
    used_at    INTEGER,
    used_by    TEXT
);

CREATE TABLE IF NOT EXISTS user_settings (
    user_id    TEXT    PRIMARY KEY,
    settings   TEXT    NOT NULL DEFAULT '{}',
    updated_at INTEGER NOT NULL
);

-- A live session scoped to a project. Only one may be 'running' per project at a time.
CREATE TABLE IF NOT EXISTS game_sessions (
    id          TEXT    PRIMARY KEY,
    project_id  TEXT    NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    status      TEXT    NOT NULL DEFAULT 'running', -- 'running' | 'stopped'
    started_by  TEXT    NOT NULL,
    started_at  INTEGER NOT NULL,
    stopped_by  TEXT,
    stopped_at  INTEGER
);

CREATE INDEX IF NOT EXISTS idx_game_sessions_project ON game_sessions(project_id, status);

CREATE UNIQUE INDEX IF NOT EXISTS idx_game_sessions_one_active
    ON game_sessions(project_id) WHERE status = 'running';

-- Chat history for a game_session. entry_id has no FK (post-toV8): the
-- entries table it used to reference is gone, and card_title/card_body
-- already hold the display snapshot taken at share-time.
CREATE TABLE IF NOT EXISTS chat_messages (
    id         TEXT    PRIMARY KEY,
    session_id TEXT    NOT NULL REFERENCES game_sessions(id) ON DELETE CASCADE,
    project_id TEXT    NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    sender_id  TEXT    NOT NULL,
    type       TEXT    NOT NULL DEFAULT 'text', -- 'text' | 'note_card' | 'system'
    body       TEXT    NOT NULL DEFAULT '',
    entry_id   TEXT,
    card_title TEXT    NOT NULL DEFAULT '',
    card_body  TEXT    NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_chat_messages_session ON chat_messages(session_id, created_at);

-- The system "global" project that historically owned admin-managed
-- categories (toV2). Its category tables are gone (toV8), but the project
-- row itself was never dropped, and internal/handler/projects.go still
-- filters it out of listings by type = 'global' — so it's reproduced here
-- for schema/data parity with every already-migrated install.
INSERT OR IGNORE INTO projects (id, name, type, created_by, created_at)
VALUES ('global', 'Global Categories', 'global', 'system', CAST(strftime('%s', 'now') AS INTEGER));

-- +goose Down
DROP TABLE IF EXISTS chat_messages;
DROP TABLE IF EXISTS game_sessions;
DROP TABLE IF EXISTS user_settings;
DROP TABLE IF EXISTS project_invites;
DROP TABLE IF EXISTS project_members;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS sessions;
