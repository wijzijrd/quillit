# quillit

A CLI for running a D&D game from annotated markdown notes. One `.md` file
per entry — NPC, location, session, whatever — with `:::secret` and
`:::card <facet>` blocks marking DM-only content and flash-card extracts.
`render`/`export` derive the DM view, the player-safe view, and any flash
card from that single file. See [`docs/cli-spec.md`](../docs/cli-spec.md)
in the repo root for the full spec.

## Contents

- [Download](#download)
- [Build from source](#build-from-source)
- [Concepts](#concepts)
- [Quick start](#quick-start)
- [Command reference](#command-reference)
- [Configuration](#configuration)
- [Troubleshooting](#troubleshooting)

## Download

Grab the latest release for your platform from the
[Releases page](https://github.com/wijzijrd/quillit/releases/latest).
Each release includes a `.tar.gz` (macOS/Linux) or `.zip` (Windows) per
platform, plus a `checksums.txt` to verify the download:

| Platform | Asset |
|---|---|
| macOS (Apple Silicon) | `quillit-<version>-darwin-arm64.tar.gz` |
| macOS (Intel) | `quillit-<version>-darwin-amd64.tar.gz` |
| Linux (x86_64) | `quillit-<version>-linux-amd64.tar.gz` |
| Linux (ARM64) | `quillit-<version>-linux-arm64.tar.gz` |
| Windows (x86_64) | `quillit-<version>-windows-amd64.zip` |

Extract the archive and put the `quillit` (or `quillit.exe`) binary
somewhere on your `PATH`, e.g.:

```sh
tar -xzf quillit-*-darwin-arm64.tar.gz
sudo mv quillit-*/quillit /usr/local/bin/
quillit version
```

**macOS note:** these binaries aren't code-signed or notarized yet, so
Gatekeeper will block the first run with a "quillit Not Opened" popup. See
[Troubleshooting](#macos-wont-open-the-binary-quillit-not-opened) below.

## Build from source

```sh
git clone https://github.com/wijzijrd/quillit.git
cd quillit/cli
go build -o quillit .
```

Requires Go 1.26+. `cli/go.mod` references the shared content-engine
package (`pkg/contentengine/`, one directory up) via a `replace`
directive, so a plain `go build` from a full repo checkout works without
any extra setup — no workspace mode, no separate `go get`.

If your shell has `GOFLAGS=-mod=mod` set globally and this repo's
`go.work` is active, Go workspace mode will refuse to build (`-mod may
only be set to readonly or vendor when in workspace mode`). Override it
for the build: `GOFLAGS=-mod=readonly go build -o quillit .`

To run the test suite: `go test ./...` from `cli/` (same `GOFLAGS`
override applies if needed).

## Concepts

- **Entry** — a single note (NPC, location, session, etc.), one folder per
  entry, containing exactly one `<entry_name>.md` as its source of truth
  plus render scaffolding (`.html`/`.css`/`.js`) and a generated
  `links.conf`. An entry belongs to exactly one project.
- **Project** — a workspace: a directory tree of entries plus a
  `quillit.yaml` at its root. You can have several projects (e.g. one per
  campaign) inside a single quillit home.
- **Quillit home** — the directory (`$QUILLIT_HOME`) holding global config
  and every project. Created by `quillit init` the first time you run it.
- **View** — how an entry renders: `dm` (everything), `player` (secrets
  stripped), or `card <facet>` (only that facet's flash-card blocks).
- **Facet** — a flash-card category, e.g. `motivation`, `description`,
  `history`. Facets are declared globally (apply to every project) or
  per-project (`extra_facets`); a facet only names which card blocks
  belong together — it isn't a filing mechanism.
- **Directory** (inside a project, e.g. `characters/npcs/`) — the actual
  filing/organizing mechanism for entries. `quillit assign` moves an
  entry into one.
- **Link** — a `[[path/to/entry]]` wikilink from one entry's `.md` to
  another, resolved relative to the project root.

### Annotation format

Entry `.md` files use fenced directive blocks:

- `:::secret … :::` — DM-only content, stripped entirely from `player`
  view.
- `:::card <facet> … :::` — a flash-card block for the named facet;
  included in normal DM view, and the only thing that appears when you
  render/export with `--card <facet>`.

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

| View | Output |
|---|---|
| `dm` (default) | Everything: secret + all card blocks. |
| `player` | Everything except the `:::secret` block. |
| `card motivation` | Only "Wants to buy back his family farm." (plus title/frontmatter header). |

Facet names must be kebab-case (lowercase letters, digits, hyphens).
Rendering with an unrecognized facet is an error, not a silent no-op —
it's usually a typo.

**Links, one layer deep.** Inside a rendered card block, a link to another
entry pulls in that entry's *same-facet* card, inlined below the main one
— but only one layer: links inside that inlined card don't expand further,
they render as clickable "clipboard-links" (click copies the `quillit
render ...` command for that entry to your clipboard) so you can look it
up manually. Links outside card blocks, and anything in `player`/PDF
output, never expand — they show as plain text.

Frontmatter `tags` are accepted and stored but currently inert — no
command filters or renders by them yet.

## Quick start

```sh
quillit init curse-of-strahd            # bootstrap $QUILLIT_HOME + create a project
quillit create tom -A characters/npcs   # create an entry, file it immediately
quillit edit characters/npcs/tom        # write it in $EDITOR/$VISUAL
quillit render characters/npcs/tom      # open the DM view in your browser
quillit render characters/npcs/tom --view player   # player-safe view
quillit render characters/npcs/tom --card motivation --quick-view  # one flash card
quillit export characters/npcs/tom --view player    # player-safe PDF handout
```

Later, from a fresh shell or a different directory:

```sh
quillit connect curse-of-strahd   # or just `cd` into the project — see resolution order below
```

**Project resolution order** (used by every command that needs a
project): (1) walk up from the current directory looking for the nearest
`quillit.yaml`; (2) otherwise fall back to the connected project
(`quillit connect`, persisted in home config — survives closing your
terminal). If neither resolves, commands other than `init`, `connect`,
`config`, `version`, and `--help` fail with an error pointing at `quillit
connect <project_name>`.

## Command reference

Every command also has `--help`, e.g. `quillit render --help`, which is
always the source of truth for flags.

### `init <project_name>`
Bootstraps `$QUILLIT_HOME` if it isn't set up yet (prompts for a home
location, defaulting to `~/quillit`), then creates the project and
connects it. See [Configuration](#configuration) for how home is located
after this.
```sh
quillit init curse-of-strahd
```

### `connect [project_name]`
Sets the connected project, or with no argument prints which project is
currently connected (`none` if unset). Being inside a project directory
always beats the connected pointer — `connect` is the fallback for
working from elsewhere (e.g. a new terminal, or your home directory).
```sh
quillit connect curse-of-strahd
quillit connect              # prints the current one
```

### `create <entry_name> [directory]`
Creates an entry — a folder with four ready-to-use templated files
(`<entry_name>.md/.html/.css/.js`) — at the project root.
- `-A`/`--assign [<directory>]` — also file it into `directory`
  immediately; omit the directory and you'll be prompted for one.
- `[directory]` is only valid together with `-A`.
```sh
quillit create tom -A characters/npcs   # create + file in one step
quillit create tom -A                   # create + prompt for directory
quillit create tom                      # leave at project root, `assign` later
```

### `assign <entry> <directory>`
Moves an existing entry folder from the project root into an
organizational directory (created if it doesn't exist).
```sh
quillit assign tom characters/npcs
```

### `edit <path_to_entry>`
Opens the entry's folder in `$VISUAL`, falling back to `$EDITOR`, falling
back to the OS default opener (`open` on macOS, `xdg-open` on Linux,
`start` on Windows).
```sh
quillit edit characters/npcs/tom
```

### `render <path_to_entry>`
Builds the entry's `.md` into HTML (via its own scaffolding) and opens it
in your default browser.
- `--view dm|player` — default `dm`.
- `--card <facet>` — render only that facet's flash card (mutually
  exclusive with `--view`; implies DM audience, so secrets inside a
  matching card still show — card view is a DM tool).
- `-Q`/`--quick-view` — frontmatter summary + first section only, for any
  view.
```sh
quillit render characters/npcs/tom
quillit render characters/npcs/tom --view player
quillit render characters/npcs/tom --card motivation -Q
```

### `compile <path_to_entry>` / `compile --all`
Scans an entry's `.md` for wikilinks and writes its `links.conf` index.
`render`/`export` auto-recompile a stale index on their own, so this is
rarely required — mainly useful to warm the whole project's cache before
a session (`--all`). Links to entries that don't exist are recorded and
reported as warnings, not treated as errors.
```sh
quillit compile characters/npcs/tom
quillit compile --all
```

### `export [path_to_entry]`
Same rendering pipeline as `render`, but writes a PDF instead of opening
a browser. PDFs never expand links or show clipboard-link affordances —
just the link's plain text label, regardless of `--view`.
- No path → exports every entry in the project into `<project>/exports/`.
- `--view dm|player`, `--card <facet>` — same semantics as `render`.
- `--with-links` — interactively choose, from the entry's resolved
  outgoing links, which to bundle into one combined PDF (main entry
  first, each chosen link as a following section).
- `--with-links-all` — bundle every resolved outgoing link, no prompt.
- `--with <entries>` — bundle an explicit comma-separated list of entry
  paths.
- Bundling flags (`--with-links`/`--with-links-all`/`--with`) are only
  valid with a single entry path — invalid on the bulk (no-path) form.
```sh
quillit export characters/npcs/tom --view player
quillit export characters/npcs/tom --with-links-all
quillit export characters/npcs/tom --with characters/npcs/mary,locations/gilded-goose
quillit export   # every entry, project-wide
```

### `config`
With no subcommand, prints the global config, the resolved current
project's config, and the effective facet vocabulary (global ∪ project).
```sh
quillit config
```

`config add`/`config rm` manage the facet vocabulary:
- `--facet <name>` — required, kebab-case.
- `--scope global|<project_name>` — defaults to `global`; a project name
  targets that project's `extra_facets` instead.
- Adding a facet already in the effective vocabulary at that scope is a
  no-op, not an error. Removing a facet doesn't touch any `.md` files —
  entries still using it will fail loud at the next render, and the
  command prints a reminder to that effect.
```sh
quillit config add --facet stat-block --scope curse-of-strahd
quillit config rm --facet stat-block --scope curse-of-strahd
```

### `version`
Prints the quillit version and the path of the running binary.
```sh
quillit version
```

## Configuration

### `$QUILLIT_HOME`

All projects live under one home directory:

```
$QUILLIT_HOME/
├── config.yaml           # global config: facet vocabulary + connected project
├── curse-of-strahd/      # a project
└── one-shots/            # another project
```

`quillit init` bootstraps this on first run (prompting for a location if
`$QUILLIT_HOME` isn't set, defaulting to `~/quillit`) and, since v0.0.2,
also records the chosen path in `~/.quillit-home`. Every other command
resolves the home directory in this order:

1. `$QUILLIT_HOME`, if set — always wins. Useful for pointing at a second
   home, or for scripts/CI.
2. Otherwise, the path recorded in `~/.quillit-home` by the last `init`.

In practice this means: run `init` once, and every later command — in any
shell, any session — finds the home automatically, with no need to
`export QUILLIT_HOME=...` yourself. `init` still prints that export line
after bootstrapping, purely as the explicit-override option, not because
you need it.

### Project config (`quillit.yaml`)

Each project has, at minimum:

```yaml
name: curse-of-strahd
extra_facets: []          # project-only facets, e.g. [stat-block, loot]
```

Its presence at a directory marks that directory as a project root, which
is what makes cwd-based resolution (walking up from wherever you are)
work.

### Editor / browser

`edit` opens `$VISUAL`, then `$EDITOR`, then the OS default file opener.
`render` always opens your default *browser* regardless of those — it's
opening an `.html` file, not asking for a text editor.

## Troubleshooting

### macOS won't open the binary ("quillit" Not Opened)

Release binaries aren't code-signed or notarized, so on first run macOS
Gatekeeper blocks it: *"Apple could not verify 'quillit' is free of
malware..."*. This is expected for now, not a corrupted download. Clear
the quarantine flag macOS set on the extracted binary:

```sh
xattr -l /usr/local/bin/quillit          # confirm com.apple.quarantine is present
sudo xattr -d com.apple.quarantine /usr/local/bin/quillit
quillit version                          # should now run
```

This is per-binary — a future downloaded release will need the same step
repeated (or run it once on the extracted folder before installing:
`xattr -cr quillit-*/ && sudo mv quillit-*/quillit /usr/local/bin/`).

### `$QUILLIT_HOME is not set up yet` right after running `init`

If you're on a build older than v0.0.2, `init` only *printed* the export
line — it didn't persist anything, so any new shell (or even the same
shell without running that export) would hit this. Either upgrade
(`git pull && go build -o quillit .`, or grab a newer release), or export
manually in the meantime:

```sh
export QUILLIT_HOME="$HOME/quillit"   # or wherever you chose during init
```

On v0.0.2+, running `quillit init` once is enough — the resolved home is
recorded in `~/.quillit-home` and every later command finds it there
automatically, without the env var.

See `quillit --help` and `quillit <command> --help` for the full command
reference, or [`docs/cli-spec.md`](../docs/cli-spec.md) for the complete
spec.
