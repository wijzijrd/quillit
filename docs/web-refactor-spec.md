# Quillit Web App — Refactor & Alignment Specification

Companion to `quillit-cli-spec.md`. That spec is **authoritative**: wherever this document and the CLI spec conflict, the CLI spec wins. All file references point into the web app repo at `~/repositories/quillit/` unless prefixed otherwise.

## 1. Purpose & scope

The CLI spec resets Quillit around one model: **one annotated `.md` file per entry** is the single source of truth, and every audience-facing artifact (DM view, player view, flash card) is derived from it by a shared render/filter pipeline. The existing web app predates this model. This document captures (a) how the web app implements notes and projects today, (b) the target model, and (c) everything required to align the two — so that the future web app becomes a UI over the same pipeline the CLI uses, not a parallel implementation.

**This is a planning document only.** The CLI is built first (Phase 0, §11). No web app changes happen until the CLI's render/filter package exists. Sequenced work on the web app follows in Phases 1–5.

**Scope: DM experience only.** Non-goals:

- Player-facing features (share links, player accounts, player notes, `ShareView`). Deferred entirely; the only player-facing artifact this spec keeps is the *player view rendering* (a DM tool: previews, future handouts).
- Sync between CLI projects (`$QUILLIT_HOME` filesystem) and web projects (database). Out of scope, but the data-model alignment in §4 is what makes it possible later.
- Any feature not present in the CLI spec §6 command set or required to support it.

**Fixed decisions** (made upfront; treat as constraints, not options):

1. **Storage**: the web app keeps its SQLite + MinIO stack. Entry bodies become annotated markdown in the CLI directive format (`:::secret`, `:::card <facet>`, `[[wikilinks]]`), rendered server-side via the CLI's self-contained Go package (CLI golden rule 7).
2. **Legacy removal**: the parallel campaign system (`campaigns`, `players`, share tokens, `player_notes`, `linkedEntries`) is removed. The project model absorbs everything.
3. **Editor**: TipTap WYSIWYG stays, extended with custom nodes for secret blocks, card blocks, and true wikilinks. It round-trips markdown + directives; HTML is never the stored artifact again.
4. **Content domain**: a new `quillit/content` service joins the monorepo. Monorepo folders are **domains** (bounded contexts): `content` owns everything entry-shaped — metadata, bodies, links, facets, rendering, PDF export, and search. `svc` retains the project/membership/game domain. `content` **wraps** the CLI's shared Go package (it does not reimplement it) — see §7. The CLI embeds the package in-process today and may optionally target the content API in the future.

CLI golden rules this refactor directly serves: **1** (one source of truth — kills the annotation sidecar and quick-view parallel content), **5** (player-safe by default — kills the leaky mark-stripping), **6** (fail loud — facet validation), **7** (reusable pipeline — the whole point of §7).

## 2. Terminology mapping

Old web term → CLI term, with disposition. "Remove" means the concept dies with no successor; "Replace" means a CLI concept takes over its job.

