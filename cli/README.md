# quillit

A CLI for running a D&D game from annotated markdown notes. One `.md` file
per entry — NPC, location, session, whatever — with `:::secret` and
`:::card <facet>` blocks marking DM-only content and flash-card extracts.
`render`/`export` derive the DM view, the player-safe view, and any flash
card from that single file. See [`docs/cli-spec.md`](../docs/cli-spec.md)
in the repo root for the full spec.

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

## Quick start

```sh
quillit init curse-of-strahd     # bootstrap $QUILLIT_HOME + create a project
quillit connect curse-of-strahd  # (init already connects it; this is for later)
quillit create tom -A characters/npcs   # create an entry, file it immediately
quillit edit characters/npcs/tom        # write it in $EDITOR
quillit render characters/npcs/tom      # open the DM view in your browser
```

See `quillit --help` and `quillit <command> --help` for the full command
reference, or `docs/cli-spec.md` §6 for the complete spec.
