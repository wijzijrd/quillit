-- +goose Up
-- Baseline schema for auth, replacing the hand-rolled PRAGMA user_version
-- migration chain (toV1, toV2) that this project ran until now.
--
-- This reproduces the *end state* of toV1 + toV2 — the schema Open()
-- produces today after running both steps in order — derived by reading
-- internal/db/db.go's toV1/toV2 directly (see the deleted functions in git
-- history for the full trail).
--
-- Unlike svc's chain, no later step here drops or reshapes anything an
-- earlier step created: toV1 creates `users` in its final shape outright,
-- and toV2 only adds a new table (password_reset_tokens) plus its index —
-- it never ALTERs `users`. So both tables below simply survive to today's
-- schema unmodified.
--
-- toV1 also contains a second branch (recreating an even-older,
-- pre-role/pre-active `users` table via users_new/DROP/RENAME) for installs
-- that predated the user_version-tracked migration chain itself — i.e. a
-- `users` table that exists with zero PRAGMA user_version ever having been
-- set. That branch and the fresh-install branch converge on the identical
-- final `users` shape reproduced below, so only that end state is
-- reproduced here, not the recreation dance itself.
--
-- Every statement is IF NOT EXISTS so this migration has zero effect
-- against a database that already reached toV2 under the old system —
-- goose will still record it as applied, but it changes nothing there.
--
-- PRECONDITION: because every statement below is IF NOT EXISTS, this
-- migration only produces a correct schema against (a) an empty database,
-- or (b) one already at the old hand-rolled system's v2 end state. It must
-- never run against a database still at that old system's user_version 1
-- (i.e. one that ran toV1 but never toV2). For auth's specific two-step
-- chain, applying this baseline against a v1 database would happen to
-- produce a correct v2 schema anyway (toV2 only adds password_reset_tokens;
-- it never reshapes anything toV1 created) — but that's an accident of
-- today's two-step chain, not something this file can guarantee for
-- whatever gets added after it. db.go's migrate() enforces the v1
-- precondition at runtime by checking PRAGMA user_version before goose.Up
-- ever runs, exactly as svc does, so the invariant holds regardless of how
-- the chain grows later.

CREATE TABLE IF NOT EXISTS users (
    id            TEXT    PRIMARY KEY,
    email         TEXT    NOT NULL UNIQUE,
    username      TEXT    NOT NULL UNIQUE,
    password_hash TEXT    NOT NULL,
    role          TEXT    NOT NULL DEFAULT 'user'
                  CHECK (role IN ('user', 'admin')),
    active        INTEGER NOT NULL DEFAULT 1,
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL
);

-- Single-use, short-lived password reset tokens, keyed to a user.
CREATE TABLE IF NOT EXISTS password_reset_tokens (
    id         TEXT    PRIMARY KEY,
    user_id    TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT    NOT NULL UNIQUE,
    expires_at INTEGER NOT NULL,
    used       INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_user ON password_reset_tokens(user_id);

-- +goose Down
DROP TABLE IF EXISTS password_reset_tokens;
DROP TABLE IF EXISTS users;