| Web app construct | CLI spec term | Disposition | Notes |
|---|---|---|---|
| Entry (`entries` table; `Entry` in `svc/internal/handler/entries.go:40`, `ui/src/types/index.ts:4`) | **Entry** | Keep | Name already aligned. Internals change: body format (§4.1), project linkage (§4.2). |
| Project (`projects`, `project_members`; `svc/internal/handler/projects.go:69`) | **Project** | Keep | Becomes the only workspace concept. |
| Campaign (`campaigns` table; `entries.campaign_ids` JSON column) | Project | Remove/merge | Absorbed by project. Tables dropped; `campaign_ids` replaced by `project_id` FK (§4.2). |
| Player (`players` table, share tokens, `player_notes`, `ShareView.vue`) | — (deferred) | Remove | Dies with the player experience. Only the *player view* rendering survives, as a DM tool. |
| "Notes" (route `/notes`, sidebar labels, 4-way overload: entries / annotations panel / session notes / player notes) | Entry | Rename | CLI spec §3 uses only *entry*. Full UI copy audit (§8). |
| Annotation, `visibility='gm'` (`annotations` table; `AnnotationMark` TipTap extension) | **`:::secret`** directive | Replace | Sidecar rows and inline marks migrate into inline body directives (§5). |
| Annotation `visibility='shared'` / `'player'` | — none | Open question | No CLI equivalent. Recommend drop with migration report (§10.1). |
| `stripGmMarks()` player redaction (`ui/src/composables/useAnnotationVisibility.ts:21-28`) | `:::secret` full strip | Replace | Current strip unwraps the mark but **leaves the secret text visible** — live spoiler leak. CLI semantics (strip block entirely) fix it. |
| Entry `visibility` `'private'`/`'public'` globe toggle (`ui/src/components/EntryEditor.vue:23-31`) | **View** (`dm`/`player`) | Replace | Visibility stops being an entry property; it becomes a render-time view. Column dropped. |
| `previewMode` boolean (`EntryEditor.vue:88-96`) | View (`dm` / `player` / `card <facet>`) | Replace | Becomes a three-way view switcher backed by the render endpoint (§8). |
| Quick view template (`quick_view_templates` table; `ui/src/config/quickViewTemplates.ts`) | **Facet** vocabulary (global ∪ project `extra_facets`) | Replace | DB-backed, kebab-case, fail-loud (§4.3). Client-side defaults deleted. |
| `entries.quick_view_data` (structured JSON per entry) | **`:::card <facet>`** blocks (freeform markdown) | Replace | Structured → freeform is lossy; migration rule §5, risk §10.2. |
| `QuickViewPanel.vue` | Card view | Rework | Becomes the card render / quick-edit surface (§8). |
| Category (free string on entry; `categories` + `project_global_categories` tables) | *conflates three CLI concepts* | **Split** | → organizational **directories** (primary successor), frontmatter **tags** (cross-cutting labels), and quick-view template names → **facets**. See §4.4. |
| TipTap `EntryMention` (`ui/src/extensions/EntryMention.ts:9-11` — displays `[[label]]`, stores entry **id**) | **`[[wikilink]]`** (path + optional label) | Replace | True path-addressed wikilink node (§8). |
| `entries.linked_entries` JSON | — | Remove | Already dead server-side; UI panel badges it "Simple Links — legacy". |
| `entry_relations` (typed, labelled, directional; `svc/internal/handler/relations.go`) | Wikilink + link index | Replace/remove | Body wikilinks become the source of truth; `entry_links` table (§4.6) is the `links.conf` analogue. Typed relations have no CLI equivalent — labels survive as wikilink labels during migration; the rest goes in the migration report. |
| `member_folders` / `member_folder_entries` / `member_entry_meta` (per-user, private) | — none | Remove (deferred with sharing) | Explicitly **not** the directory concept: CLI assignment is project-wide structure, these are per-user consumption features for shared entries. Must not be repurposed. |
| Category grouping in `QuillitView.vue:132-143` | **Assignment** / organizational directories | New | Project-wide directory tree; moving an entry = assignment (§4.4, §8). |
| — (no web equivalent) | Quillit home / `$QUILLIT_HOME` | Map | Web analogue = the server database + MinIO as a whole; global facet config lives in a global table (§4.3). |
| — (no web equivalent) | `links.conf` | New | Server-side compiled `entry_links` index table (§4.6). |
| — (no web equivalent) | Tags (frontmatter) | New | First-class, denormalized for filtering (§4.1). |
| Bulk import `POST /api/migrate/import` (`svc/internal/handler/migrate.go:33` — destructive truncate+reinsert, not project-aware) | — | Remove/rework | Incompatible with target model; any future import goes through the CLI parser. |
| Game-mode `note_card` chat snapshot (`svc/internal/handler/chat_ws.go:191-200` — verbatim HTML copy) | Player view render | Rework (deferred) | Rule recorded now: snapshots must be player-view render output from the shared package, never raw body (§9). |
| `project_members.role` `'gm'`/`'player'` (`projects.go:17-37`) | — | Keep `gm` only | `player` role stays in schema but inert until the player experience returns. |

## 3. Current state summary

Self-contained capture of the as-is system. Stack: Vue 3 + Pinia + vue-router + TipTap 3 + shadcn-vue + Tailwind 4 (`ui/`); Go + SQLite + MinIO (`svc/`); external auth service (out of scope).

### 3.1 Backend (`svc/`)

- **Schema** is code-defined in `svc/internal/db/db.go`, versioned by `PRAGMA user_version`, currently **v6** (`db.go:62-98`). No SQL migration files.
- **Entries** (`db.go:135-147`): `id` (24-char hex), `title`, `category` (free string), `body` / `body_key`, `visibility` (`'private'|'public'`), `campaign_ids` (JSON array — **this is the project membership**, legacy name), dead unused `project_ids` (`db.go:288`), `linked_entries` (round-tripped, unused), `tags`, `quick_view_data`, `owner_user_id`, timestamps. JSON columns are `json.RawMessage` passthrough; the only code parsing `campaign_ids` is `entryInProject()` (`chat_ws.go:208-219`).
- **Bodies are TipTap HTML**, stored in MinIO at key `entries/{id}/body.html` (`entries.go:195-196`) with inline-column fallback when MinIO is absent. Images at `entries/{id}/images/{imgID}.ext`. **No markdown library in `go.mod`; no render pipeline, sanitizer, or templating anywhere server-side.** The blob layout (folder per entry) is already shaped like the CLI's entry-folder model — just with HTML instead of `.md`.
- **Projects** (`db.go:210-216`): only type `campaign` → roles `["gm","player"]`. Reserved `id='global'` system project owns admin categories. Membership via `project_members` (unique project+user), invites via `project_invites` (single-use, 7-day TTL). No local users table — identity is JWT `sub` from the auth service.
- **Secrets analogue**: `annotations` sidecar table (`db.go:149-157`), `visibility` default `'gm'`, `shared_with` JSON never parsed server-side.
- **Facet analogue**: `quick_view_templates` (category name → JSON field schema, global, `quickview.go`); values in `entries.quick_view_data`. Default field sets live client-side (`ui/src/config/quickViewTemplates.ts`).
- **Categories**: two-tier — global admin `categories` + project-scoped rows (`UNIQUE(name, project_id)`), with `project_global_categories` opt-in junction (`db.go:340-378`). `entries.category` joins by *name*, never id.
- **Links**: `entry_relations` (typed/labelled/directional, bidirectional listing at `relations.go:35-63`) coexists with dead `linked_entries`.
- **Organization**: `member_folders` family (`db.go:252-277`) is per-user private, ownership-checked on every mutation; not project structure.
- **Legacy**: `campaigns`, `players` (public share `token`), `player_notes` tables and their handlers (`campaigns.go`, `share.go`) still routed.
- **Route table**: `svc/main.go:87-213`.

