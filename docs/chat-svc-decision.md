# Decision: chat stays in svc (no chat-svc extraction)

**Date:** 2026-07-22
**Status:** decided — revisit only on the triggers below

## Decision

Game Mode chat (the WebSocket hub in `svc/internal/ws/` and the handlers in
`svc/internal/handler/chat_ws.go` / `game_sessions.go`) remains part of `svc`
rather than being split into a separate `chat-svc` service.

## Why

1. **Single SQLite writer.** `svc` opens `quillit.db` with `SetMaxOpenConns(1)`
   and WAL mode; chat persists every message to `chat_messages` in that same
   file. A second process writing the same SQLite file breaks the
   one-writer model that the rest of the service is built on.
2. **In-process entry access.** `share_card` resolves entries through
   `EntriesHandler.fetchResolved` and scopes them to the project via
   `campaign_ids` — a direct in-process call, not an API. Extraction would
   require a new cross-service entries/membership API.
3. **Shared auth.** Chat uses the same session cookie → JWT middleware
   (`middleware.RequireSession`) and `JWT_SECRET` as every other route.
4. **Single-instance hub by design.** `ws.Hub` is an in-process room registry
   (`hub.go`); multi-instance fan-out is explicitly out of scope. A separate
   service would gain nothing until an external pub/sub replaces it.

The chat code is already well-bounded inside `svc` (`internal/ws` is
transport-agnostic and handler-free), so the modularity benefit of extraction
is realized without the operational cost.

## Revisit if

- Horizontal scaling of chat is needed (requires external pub/sub anyway).
- The database moves from SQLite to Postgres (removes the single-writer
  constraint).
- Chat needs an independent deploy/release cadence from the rest of svc.
