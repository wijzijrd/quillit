# Current state (2026-07-05)

Where the self-hosted LAN setup actually stands right now, on the box it's actually
running on (`pop-os`). This is a status snapshot, not a runbook — for step-by-step
instructions see [SERVER_SETUP.md](SERVER_SETUP.md) and the [README](../../README.md).

## Architecture at a glance

Four core services, always on, via `infra/docker-compose.yml`:

| Service | Port | Notes |
|---|---|---|
| `auth` | 3002 (not published) | Issues/validates JWTs, seeds the first admin user. |
| `svc` | 3000 (not published) | Main API. Talks to `auth` and `minio` by Docker service name. |
| `minio` | 9000/9001, bound `127.0.0.1` only | Object storage. Console reachable only via SSH tunnel. |
| `ui` | 8080 (or fronted by Caddy) | nginx serving the built Vue app, proxies `/api/` to `svc`. |

Two optional overlays, combined via the generated `compose.sh` wrapper:

- `infra/docker-compose.caddy.yml` — HTTPS via Caddy. **Active on this machine.**
- `infra/docker-compose.logging.yml` — Loki + Promtail + Grafana log aggregation. **Active on this machine.**

As of the `infra/`/`ops/` restructure (2026-08-08), `compose.sh` should read:
```sh
docker compose -f infra/docker-compose.yml -f infra/docker-compose.caddy.yml -f infra/docker-compose.logging.yml --project-directory . "$@"
```
Use `./compose.sh` for everything (`build`, `up -d`, `logs`, `exec`) instead of raw `docker compose`.