### 3.2 Frontend (`ui/`)

- **Editor**: TipTap emits/consumes HTML (`TiptapEditor.vue:37-50`, saves debounced 800 ms). Custom extensions: `AnnotationMark` (inline `<mark data-visibility>` spans) and `EntryMention` (`@`-triggered, renders `[[label]]` but stores entry id).
- **Secrets in UI**: entry-level private/public globe + inline annotation marks. Player redaction = `stripGmMarks()` unwrapping mark elements — underlying text remains.
- **Views**: no concept beyond the `previewMode` boolean (adds `.player-preview` CSS class, filters annotation panel).
- **Organization**: entries grouped by category string inside a project (`QuillitView.vue:132-143`); no author-facing directories; member `Folder` UI is a stub.
- **State**: Pinia stores per domain (`useEntriesStore`, `useProjectStore`, …) with IndexedDB cache-then-network via `usePersistence.ts`. API client: ofetch, `/api` base, cookie auth (`src/api/client.ts`).
- **Drift/dead**: `types/index.ts` missing many fields templates use; `PlayersView.vue`, `AdminCategoriesView.vue` unrouted; `gray-matter`/`marked` unused devDeps.

### 3.3 Known defects

| # | Defect | Where | Fate under this spec |
|---|--------|-------|----------------------|
| a | Player redaction leaks secret text: `stripGmMarks()` removes the highlight, not the content | `ui/src/composables/useAnnotationVisibility.ts:21-28` | **Fixed by replacement** — `:::secret` blocks are stripped entirely by the shared filter (§7). Until then, treat all "player" output as unsafe. |
| b | No read authorization: `GET /api/entries`, `GET /api/entries/{id}`, `GET /api/annotations` return everything to any authenticated user | `entries.go:97-155`, `annotations.go:46-67`; acknowledged in-code at `chat_ws.go:180-185` | **Must fix** — Phase 1 prerequisite (§6.1). |
| c | Malformed SQL (double `FROM`/`WHERE`) in public-share lookup; always falls through to `campaign_ids LIKE '%id%'` substring match, which can false-positive | `share.go:40-48` | Mooted — endpoint removed with legacy share system. |
| d | `sharedEntrySelect` selects 12 columns, scanner expects 13; error swallowed, `GET /api/member/shared` silently returns `[]` | `member.go:81`, `member.go:112` | Mooted — member sharing surface removed/deferred. (Also evidence `shared` visibility was never actually usable — see §10.1.) |
| e | `db.SetMaxOpenConns(1)` serialises the whole service on one SQLite connection | `db.go:25` | Revisit in Phase 3 — render + compile-on-save add read load (§10.9). |

## 4. Target data model & schema

Changes land as a `user_version` v6→v7(+) migration. Given the scale, introduce ordered, explicit migration steps (add columns → backfill → convert content → drop). See §5 for the content conversion itself.

**End-state home**: the entry-domain tables below (`entries`, `entry_links`, `facets`, `project_facets`) ultimately live in **`quillit/content`'s own database** (§7.2). Sequencing choice for Phase 2→3: either migrate in place in `svc`'s `quillit.db` and lift the tables into `content`'s DB at Phase 3, or stand up `content`'s DB first and migrate straight into it. Decide when Phase 3 design starts; the schema is identical either way. A search index (FTS5 over title/tags/body) is added in `content`'s DB alongside.

### 4.1 Entry content

- Body = **annotated markdown**, the single source of truth. MinIO key becomes `entries/{id}/body.md` (`text/markdown; charset=utf-8`); inline `entries.body` fallback also markdown.
- **Frontmatter is stored in the body** (`name`, `tags`), byte-identical to what a CLI entry file would hold. The `title` and `tags` columns become **denormalized copies** maintained on save, used only for querying/list views. Rationale: keeps web entry bodies directly portable to/from CLI entry files.
- New column `entries.slug` — kebab-case, unique within (project, directory). Required because CLI wikilinks are **path-addressed** (`characters/npcs/mary`), not id-addressed. Entry path = `directory_path` + `/` + `slug`.

