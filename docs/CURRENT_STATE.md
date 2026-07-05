# Current state (2026-07-05)

Where the self-hosted LAN setup actually stands right now, on the box it's actually
running on (`pop-os`). This is a status snapshot, not a runbook — for step-by-step
instructions see [SERVER_SETUP.md](SERVER_SETUP.md) and the [README](../README.md).

## Architecture at a glance

Four core services, always on, via `docker-compose.yml`:

| Service | Port | Notes |
|---|---|---|
| `auth` | 3002 (not published) | Issues/validates JWTs, seeds the first admin user. |
| `svc` | 3000 (not published) | Main API. Talks to `auth` and `minio` by Docker service name. |
| `minio` | 9000/9001, bound `127.0.0.1` only | Object storage. Console reachable only via SSH tunnel. |
| `ui` | 8080 (or fronted by Caddy) | nginx serving the built Vue app, proxies `/api/` to `svc`. |

Two optional overlays, combined via the generated `compose.sh` wrapper:

- `docker-compose.caddy.yml` — HTTPS via Caddy. **Active on this machine.**
- `docker-compose.logging.yml` — Loki + Promtail + Grafana log aggregation. **Active on this machine.**

`compose.sh` on `pop-os` currently reads:
```sh
docker compose -f docker-compose.yml -f docker-compose.caddy.yml -f docker-compose.logging.yml "$@"
```
Use `./compose.sh` for everything (`build`, `up -d`, `logs`, `exec`) instead of raw `docker compose`.

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

## Current machine state (`pop-os`)

- Reachable on the LAN via a stable IP and an mDNS alias (avahi, installed by
  `setup.sh`) — both values live only in this box's local `.env` (`CADDY_HOST`,
  `CORS_ORIGIN`), never in a tracked file.
- `.env`: `COOKIE_SECURE=true` — already configured for the LAN HTTPS path, not the
  plain-HTTP or public-domain path.
- `Caddyfile` (tracked, generic — works on any machine without editing):
  ```
  # Self-signed LAN HTTPS (tls internal). Set CADDY_HOST in .env — see Caddyfile.internal.example.
  {$CADDY_HOST} {
      tls internal
      reverse_proxy ui:80
  }
  ```
- UFW: only 22 (SSH), 80/tcp, 443/tcp, 5353/udp (mDNS) open. MinIO (9001) and Grafana
  (3001) are loopback-only — not reachable from the LAN at all, only via SSH tunnel.

### Why the Caddyfile is env-var driven

`.github/workflows/deploy.yml`'s `deploy` job runs `git fetch --prune origin && git
reset --hard origin/main` on every deploy — anything hand-edited into a tracked file
gets silently reverted on the next deploy. Caddy resolves `{$VAR}` placeholders from
the container's environment before parsing its config, so `Caddyfile` can stay generic
and committed while the actual LAN address lives only in this box's gitignored `.env`
(`CADDY_HOST`), passed through via `docker-compose.caddy.yml`'s `environment:` key. No
per-machine edit to `Caddyfile` is needed, and no address ever ends up in git history.

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

`.github/workflows/deploy.yml` ("Deploy to home server"):

- Triggers on push to `main`, or manual dispatch.
- `preflight` (GitHub-hosted) — fails fast if the self-hosted runner (label `quillit`)
  isn't online, so a deploy never hangs waiting on a powered-off home server.
- `test` (GitHub-hosted) — `go test ./...` for `svc` and `auth`.
- `deploy` (runs **on** `pop-os` itself, via the self-hosted runner) — hard-resets the
  checkout to `origin/main`, rebuilds/restarts via `./compose.sh`, smoke-tests `ui`,
  `svc /healthz`, `auth /healthz`, and rolls back to the previous commit automatically
  if any of that fails.
- No inbound port is opened for CI — the runner polls GitHub outbound only; UFW stays
  at 22/80/443.

## Open items — not yet done

- **Confirm the self-hosted runner is actually registered and online on `pop-os`.**
  This wasn't verified as part of this round of changes — if it's not registered (or
  the `RUNNER_STATUS_TOKEN` secret isn't set), `preflight` will fail every run. Check
  via `sudo ./svc.sh status` in `~/actions-runner` on the server, or GitHub's
  **Settings → Actions → Runners** page. Full setup steps: SERVER_SETUP.md §8–9.
- **Remote access beyond the LAN** (Tailscale, dynamic DNS) is mentioned in
  SERVER_SETUP.md as "covered in prior planning notes" but not implemented — out of
  scope for now since everything is LAN-only until a domain is purchased.
- **`CORS_ORIGIN` is a single exact-match value** — `svc` doesn't support multiple
  allowed origins. If a client ever needs to reach the API via an origin other than
  `https://pop-os.local` (e.g. the raw IP, or a future domain), this needs to change to
  a small allowlist rather than one hardcoded string.