> **Update (2026-08-16): `compose.sh` is gone from the deploy path entirely.** The
> ephemeral-checkout model (issue #86, cut over in #87) replaced it with a per-run
> `mktemp` checkout plus inline `docker compose -f infra/docker-compose.yml
> $QUILLIT_COMPOSE_OVERLAYS -p quillit --project-directory . --env-file
> $QUILLIT_ENV_FILE ...` invocations — `$QUILLIT_COMPOSE_OVERLAYS` and
> `$QUILLIT_ENV_FILE` are GitHub Actions repo variables (see SERVER_SETUP.md), not a
> gitignored generated script. There is nothing left on `pop-os` for a human to hand-fix
> after a restructure lands — a fresh checkout picks up path changes automatically. A
> `compose.sh` may still exist as a leftover from before this migration; it is no longer
> read by CI and can be ignored or deleted.

## What's been done (chronological)

The first four items below already landed on `main` via PR #2. Only the last one is
still open, on branch `fix/deploy-hardening-and-logging` (PR #3):

1. **`70578f1` Readme and deploy updates** — added `Caddyfile.internal.example` (the
   template for LAN-only self-signed HTTPS), expanded README/SERVER_SETUP.md.
2. **`6f6771c` fix(setup): support Ubuntu/Debian derivatives and fix docker-group race** —
   `setup.sh` now resolves the right Docker apt repo on derivatives like Pop!_OS (via
   `ID_LIKE` fallback), and no longer fails on a fresh install when the current shell
   hasn't picked up the newly-added `docker` group yet (falls back to `sg docker`).
3. **`d9f0aa8` fix(ui): pin @tiptap/extension-image** — fixed an npm ERESOLVE conflict
   (one `@tiptap/*` package was on a caret range while its siblings were pinned).
4. **`29f09e1` feat(logging): add opt-in Loki/Promtail/Grafana log aggregation** — new
   overlay, Promtail auto-discovers containers via the Docker socket, Grafana bound to
   `127.0.0.1` with Loki pre-provisioned as a datasource, retention via Loki's compactor.
5. **`4ef1034` fix(db): repair svc category_default_tags FK corruption, harden migrations** —
   fixed a dangling foreign key left over from an old migration (`toV2` renamed
   `categories` → `categories_v1` and SQLite silently repointed the FK before the old
   table was dropped), added a `toV4` repair migration, added a `checkForeignKeys`
   post-migration integrity check to both `svc` and `auth`, added regression tests, and
   added a `go test ./...` CI job to `.github/workflows/deploy.yml`.
6. **This commit** — made `Caddyfile`'s LAN address configurable via a `CADDY_HOST` env
   var instead of hardcoding this machine's IP/hostname directly into the tracked file,
   and made `backup.sh` executable (mode-only fix; `setup.sh` normally does this itself
   but this machine's copy predates that).
7. **`infra`/`ops` restructure (2026-08-08)** — moved the compose files + `Caddyfile`
   into `infra/`, and `loki-config.yml`/`promtail-config.yml`/`grafana/` into `ops/`, to
   declutter the repo root. Requires the manual `compose.sh` fix on `pop-os` noted
   above.

## Current machine state (`pop-os`)

- Reachable on the LAN via a stable IP and an mDNS alias (avahi, installed by
  `setup.sh`) — both values live only in this box's local `.env` (`CADDY_HOST`,
  `CORS_ORIGIN`), never in a tracked file.
- `.env`: `COOKIE_SECURE=true` — already configured for the LAN HTTPS path, not the
  plain-HTTP or public-domain path.
- `infra/Caddyfile` (tracked, generic — works on any machine without editing):
  ```
  # Self-signed LAN HTTPS (tls internal). Set CADDY_HOST in .env — see Caddyfile.internal.example.
  {$CADDY_HOST} {
      tls internal
      reverse_proxy ui:80
  }
  ```
- UFW: only 22 (SSH), 80/tcp, 443/tcp, 5353/udp (mDNS) open. MinIO (9001) and Grafana
  (3001) are loopback-only — not reachable from the LAN at all, only via SSH tunnel.
- SSH itself is served by **Tailscale SSH**, not `openssh-server` (which was never
  installed — port 22 answers nothing on the LAN despite the UFW rule). Connect from
  any device on the tailnet with `ssh <user>@quillit` (MagicDNS name set via
  `tailscale up --ssh --hostname=quillit`); tunnels to Grafana/MinIO work through it
  from any network, not just the LAN. See SERVER_SETUP.md §6 "Remote access from
  anywhere".

### Why the Caddyfile is env-var driven

Every category pipeline's `deploy` job (`app-pipeline.yml`, `infra-pipeline.yml`,
`observability-pipeline.yml`) runs `git fetch --prune origin && git reset --hard
origin/main` on every deploy — anything hand-edited into a tracked file gets silently
reverted on the next deploy. Caddy resolves `{$VAR}` placeholders from
the container's environment before parsing its config, so `Caddyfile` can stay generic
and committed while the actual LAN address lives only in this box's gitignored `.env`
(`CADDY_HOST`), passed through via `infra/docker-compose.caddy.yml`'s `environment:` key.
No per-machine edit to `Caddyfile` is needed, and no address ever ends up in git history.

## Accessing the app from a second PC on the same LAN

The second PC is a **browser client only** — it doesn't need to clone the repo, build
anything, or run its own stack. `pop-os` is already running the full app.

1. Browse to `https://pop-os.local`. If mDNS doesn't resolve (`.local` names are
   unreliable on Windows and some Android versions — see SERVER_SETUP.md §4), use the
   box's LAN IP instead — both are set in this machine's `.env` (`CADDY_HOST`).