### 4.2 Project linkage

- Replace `campaign_ids` (JSON) and the dead `project_ids` column with a single **`entries.project_id` FK + index**.
- Adopt the CLI's implicit rule as explicit: **an entry belongs to exactly one project** (an entry folder lives in exactly one project tree). Migration of multi-campaign entries: pick primary (first id in the array), report the remainder (§10.3).
- "Session notes" today are entries with `campaign_ids = '[]'` (`member.go:414` string comparison). Under the new model: `project_id NULL` is invalid; personal scratch entries are deferred with the member surface.

### 4.3 Facets

- New tables:
  - `facets(name TEXT PRIMARY KEY)` — global vocabulary. Direct analogue of home `config.yaml` `facets`.
  - `project_facets(project_id, name, UNIQUE(project_id, name))` — analogue of `quillit.yaml` `extra_facets`.
- Kebab-case enforced (CHECK or app-level, matching CLI: lowercase, digits, hyphens). Effective vocabulary = global ∪ project. Projects add, never remove global.
- Seed global facets = CLI defaults (`motivation`, `description`, `history`) plus any quick-view template names in active use (kebab-cased).
- **Fail loud** (golden rule 6): a body containing an undeclared facet errors at **save time** (editor UX) *and* at render time (defense in depth), naming the facet and the effective vocabulary. Never guess silently.
- Drop `quick_view_templates`; delete `ui/src/config/quickViewTemplates.ts`.

### 4.4 Category disambiguation (decision)

The web "category" conflates three CLI concepts. Explicit split:

| Job categories do today | CLI concept | Target mechanism |
|---|---|---|
| Grouping entries in the list view | **Organizational directory** (assignment) | New `entries.directory_path` TEXT (e.g. `characters/npcs`); empty = project root / unassigned. Assignment = updating the path. No `directories` table — a plain path column suffices (CLI directories carry no metadata either). |
| Cross-cutting labels / default tag suggestions | **Tags** (frontmatter) | Frontmatter `tags`, denormalized to a column for filtering. |
| Keying quick-view field templates | **Facet** | §4.3. |

Drop `categories`, `category_default_tags`, `project_global_categories`. Migration converts each entry's category name to a kebab-cased directory path (`Characters` → `characters/`) and assigns the entry there.

### 4.5 Secrets

- `annotations` rows with `visibility='gm'` migrate into `:::secret` blocks in the entry body (anchored at the marked span where `AnnotationMark` position data allows; appended under a trailing "Secrets" section otherwise). Table dropped post-migration.
- `shared`/`player` rows: no CLI equivalent — see §10.1.
- Entry-level `visibility` column dropped (§2: visibility is now a render-time view, not stored state).

### 4.6 Links

- **Wikilinks in the body are the source of truth.** New compiled index table — the server-side `links.conf`:

  `entry_links(entry_id, target_path, target_entry_id NULLABLE, label, card_facet NULLABLE, resolved BOOL)`

