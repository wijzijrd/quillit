# Quillit CLI — Specification

## 1. Goal & core need

Quillit grew past original purpose. Spec resets project to simple, actionable CLI serving core need, ahead of bigger web app (comes later, with integrations).

**Core need:** better notes for running D&D game. Today one idea (NPC, location, plot beat) needs **three separate files**:

1. **Full DM version** — complete outline, plot, secrets.
2. **Player-safe version** — same notes, spoilers redacted, manual bookkeeping of what players may see.
3. **Flash-card version** — quick-reference extracts (character motivation, description, history) for table use. Needed extract changes daily: today motivation card, tomorrow description card.

CLI collapses this into **one annotated `.md` file per entry**. Annotations mark secret sections and flash-card blocks; render/export commands produce any of three views from single source of truth.

**Instruction to implementer:** evaluate spec against existing quillit repo, reuse what fits. Current web app is **Vue + Go**.

## 2. Golden rules

Scope guardrails. Every proposed change — human or AI — must pass these before implementation. Change conflicts with rule → change loses.

1. **One source of truth.** Entry `.md` is only authored content; every view (DM, player, card) derives from it. Reject any feature needing hand-maintained parallel versions of same content.
2. **Serve the table.** Feature must help DM prep or live play — three-views need in §1. Integrations and cool-factor belong to future web app, not CLI.
3. **Simple and actionable.** Build smallest implementation meeting stated need; no speculative generality. Change not mapping to §6 command at P0–P2 → out of scope.
4. **Cheap by default.** Bound expensive work: link expansion stops at depth 1, indexes (`links.conf`) replace re-scanning, anything deeper is clipboard-link DM runs manually.
5. **Player-safe by default.** Anything reaching players (player view, exports) must strip secrets — including bundled/linked entries. In doubt → redact.
6. **Fail loud, fail clear.** Typos and misuse (unknown facet, bad path, outside project) error immediately, name fix. Never guess silently.
7. **CLI now, web app later — as a package contract, not a slogan.** Keep render/filter pipeline a self-contained Go package the Vue + Go web app (and, later, the `quillit/content` service that owns the web app's entry domain) can import directly. No web features now, but the boundary must already be right:
   - **Output is a fragment, not a document.** The package returns rendered HTML for the entry's content only — no `<html>`/`<body>` wrapper. Assembling that fragment into the CLI's own browser page (via the entry's `.html`/`.css`/`.js` scaffolding, §5) is CLI-side, outside the package.
   - **Link resolution is an interface, not a filesystem call.** The package resolves wikilinks through a `Resolver` (does this path exist? what's its card for facet X?) that the caller supplies. The CLI implements it over the project tree + `links.conf`; a future service implements the same interface over a database. The package itself never touches disk.
   - **Facet vocabulary is a value, not a file read.** The package never reads `config.yaml`/`quillit.yaml` itself — the effective vocabulary (global ∪ project) is passed in by the caller.
   - **Errors are values.** Unknown-facet, malformed-directive, and similar failures are typed errors returned to the caller, not `os.Exit`/`log.Fatal` calls buried in the package — golden rule 6 (fail loud) is the caller's presentation choice, not baked into the package's control flow.
   - **PDF export lives in the same package family.** `export` (§7) is built as another self-contained package alongside parse/filter/render/linkindex, not a one-off script, since it has the same two consumers.
   - This package's consumers are, from day one, plural: the CLI itself, and — later — the `quillit/content` service that becomes the web app's entry-domain backend. Build to that contract now rather than retrofitting it once a second consumer shows up.

## 3. Terminology

- **Entry** — single note (NPC, location, session, etc.). Own folder. (Draft spec mixed "entry"/"note"; this spec uses only *entry*.) An entry belongs to exactly **one** project — its folder lives in exactly one project tree, never split across or shared between projects.
- **Project** — quillit workspace: directory tree of entries, config file at root. All projects live inside quillit home.
- **Quillit home** — directory `$QUILLIT_HOME` points at: holds global config and every project. Bootstrapped by `init` on first run.
- **View** — rendering of entry: `dm` (everything), `player` (secrets stripped), or `card <facet>` (only matching flash-card blocks).
- **Facet** — flash-card category (e.g. `motivation`, `description`, `history`). Global facets defined once in home config, apply to every project; project config may declare extra facets, project-only. A facet is **not** a grouping mechanism — it only names which flash-card blocks belong together. Two other concepts that sound similar are deliberately different: **directories** (below) group entries for browsing/filing, and **tags** (§4) are freeform cross-cutting labels on a single entry. Don't reach for a facet to organize entries or to label them — that's what directories and tags are for.
- **Assignment** — moving entry folder from project root into organizational directory (e.g. `characters/npcs/`). Directories are the organizing mechanism for entries; they carry no vocabulary or schema of their own, just a filing location.
- **Current project** — pointer in home config naming project commands act on when cwd not inside one. Set with `quillit connect`.
- **Link** — wikilink from one entry's `.md` to another entry (e.g. character's spouse).
- **Link index** — generated `links.conf` in entry folder listing outgoing links, so render/export skip re-scan.

## 4. Annotation format

Entry `.md` files use fenced **directive blocks**:

- `:::secret … :::` — DM-only content. Stripped entirely from `player` view.
- `:::card <facet> … :::` — flash-card block for named facet. Card blocks part of normal DM view; card render collects only blocks matching requested facet.

Example (`characters/npcs/tom/tom.md`):

```markdown
---
name: Tom the Innkeeper
tags: [npc, waterdeep]
---

# Tom the Innkeeper

Tom runs the Gilded Goose inn. He rarely speaks of [[characters/npcs/mary|Mary]].

:::secret
Tom is secretly a spy for the Crimson Hand.
:::

:::card motivation
Wants to buy back his family farm.
:::

:::card description
Round-faced, ale-stained apron, booming laugh.
Spouse: [[characters/npcs/mary|Mary]]
:::
```

Frontmatter `tags` (as in the example above) is accepted and stored but **inert for now** — no command reads, filters, or renders by it yet. Treat it as metadata reserved for a future feature, not something to build ad-hoc tooling around today.

View semantics for example:

| View | Output |
|------|--------|
| `dm` (default) | Everything: secret + all card blocks. |
| `player` | Everything **except** `:::secret` block. |
| `card motivation` | Only "Wants to buy back his family farm." (plus entry title/frontmatter header). |

**Facet vocabulary fixed**: global facets in home config, plus project-only additions in project config (§5). Render with unknown facet, or `.md` containing undeclared facet → error — catches typos.

Card blocks may sit inside secret blocks; such cards DM-only, still render in card view (card view is DM tool).

### Links

Entries link via **wikilinks**: `[[path/to/entry]]` or `[[path/to/entry|Label]]`. Path relative to project root (entry folder path, e.g. `characters/npcs/mary`).

**Card expansion — one layer deep.** Rendering card: link **inside rendered card block** pulls linked entry's **same-facet card**, inlined below main card. Example above: `quillit render characters/npcs/tom --card description` renders Tom description card then Mary description card. Links elsewhere in entry (Tom's opening paragraph) don't trigger expansion.

**Depth capped at 1.** Transitive link processing expensive, so links inside inlined (expanded) card do **not** expand further. They render as **clipboard-links**: click copies CLI command to view entry — e.g. `quillit render characters/npcs/mary --card description` — to clipboard (via JS in entry scaffolding); DM runs manually. Non-expanded links in full DM view behave same.

**Links are DM-render-only.** Expansion and clipboard-links exist only in DM browser render. Player view and PDF export show link's plain text label only — no commands, no expansion.

Linked entry lacks card for requested facet → link renders as clipboard-link with small "no `<facet>` card" note; render doesn't fail.

### Assets

Entry folders may contain files other than `.md`/`.html`/`.css`/`.js`/`links.conf` — most commonly images referenced from the body via standard markdown image syntax (`![alt](tom-portrait.png)`, path relative to the entry folder). `render`/`export` don't process these files themselves; they pass through untouched, resolved as relative paths by whatever renders the markdown (the browser for `render`, the PDF pipeline for `export`). No convention beyond "put the file in the entry folder and reference it by relative path" is required or enforced.

## 5. Quillit home & project layout

### Quillit home

All projects live in single home directory, located by **`$QUILLIT_HOME`** env var:

```
$QUILLIT_HOME/
├── config.yaml           # global config — global facet vocabulary
├── curse-of-strahd/      # a project (see project layout below)
└── one-shots/            # another project
```

`config.yaml` holds **global facets** (available every project) and **current project** pointer:

```yaml
facets:
  - motivation
  - description
  - history
current_project: curse-of-strahd   # set by `quillit connect`; may be empty
```

**Facet resolution:** effective vocabulary for project = union of global facets and project extras. Projects add facets, never remove global.

### Project layout

`quillit init <project_name>` creates project inside `$QUILLIT_HOME` (bootstraps home first if needed — see `init` in §7):

```
<project_name>/
├── quillit.yaml          # project config — its presence marks the project root
├── characters/           # starter organizational folders (suggestions, not enforced)
├── locations/
└── sessions/
```

`quillit.yaml` holds at minimum:

```yaml
name: <project_name>
extra_facets: []          # project-only facets, added to the global vocabulary
                          # e.g. [stat-block, loot]
```

**Project resolution order** (every command needing project): (1) walk up from cwd to nearest `quillit.yaml`; (2) else fall back to `current_project` in home config (set via `quillit connect` — survives closing terminal). Neither resolves → commands other than `init`, `connect`, `config`, `version`, `--help` fail with clear error suggesting `quillit connect <project_name>`. Entry paths then relative to resolved project root.

**Entry folder anatomy** — `quillit create <entry_name>` produces:

```
<entry_name>/
├── <entry_name>.md       # the source of truth — frontmatter skeleton (name, tags)
│                         # plus commented example :::secret and :::card blocks
├── <entry_name>.html     # render scaffolding: working HTML template the render
├── <entry_name>.css      # command injects rendered content into; usable immediately
├── <entry_name>.js       # without editing (includes clipboard-link handler)
└── links.conf            # generated link index (see `compile`) — not created until
                          # the entry is first compiled/rendered/exported
```

All four template files created **from working templates** — fresh entry renders with no manual setup. New entries live in **project root** until assigned.

**`links.conf`** generated, never hand-edited. Lists entry's outgoing wikilinks — target path, label, whether link sits inside card block (which facet) — so export/render resolve links without re-scanning `.md` files.

## 6. Command reference

| Command | Arguments & flags | Priority | Description | Example |
|---------|-------------------|----------|-------------|---------|
| `--help` | global flag; also per-command | P2 | List commands; with command, show its usage. | `quillit --help`, `quillit render --help` |
| `version` | — | P2 | Print version + installed path of quillit binary. | `quillit version` |
| `init` | `<project_name>` | P1 | Create project inside `$QUILLIT_HOME` (config + starter folders, §5). `$QUILLIT_HOME` unset/missing → bootstrap home first (§7). | `quillit init curse-of-strahd` |
| `connect` | `<project_name>` | **P0** | Set **current project** pointer in home config — commands work from any directory (e.g. after closing terminal). No argument: print connected project. | `quillit connect curse-of-strahd` |
| `config` | — (print); `add --facet <name> [--scope global\|<project_name>]`; `rm --facet <name> [--scope global\|<project_name>]` | P2 (print), P1 (add/rm) | Bare `config`: print global config + current project config. `add`/`rm`: add/remove facet; `--scope` defaults **global**, or names project to edit its `extra_facets`. | `quillit config add --facet stat-block --scope curse-of-strahd` |
| `create` | `<entry_name>`; `--assign`/`-A [<directory>]` | **P0** | Create entry (folder + 4 templated files, §5) in project root. `-A`: assign immediately; directory omitted → prompt. Without `-A`, use `quillit assign` later. | `quillit create tom -A characters/npcs` |
| `assign` | `<entry> <directory>` | **P0** | Move entry folder from project root into target directory (created if missing). | `quillit assign tom characters/npcs` |
| `edit` | `<path_to_entry>` | **P0** | Open entry folder in user editor (`$VISUAL`/`$EDITOR`, fallback OS default opener). | `quillit edit characters/npcs/tom` |
| `render` | `<path_to_entry>`; `--view dm\|player`; `--card <facet>`; `--quick-view`/`-Q` | **P0** | Build entry `.md` into HTML using its scaffolding files, open in default browser. Default view `dm`. `--card`: flash card for facet. `-Q`: summarized version (frontmatter + first section). | `quillit render characters/npcs/tom --card motivation` |
| `compile` | `<path_to_entry>` or `--all` | P1 | Scan entry `.md`, write `links.conf` link index. `--all`: every entry in project. Render/export auto-recompile stale indexes — this mainly forces/warms cache. | `quillit compile characters/npcs/tom` |
| `export` | `[<path_to_entry>]`; `--view dm\|player`; `--card <facet>`; `--with-links [all]`; `--with <entries>` | P1 | Render to **PDF**. No path → export **all** entries. Same view flags as `render` — e.g. player-safe PDF handout. `--with-links`: interactive checklist of linked entries to bundle into one combined PDF. | `quillit export characters/npcs/tom --view player --with-links` |

**Priorities:** P0 = `connect`, `create` + `assign`, `edit`, `render` (MVP loop: connect, make note, file it, read at table). P1 = `init`, `compile`, `export`, `config add`/`config rm`. P2 = `version`, bare `config`, `--help` polish.

## 7. Behavior details

### `init`
- **Home bootstrap**: `$QUILLIT_HOME` unset, or set but directory missing → `init` runs first-time setup before creating project:
  1. Prompt for home location (default `~/quillit`; `$QUILLIT_HOME` set but missing → use its value).
  2. Create directory + `config.yaml` seeded with default global facets (`motivation`, `description`, `history`).
  3. CLI can't persist env var for user's shell — print exact line for shell profile (e.g. `export QUILLIT_HOME="$HOME/quillit"`) and which file.
- Project created at `$QUILLIT_HOME/<project_name>` regardless of cwd. Fail if project name exists.
- After create, set as current project (same effect as `quillit connect <project_name>`).

### `connect`
- Writes `current_project: <project_name>` to home `config.yaml` after checking `$QUILLIT_HOME/<project_name>/quillit.yaml` exists; else error listing available projects.
- No argument → prints connected project (or "none").
- Inside project directory always beats pointer (resolution order, §5) — `connect` is fallback for working elsewhere.

### `config`
- Bare `quillit config`: prints global `config.yaml` + resolved current project `quillit.yaml`, clearly labeled, plus effective facet vocabulary (global ∪ project).
- `config add --facet <name>`: adds to global list, or project `extra_facets` with `--scope <project_name>`. `--scope global` default. Duplicate (already in effective vocabulary at scope) → no-op with note.
- `config rm --facet <name>`: removes from chosen scope. Removal doesn't touch `.md` files — entries still using facet fail loud at render (golden rule 6); command prints reminder.
- Facet names: lowercase, digits, hyphens (kebab-case) — keeps directive parsing unambiguous.

### `create`
- Entry name = folder + file basename. Fail if entry name exists at target location.
- `-A` no directory → interactive prompt for target.
- `.md` template includes commented `:::secret` and `:::card` examples so annotation syntax discoverable.

### `assign`
- Physically moves entry folder (`mv` equivalent). Fails clearly if entry missing from root or destination has same-name entry.
- `create -A` = shorthand for `create` then `assign`.
- **Link rot on move**: assigning an entry changes its path, which can break inbound `[[path]]` wikilinks in *other* entries that pointed at its old location. `assign` itself doesn't scan the whole project for inbound references (that's expensive — golden rule 4). Instead, any entry with a now-broken inbound link surfaces it the normal way: the next `compile`/`render`/`export` touching that entry reports the link as dangling (§7 `compile`), same as any other broken wikilink. Rewriting inbound links automatically is out of scope for the CLI — it would mean scanning every entry on every `assign`. (A future web app with a persistent link index across the whole project can do this cheaply; the CLI intentionally doesn't.)

### `render`
- Pipeline: parse `.md` → apply view filter (strip secrets for `player`; extract matching blocks for `--card`) → resolve links (§4, DM renders only: expand same-facet cards one layer, clipboard-links beyond) → markdown to HTML → inject into entry `.html` template (with `.css`/`.js`) → write to temp/build location → open in default browser.
- Link resolution reads `links.conf`, recompiles first if `.md` newer (see `compile`).
- `--view` and `--card` mutually exclusive; `--card` implies DM audience.
- Unknown facet (not in global config or project `extra_facets`) → error listing effective facet vocabulary.
- `-Q` (quick view) works with any view: frontmatter summary + first content section only.

### `compile`
- Parses entry `.md` for wikilinks, writes `links.conf` (target path, label, containing card facet if any) into entry folder.
- **Lazy auto-refresh**: `render`/`export` compare `.md` mtime vs `links.conf`, recompile when stale — `compile` never required, just forces/warms index (`--all` for whole project, e.g. before session).
- Links to non-existent entries recorded, reported as warnings.

### `export`
- Same pipeline as `render`, output PDF next to entry (or project-level `exports/` directory for bulk) instead of browser. Per §4, PDFs never expand links — plain text labels only.
- **Selective link bundling**: `--with-links` reads entry `links.conf`, shows interactive checklist of linked entries to include. Non-interactive: `--with mary,gilded-goose` (comma-separated entry paths) or `--with-links all`.
- Bundled output = **one combined PDF**: main entry first, each selected linked entry as following section. `--view`/`--card` flags apply to every included entry (player export strips secrets from linked entries too).
- Bulk export (`quillit export` no path) applies view flags to every entry; `--with-links` invalid there.

### Errors (all commands)
- No project resolvable (not inside one, no `current_project`): clear error naming fixes (`cd` into project or `quillit connect <project_name>`), except `init`, `connect`, `config`, `version`, `--help`.
- `current_project` points at dead project: error suggesting `quillit connect` with project list from `$QUILLIT_HOME`.
- `$QUILLIT_HOME` unset/missing for commands needing global config: error pointing at `quillit init` to bootstrap home.
- Bad entry path: error stating path checked.

## 8. Implementation notes

- **Language: Go** (matches existing backend). Suggested: [cobra](https://github.com/spf13/cobra) for command tree; CommonMark library with container-directive support (or small pre-parser for `:::` blocks) for rendering.
- Ship single static binary; `version` reports install path.
- Keep markdown → filtered-view pipeline self-contained package so future web app (Vue + Go) reuses it server-side for same three views.
- **PDF export has the same static-binary constraint.** The `export` package (§2 golden rule 7, §7) must not shell out to a host toolchain (e.g. system `wkhtmltopdf`) — that would break the single-static-binary property and wouldn't survive being imported into a containerized service later. Prefer a pure-Go PDF library or an embedded/vendored renderer.

## 9. Out of scope

- Web app and all integrations (CLI is interim tool).
- Sharing beyond exported PDFs (no hosting, no links, no player accounts).
- Sync, collaboration, network features.
- Anything not in §6.