2. Trust Caddy's self-signed root CA once, to avoid a browser TLS warning:
   ```sh
   docker compose cp caddy:/data/caddy/pki/authorities/local/root.crt .
   ```
   run on `pop-os`, then copy `root.crt` to the second PC and install it as a trusted
   root certificate (OS-specific steps are in the README's "HTTPS on your local
   network" section).
3. That's it — no `.env`, no ports to open on the second PC, nothing to install.

## Git / CI pipeline

*(Updated 2026-08-16 — the section below describes the ephemeral-checkout model that
replaced the persistent-clone-plus-`git reset --hard` model referenced elsewhere in
this doc; treat any other mention of `compose.sh` or a hard-reset checkout in this file
as historical.)*

`.github/workflows/ci.yml` ("CI") orchestrates deploys — it's a thin wrapper around
one reusable `workflow_call` pipeline per category:

- Triggers on push to `main`, or manual dispatch (manual dispatch runs every
  category regardless of what changed).
- "Health gate — is the server online?" (GitHub-hosted) — queries the GitHub API for
  the self-hosted runner's (label `quillit`) status and fails fast if it isn't
  `online`, so a deploy never hangs waiting on a powered-off/disconnected home server.
  Every category pipeline below depends on it.
- "Detect changed components" (GitHub-hosted) — a single `dorny/paths-filter` step
  detects which of `auth`/`svc`/`content`/`ui`/`messaging`/`infra`/`observability`/
  `operations` changed; `ci.yml` only calls the pipelines for components that actually
  changed (or all of them, on manual dispatch).
- `app-pipeline.yml` (called once per service: auth, svc, content, messaging, ui) —
  `test` (GitHub-hosted: `go test ./...` for the Go services, `vue-tsc --noEmit` +
  `npm run build` for `ui`) then `deploy` (runs **on** `pop-os` itself, via the
  self-hosted runner) — checks out that commit into a fresh `mktemp` directory (not a
  persistent clone; nothing to hard-reset or drift), rebuilds/restarts that one service
  via `docker compose -f infra/docker-compose.yml $QUILLIT_COMPOSE_OVERLAYS -p quillit
  --project-directory . --env-file $QUILLIT_ENV_FILE up -d --no-deps <service>`
  (project name pinned to `quillit` so it always targets the same named
  volumes/networks regardless of the ephemeral checkout's path), smoke-tests it, rolls
  back to the previous image automatically if any of that fails, then `rm -rf`s the
  ephemeral checkout as its last step (`if: always()`, runs even on failure). A failing
  test or deploy for one service never touches the others.
- `infra-pipeline.yml` / `observability-pipeline.yml` — same ephemeral-checkout +
  pinned-project-name pattern; validate the relevant `docker compose config`, then (on
  the self-hosted runner) restart Caddy or Loki/Promtail/Grafana respectively. Both are
  pulled images, not built from this repo, so there's no image rollback step for them.
- `operations-pipeline.yml` — lint-only (`shellcheck`) on `operations/*.sh`, no
  deploy stage.
- "Final whole-stack smoke test" (runs **on** `pop-os`) — gated on the health gate plus
  every app pipeline that ran succeeding.
- No inbound port is opened for CI — the runner polls GitHub outbound only; UFW stays
  at 22/80/443.
- `$QUILLIT_COMPOSE_OVERLAYS` and `$QUILLIT_ENV_FILE` are GitHub Actions repo
  variables (`gh variable list`), not values baked into any file on `pop-os` — see
  SERVER_SETUP.md. `$QUILLIT_ENV_FILE` points at `~/quillit-secrets/.env` (mode `600`),
  a stable secrets file outside any ephemeral checkout.

## Open items — not yet done

- **Self-hosted runner is confirmed registered and online** — a manual
  `workflow_dispatch` run passed `preflight` and reached `deploy`. That run did surface
  a real bug in the `smoke test` step (it checked `ui` via a host-published port 8080
  that doesn't exist once the Caddy overlay drops it) — fixed by checking `ui`
  internally via `compose.sh exec`, the same pattern already used for `svc`/`auth`.
  The deploy's own rollback step worked correctly when the old smoke test failed,
  restoring the box to its prior commit with no manual intervention needed.
- **Remote access beyond the LAN** — done for SSH/tunnels via Tailscale SSH (see
  above). The app itself (`https://quillit.local`) is still LAN-only; a public domain
  or `tailscale serve` for the app remains out of scope until needed.
- **`CORS_ORIGIN` is a single exact-match value** — `svc` doesn't support multiple
  allowed origins. If a client ever needs to reach the API via an origin other than
  `https://pop-os.local` (e.g. the raw IP, or a future domain), this needs to change to
  a small allowlist rather than one hardcoded string.