- Recompiled on entry save (web analogue of the CLI's lazy mtime-based recompile; a save *is* the staleness event). Dangling links recorded and surfaced as warnings, never errors — matching CLI `compile`.
- Drop `entries.linked_entries` and `entry_relations`. Relation labels are preserved as wikilink labels during migration; typed/directional semantics that don't translate go in the migration report.

### 4.7 Personal folders

`member_folders`, `member_folder_entries`, `member_entry_meta` are removed with the deferred sharing surface. Stated explicitly: they are **not** the directory concept and must not be repurposed as such.

### 4.8 Removals (full list)

- **Tables**: `campaigns`, `players`, `player_notes`, `annotations` (post-migration), `quick_view_templates`, `categories`, `category_default_tags`, `project_global_categories`, `entry_relations`, `member_folders`, `member_folder_entries`, `member_entry_meta`.
- **Columns on `entries`**: `campaign_ids`, `project_ids`, `linked_entries`, `visibility`, `quick_view_data`, `category`.
- **Added on `entries`**: `project_id` (FK, indexed), `slug`, `directory_path`.

## 5. Content migration (HTML → annotated markdown)

One-time conversion, run after the CLI parser exists (Phase 2). **The CLI parser is the acceptance test**: every migrated body must parse clean (no malformed directives, no undeclared facets) before cutover.

Pipeline per entry:

1. Fetch `entries/{id}/body.html` (or inline `body`).
2. Convert TipTap HTML → markdown. Expected constructs and targets: headings → `#`/`##`, lists, bold/italic, HR, links, images, text-align (drop or HTML-comment), `<mark class="annotation-mark--gm">` spans → fold into `:::secret` (with the corresponding `annotations` row content), `EntryMention` nodes → `[[directory/slug|label]]`.
3. Mention conversion is **id → path**, so slugs and directory paths must be backfilled **before** body conversion (ordering constraint: §4.4 category→directory and slug generation run first).
4. `quick_view_data` fields → one `:::card <facet>` block per facet with values, using a single fixed rule: field label as a bold lead-in (`**Role:** innkeeper`). One rule, applied uniformly — no per-entry judgment.
5. `annotations` (`gm`) → `:::secret` blocks per §4.5.
6. Images: `<img>` MinIO URLs become standard markdown images pointing at the same objects. Asset storage stays MinIO (divergence from CLI noted in §10.5).
7. Prepend frontmatter (`name` from title, `tags`).
8. Anything unconvertible is preserved as-is (raw HTML passthrough is legal CommonMark) and **flagged in the migration report** — never silently dropped.

Safety: mandatory **dry-run producing a full report** (per-entry conversion status, multi-project picks, dropped annotation rows, unconvertible fragments, dangling mention targets) before destructive cutover. `body.html` objects are retained until the migration is verified and signed off.

## 6. API surface changes

### 6.1 Prerequisite: authorization & project scoping

Lands before anything else (Phase 1; independent of the CLI):

- Every entry read is filtered by `project_members` membership. `GET /api/entries` becomes project-scoped — `GET /api/projects/{id}/entries` (preferred) — with a membership check; `GET /api/entries/{id}` checks membership via the entry's project. Closes the current read-anything hole (§3.3b).
- The annotations leak closes when the endpoint is removed; until then it is a known exposure.

### 6.2 Removed

`/api/campaigns/*`, `/api/share/*`, players endpoints, `/api/annotations*` (post-migration), `/api/quickview*`, `/api/categories*` and `/api/projects/{id}/categories*`, `/api/member/*`, `/api/entry-relations*` + `/api/entries/{id}/relations` + `/api/relation-labels`, `/api/migrate/import`.

### 6.3 Changed

- **Entry CRUD**: body is markdown; create/update accept `slug` and `directory_path`; server validates directives and facets on write (fail loud — structured error naming the offending facet and the effective vocabulary). Blob key `body.md`.
- **Entry list**: project-scoped (§6.1); returns path data (`directory_path`, `slug`) so the UI can build the directory tree. Bodies stay lazily hydrated (list omits body, detail resolves it) — unchanged pattern.

### 6.4 New

| Endpoint | CLI analogue | Behavior |
|---|---|---|
| `GET /api/entries/{id}/render?view=dm\|player` and `?card=<facet>` (+ `quick=1`) | `render` / `-Q` | Server-side render via the shared package (§7). `view` and `card` mutually exclusive; `card` implies DM audience. **The only way player-facing HTML is ever produced** (golden rule 5). |
| `GET /api/entries/{id}/export?view=…` / `?card=…` (+ `with=` bundle list) | `export` | PDF export via the shared `export` package. |
| `GET/POST/DELETE /api/facets`; `GET/POST/DELETE /api/projects/{id}/facets`; GET returns effective vocabulary | `config` / `config add\|rm --facet` | Removal doesn't touch bodies; entries still using the facet fail loud at save/render, mirroring CLI `config rm`. |
| `POST /api/entries/{id}/assign` `{directory_path}` | `assign` | Creates the path implicitly; fails on slug collision at destination. |
| `GET /api/entries/{id}/links` | `links.conf` read | Compiled index incl. dangling-link warnings. |
| `POST /api/projects/{id}/compile` | `compile --all` | Optional; recompiles all `entry_links`. Normal path is compile-on-save. |
| `GET /api/projects/{id}/entries/lookup?q=` | — | Wikilink autocomplete for the editor (`[[` trigger): search by title/path, returns paths. |

Service placement: **all entry-domain endpoints above — metadata CRUD, body, render, export, assign, images, links/compile, facets, search/lookup — are served by `quillit/content`** (§7.2, from Phase 3; until then they live in `svc` only if implemented early). `svc` keeps only project/membership/invite/game-mode endpoints. The `/api/entries*` paths in §6.1–6.4 describe the *shapes*; their final home is `content`, and gateway routing decides the public prefix (`/api/content/*` or transparent).

### 6.5 Infra notes

- `SetMaxOpenConns(1)` revisited when render endpoints land (§10.9).
- Game-mode rule recorded now, implemented later: `note_card` snapshots (`chat_ws.go:191-200`) must snapshot **player-view render output** from the shared package, never the raw body.

## 7. Content engine — shared package + `quillit/content` service

Two layers, one implementation:

1. **Shared Go package** (built in Phase 0 as part of the CLI) — parsing, filtering, rendering, link indexing, PDF export. Embeddable; no I/O assumptions.
2. **`quillit/content` service** (new monorepo service, Phase 3) — wraps the package behind an HTTP API and owns content storage. Consumers: the web UI (via gateway) and, optionally in the future, the CLI.

### 7.1 Shared package — contract with the CLI

This subsection is directed at the **CLI implementer** (Phase 0). It operationalizes golden rule 7 so the package is service-wrappable from day one rather than retrofitted.

**Suggested package boundaries** (one Go module area, no CLI/cobra imports):

| Package | Responsibility |
|---|---|
| `parse` | Frontmatter + `:::` directive blocks + wikilinks → structured entry (AST). |
| `filter` | Apply view: strip `:::secret` for `player`; extract matching blocks for `card <facet>`; quick-view (`-Q`) truncation. |
| `render` | Filtered AST → **HTML fragment**. |
| `linkindex` | Extract link records (target path, label, containing card facet) — the `links.conf` content as a Go type. |
| `export` | Rendered view(s) → **PDF**, including combined bundles (CLI `--with-links`). PDF engine choice open (pure-Go lib vs headless renderer) but must work inside both a static CLI binary and the content service container. |

**Hard requirements:**

1. **No filesystem coupling in core.** Link resolution goes through an interface — `Resolver` (path → exists? / target metadata / card content for facet). CLI implements it over the project tree + `links.conf` files; web implements it over SQLite + `entry_links`. Facet vocabulary is **passed in as a value**, never read from `config.yaml` inside the package.
2. **Fragment is the core output.** CLI wraps the fragment in the entry's scaffolding files (`.html`/`.css`/`.js`); web injects it into its own app shell. Full-document assembly is CLI-side, outside the shared package.
3. **Depth-1 card expansion lives in the shared package** — it is semantics (CLI spec §4), not presentation. The *presentation* of non-expanded links is a caller-supplied link-renderer callback: CLI emits clipboard-links (copy `quillit render …` command); web emits in-app navigation links for the same cases. Player view/export link handling (plain label only) is `filter`-enforced, not callback-dependent.
4. **Errors as values.** Typed errors — `UnknownFacet{Name, Vocabulary}`, `MalformedDirective{…}`, etc. No `os.Exit`/`log.Fatal` in the package; golden rule 6 presentation is the caller's job.
5. **Player-safety as a package guarantee**: `filter(view=player)` output contains zero secret content, including in expanded or bundled entries. The web app must never produce player-bound output through any other path.

Acceptance criterion for Phase 0: the CLI's own `render`/`export` are thin callers of this package, and the package compiles with no dependency on `$QUILLIT_HOME`, cobra, or the terminal.

### 7.2 `quillit/content` service (Phase 3)

New service in the monorepo (`quillit/content`, Go, own Dockerfile, **own SQLite database** — same shape as `svc`/`auth`). Monorepo folders are **domains**: `content` is the bounded context for everything entry-shaped. It wraps the §7.1 package for all parsing/filtering/rendering — no reimplementation — and adds storage, metadata, and search around it.

**Domain ownership** — moves from `svc` to `content`:

- **Entry metadata** — the `entries` table (id, project_id, slug, directory_path, denormalized title/tags, timestamps) and `entry_links` live in `content`'s database.
- **Entry bodies & images** — MinIO objects (`entries/{id}/body.md`, `entries/{id}/images/*`).
- **Facet vocabulary** — `facets` + `project_facets` (§4.3): facets govern directive validity, so they are content-domain config.
- **Assignment** — directory-path moves (the `assign` operation).
- **Rendering** — dm/player/card/quick views via shared `filter`+`render`; implements the `Resolver` over `entry_links`; depth-1 card expansion with in-app link callback.
- **PDF export** — single-entry and bundled (`--with-links` analogue) via the `export` package; brings web PDF export (previously deferred) into scope.
- **Search** — full-text + field search over entries (SQLite FTS5 over title/tags/body is the natural fit; index refreshed on save). Also backs the wikilink autocomplete. New capability with no CLI-spec analogue — a web/API addition, not a CLI alignment item.
- **Validation & link index** — parse/facet validation on body writes (fail loud, typed errors → structured HTTP errors); recompiles `entry_links` on save.

**`svc` retains**: projects, membership, invites, roles, game mode, admin — the collaboration domain. `svc`'s `entries` handlers are removed entirely.

**API sketch** (behind the same gateway; cookie/JWT auth like `svc`):

| Endpoint | Wraps |
|---|---|
| `GET/POST /content/projects/{id}/entries`; `GET/PATCH/DELETE /content/entries/{id}` | entry metadata + body CRUD (writes validate + recompile links) |
| `GET /content/entries/{id}/render?view=dm\|player` / `?card=<facet>` / `&quick=1` | `filter`+`render` |
| `GET /content/entries/{id}/export?view=…&card=…&with=…` | `export` (PDF) |
| `POST /content/entries/{id}/assign` | assignment (directory move; slug-collision check) |
| `POST /content/entries/{id}/images` | image storage |
| `GET /content/entries/{id}/links`, `POST /content/projects/{id}/compile` | `linkindex` |
| `GET/POST/DELETE /content/facets`, `/content/projects/{id}/facets` | facet vocabulary (effective = global ∪ project) |
| `GET /content/projects/{id}/search?q=` | search (FTS); also serves `[[` autocomplete |

**Cross-domain boundary:** the one seam is authorization — `content` scopes everything by project but does not own membership. `content` verifies the same JWT, then resolves membership via an internal `svc` endpoint (with short-lived caching) or a shared claims convention. Project deletion in `svc` must notify `content` (delete/orphan entries) — the single cross-domain event. Details at Phase 3 design (§10.11).

**CLI relationship:** the CLI **embeds the package in-process** — it stays a single offline static binary (CLI spec §8); it never requires the service. A future authenticated CLI *may* target the content API (e.g. operating on server-hosted projects); that is the sync/remote scenario, out of scope (§10.10), but the API shapes above are deliberately CLI-command-shaped (`render`, `export`, `compile`, `assign`) to keep that door open.

## 8. Editor & UI changes

- **Markdown round-trip.** TipTap serializes to / parses from markdown + directives. The stored artifact is always the annotated markdown; the TipTap document is an ephemeral projection. (`gray-matter`/`marked` devDeps — currently unused — either get used here or removed.)
- **`SecretBlock` node** — visually distinct block in the editor; serializes `:::secret … :::`. Replaces `AnnotationMark` (`ui/src/extensions/AnnotationMark.ts`), the annotations panel, and the visibility globe.
- **`CardBlock` node** — `facet` attribute; facet picker constrained to the effective vocabulary fetched from the facet API (fail-loud surfaced as inline validation). Serializes `:::card <facet> … :::`. Cards inside secrets allowed (CLI spec §4).
- **Wikilink node** replacing `EntryMention` (`ui/src/extensions/EntryMention.ts`): stores `path` + optional `label`, serializes `[[path|label]]`; `[[`/`@` autocomplete backed by the lookup endpoint (§6.4); dangling-target visual state.
- **View switcher** replacing `previewMode` (`EntryEditor.vue:88-96`): **DM (edit)** / **Player preview** / **Card preview** (facet select). Previews are fetched from `GET …/render`, not client-rendered — guaranteeing parity with the shared package and the CLI, and making the leaky client-side strip logic (`useAnnotationVisibility.ts`) deletable.
- **Organization UI**: `QuillitView` regroups from category → directory tree built from `directory_path`; drag/move = `assign` call; project-root bucket = "unassigned" (mirrors CLI: new entries land in project root until assigned).
- **Facet management UI**: global + project scopes (replaces quick-view template editing and `AdminCategoriesView`'s role).
- **Search**: dashboard/global search re-backed by the `content` search endpoint (FTS over title/tags/body) instead of client-side filtering over cached lists; body search becomes possible for the first time (bodies are lazily hydrated today, so client-side search never saw them).
- **`QuickViewPanel` rework**: card-blocks summary per facet + the `quick=1` quick-view rendering.
- **Removals**: `ShareView.vue`, `PlayersView.vue`, `AdminCategoriesView.vue`, annotations "Notes" panel, `useAnnotationVisibility.ts`, `usePlayerNotes.ts`, campaign store/views, legacy link panel section.
- **Types**: `ui/src/types/index.ts` reconciled to the new `Entry` shape; fix the existing declared-vs-used drift while touching it.
- **Offline cache**: IndexedDB entries cache markdown source (+ rendered fragments if kept); cutover must invalidate stale HTML-body caches; cached player renders must also be package-produced (never client-derived).
- **Copy audit**: "entry" everywhere a note-like thing is meant; "Notes" label retired; route `/notes` → `/entries` (redirect kept).

## 9. What stays untouched

Explicit, to prevent scope creep: the auth service and session handling; MinIO infrastructure (blob *access* moves to the `content` domain in Phase 3, deployment unchanged); project/membership/invite CRUD in `svc` (minus role semantics — `player` role inert; and minus its entry handlers, which move to `content`); game mode (frozen as-is, with the §6.5 `note_card` rule recorded for its eventual rework); dashboard/app chrome; docker/deploy setup (plus one added compose service + DB for `content` in Phase 3, and `backup.sh` gaining its database).

## 10. Gaps, risks, open questions

1. **`shared`/`player` annotation visibilities** — no CLI equivalent; `shared_with` was never parsed server-side, and the §3.3d bug means shared-entry retrieval silently returned `[]`, so real usage is likely near zero. **Recommend: drop, with a migration report listing affected rows.**
2. **Structured `quick_view_data` → freeform card blocks** loses field-level structure (no per-field querying). **Recommend: accept**; the bold-label convention (§5.4) is the escape hatch if structure ever matters again.
3. **Multi-project entries** — CLI model is single-project; migration picks a primary and reports the rest. Risk: genuine multi-campaign entries exist. Mitigation: dry-run report reviewed before cutover; worst case, duplicate manually.
4. **HTML→markdown fidelity** — TipTap-specific HTML (alignment, nested marks) may not convert cleanly. Mitigation: raw-HTML passthrough + report (§5.8), `body.html` retained until sign-off.
5. **Images** — the CLI spec is silent on non-`.md` assets in entry folders; web bodies will contain app-served MinIO URLs a CLI could not resolve. Acceptable now (no sync); flagged as CLI amendment (§12.5).
6. **Wikilink path rot on assign** — moving an entry changes its path and breaks inbound `[[path]]` links. CLI behavior: dangling links surface as warnings (compile/render). Web can go further: **auto-rewrite inbound links via `entry_links`** (cheap with a global index). **Recommend: web auto-rewrites**, with the CLI divergence noted; see CLI amendment §12.3.
7. **Fail-loud UX in a web editor** — save-time facet errors vs work-in-progress drafts. **Recommend: block save with inline error** (mirrors CLI; golden rule 6). No silent coercion.
8. **Offline cache migration** — IndexedDB format change; stale caches invalidated on cutover (§8).
9. **`SetMaxOpenConns(1)`** — render + compile-on-save add read load; revisit connection strategy (e.g. separate read pool, WAL already enabled) in Phase 3.
10. **CLI↔web project sync** — explicitly out of scope. The alignment in §4 (paths, slugs, frontmatter-in-body, identical directive format) and the CLI-command-shaped content API (§7.2) are deliberately what make a future import/export, sync, or remote-CLI mode feasible; no commitments here.
11. **`svc`↔`content` domain seam** — with metadata moved into `content`, an entry is wholly owned by one domain (no split-save consistency problem). Remaining seams: (a) authorization — `content` resolves project membership from `svc` (internal endpoint + short-lived cache, or shared JWT claims); (b) project deletion — `svc` must emit a delete event `content` honors (drop or orphan entries); (c) operational — a third SQLite DB joins `backup.sh` and the compose stack. Decide details at Phase 3 design. Fallback if a separate service proves heavy for self-hosting: ship `content` as a Go module with its own DB inside the `svc` process, extract later — domain boundary holds either way.

## 11. Phasing

| Phase | Depends on | Content |
|---|---|---|
| **0 — CLI** | — | Build the CLI per its spec (P0→P2). §7 package contract is an acceptance criterion of this phase. **No web changes.** |
| **1 — Web prerequisites** | none (parallel with 0) | Authorization + project scoping (§6.1); remove legacy campaign/player/share code, dead columns, dead views; copy-audit groundwork. No content-model change — safe anytime. |
| **2 — Schema & content migration** | 0 (parser) | v7+ migration (§4.8 adds), slug/directory backfill, then HTML→markdown conversion (§5) validated by the CLI parser; dry-run → report → cutover → drops. |
| **3 — Content domain** | 2 | Stand up `quillit/content` (§7.2): own DB; entry metadata, `entry_links`, facet tables move in; shared package behind entry CRUD/render/export/assign/links/facets/search endpoints; `svc` entry handlers removed; membership-resolution + project-delete seam (§10.11); FTS index. |
| **4 — Editor rework** | 3 | Markdown round-trip, `SecretBlock`/`CardBlock`/wikilink nodes, view switcher, AnnotationMark removal (§8). |
| **5 — Organization & polish** | 4 | Directory-tree UI, facet management UI, QuickViewPanel rework, types cleanup, cache migration, copy audit completion. |
| **Deferred indefinitely** | — | Player experience, web PDF export, game-mode rework, CLI↔web sync. |

Rationale: Phase 1 is pure security/debt paydown and needs nothing from the CLI; Phases 2–4 hard-depend on the shared package existing and proven inside the CLI first.

## 12. Recommended amendments to the CLI spec

Surfaced by this exercise; all small and additive:

1. **§3 terminology** — add a line distinguishing **facet** (card category) from organizational **directories** (assignment) and frontmatter **tags** (cross-cutting labels). The web app's "category" conflated all three; naming the distinction stops the CLI reintroducing it.
2. **Single-project entries** — state explicitly that an entry belongs to exactly one project. Implicit in the folder model; the web refactor (§4.2) depends on it as a rule.
3. **§7 `assign` link rot** — define behavior when an assigned (moved) entry has inbound wikilinks: at minimum, `compile`/render report them as dangling. Note that inbound-link rewriting is out of scope for the CLI but the web app will do it (§10.6).
4. **Golden rule 7 → package contract** — expand into (or reference) the §7.1 sketch: fragment-vs-document output, `Resolver` interface, vocabulary-as-value, typed errors, PDF export in the shared `export` package. Cheaper to build to this from day one than retrofit. Note the package's second consumer is the planned `quillit/content` service (§7.2), not just "the web app" abstractly.
5. **Assets** — one line on non-`.md` files in entry folders (images) and how render/export treat them; the web needs a compatible convention eventually.
6. **Frontmatter `tags`** — appear in the §4 example but have no defined behavior; mark explicitly as inert metadata for now so no ad-hoc tag features grow.
7. **PDF engine constraint** — CLI spec §8 ships a single static binary; the shared `export` package's PDF approach must satisfy that *and* run in the `content` service container. Worth a one-line implementation note so the CLI implementer doesn't pick a host-dependent toolchain (e.g. system wkhtmltopdf).
