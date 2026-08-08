# Server setup guide

Zero-to-production runbook for running Quillit on a self-hosted Linux box, including
the automated GitHub Actions deploy pipeline. For a quick summary see the
[README](../README.md#option-2--linux-server-recommended); this doc goes step by step.

## Contents

1. [Prerequisites](#1-prerequisites)
2. [What gets built vs. pulled](#2-what-gets-built-vs-pulled)
3. [Clone + run `setup.sh`](#3-clone--run-setupsh)
4. [Environment variable reference](#4-environment-variable-reference)
5. [HTTPS with Caddy (optional)](#5-https-with-caddy-optional)
6. [Log aggregation with Loki/Grafana (optional)](#6-log-aggregation-with-lokigrafana-optional)
7. [Always-on / reboot survival](#7-always-on--reboot-survival)
8. [GitHub Actions self-hosted runner](#8-github-actions-self-hosted-runner)
9. [Pipeline secrets & variables](#9-pipeline-secrets--variables)
10. [Verify end-to-end](#10-verify-end-to-end)
11. [Day 2 ops](#11-day-2-ops)

---

## 1. Prerequisites

- Ubuntu 22.04 LTS or Debian 12 (what `setup.sh` is written/tested for; other distros
  need manual package-manager substitutions).
- A non-root user with `sudo`.
- Outbound internet access (Docker Hub, GitHub, and — for HTTPS — Let's Encrypt).
- Optional, for HTTPS: a domain with an **A record** pointing at the server's public
  IP, and ports 80/443 reachable from the internet.

## 2. What gets built vs. pulled

Nothing is pulled from a private registry — the app images are always built from this
repo, directly on the server.

| Service | Image | Source |
|---|---|---|
| `ui` | `quillit-ui` | **Built locally** — `ui/Dockerfile`: `node:22-alpine` build stage → `nginx:alpine` runtime, serving the compiled `dist/` via `ui/nginx.conf`. |
| `svc` | `quillit-svc` | **Built locally** — `svc/Dockerfile`: `golang:1.26-alpine` build stage (`CGO_ENABLED=0 GOOS=linux go build`) → `alpine:3.21` runtime. |
| `auth` | `quillit-auth` | **Built locally** — `auth/Dockerfile`: same multi-stage pattern as `svc`. |
| `minio` | `minio/minio:latest` | **Pulled** from Docker Hub. |
| `caddy` | `caddy:alpine` | **Pulled** from Docker Hub — only added when the HTTPS overlay (`infra/docker-compose.caddy.yml`) is used. |

Because `svc`/`auth` use multi-stage Dockerfiles, `docker compose build` compiles the
Go binaries **inside** the build — no Go toolchain needs to be installed on the host,
and there's no separate `make build-linux` step to remember.

## 3. Clone + run `setup.sh`

```sh
git clone https://github.com/wijzijrd/quillit.git
cd quillit
chmod +x setup.sh && ./setup.sh
```

What it does, in order:

1. **OS check** — warns (doesn't block) if not Ubuntu/Debian.
2. **Docker install** — if `docker` + `docker compose` aren't already present:
   adds Docker's official apt repo (`/etc/apt/keyrings/docker.gpg`,
   `/etc/apt/sources.list.d/docker.list`), installs
   `docker-ce docker-ce-cli containerd.io docker-buildx-plugin
   docker-compose-plugin`, and adds your user to the `docker` group (a new shell or
   `newgrp docker` is needed for that to take effect).
3. **Firewall (UFW)** — if `ufw` is present:
   ```sh
   sudo ufw default deny incoming
   sudo ufw default allow outgoing
   sudo ufw allow ssh       # 22
   sudo ufw allow 80/tcp
   sudo ufw allow 443/tcp
   sudo ufw allow 5353/udp  # mDNS
   sudo ufw --force enable
   ```
   Only **22, 80, 443, and 5353/udp (mDNS)** are ever opened. MinIO (9000/9001) gets no
   firewall rule because `infra/docker-compose.yml` binds it to `127.0.0.1` only — it's
   unreachable from the network regardless of UFW.
4. **mDNS (avahi)** — installs `avahi-daemon` if missing and enables it via
   `systemctl enable --now`. This advertises the server at `$(hostname).local` on the
   LAN with zero router configuration, so you have a stable name to hit even if the
   box's DHCP-assigned IP changes. Works out of the box on macOS/iOS and most Linux
   desktops; support on Windows and some Android versions is inconsistent — pair it
   with a router DHCP reservation (see §5) for those clients.
5. **Interactive prompts**:
   - Server IP or domain (defaults to the detected LAN/public IP).
   - Use HTTPS with Caddy? (y/N) — if yes, sets `CORS_ORIGIN=https://<host>`,
     `COOKIE_SECURE=true`, rewrites `infra/Caddyfile`'s domain via `sed`, and adds
     `-f infra/docker-compose.caddy.yml` to the compose flags used for the rest of setup
     (and baked into `compose.sh`, see below).
   - Admin email (default `admin@quillit.local`).
   - Admin password (blank = auto-generated 20-char alnum string).
6. **Secrets auto-generated**: `JWT_SECRET=$(openssl rand -hex 32)`,
   `MINIO_PASSWORD=$(openssl rand -hex 20)`.
7. **Writes `.env`** (mode `600`) with `JWT_SECRET`, `SEED_ADMIN_EMAIL`,
   `SEED_ADMIN_PASSWORD`, `MINIO_USER=quillit`, `MINIO_PASSWORD`, `CORS_ORIGIN`,
   `COOKIE_SECURE`. If `.env` already exists it's backed up to `.env.bak` (mode `600`)
   first.
8. **Builds and starts**: `docker compose $COMPOSE_FLAGS build` then
   `docker compose $COMPOSE_FLAGS up -d`.
9. **Waits for readiness** — polls `svc`'s `/healthz` from inside the compose network
   (its port isn't published to the host) for up to 60s, via
   `docker compose exec -T svc wget -q -O /dev/null http://localhost:3000/healthz`.
10. **Generates `compose.sh`** — a wrapper in the repo root that always includes the
    right `-f` flags for your install (plain, or `+ -f infra/docker-compose.caddy.yml`
    if you chose HTTPS). Use this instead of raw `docker compose` from now on. Also
    `chmod +x`'s `backup.sh`.

At the end you'll see the app URL, admin credentials (also saved in `.env`), a LAN
mDNS alias (`$(hostname).local`), and the manage/backup/update commands. Without
HTTPS, the app is at **http://YOUR_SERVER_IP:8080**.

## 4. Environment variable reference

Only these seven are read from root `.env` (via `${VAR}` substitution in
`infra/docker-compose.yml`) — everything else the services need is hardcoded in the compose
file itself and **cannot** be overridden through `.env`.

| Variable | Used by | Notes |
|---|---|---|
| `JWT_SECRET` | `auth`, `svc` | Must be identical in both — `svc` validates tokens `auth` issues. |
| `SEED_ADMIN_EMAIL` | `auth` | Seeds the first admin account on launch. |
| `SEED_ADMIN_PASSWORD` | `auth` | Same. Change before exposing beyond localhost. |
| `MINIO_USER` | `minio` (`MINIO_ROOT_USER`), `svc` (`MINIO_ACCESS_KEY`) | `setup.sh` hardcodes `quillit`, not prompted. |
| `MINIO_PASSWORD` | `minio` (`MINIO_ROOT_PASSWORD`), `svc` (`MINIO_SECRET_KEY`) | Auto-generated by `setup.sh`. |
| `CORS_ORIGIN` | `svc` | The origin the browser uses to reach the app. |
| `COOKIE_SECURE` | `svc` | `true` when serving over HTTPS, else `false`. |

Hardcoded in `infra/docker-compose.yml` (not `.env`-configurable): `PORT` (3000/3002),
`DB_PATH` (`/data/quillit.db` / `/data/quillit-auth.db`), `AUTH_SERVICE_URL`
(`http://auth:3002`), `MINIO_ENDPOINT` (`minio:9000`), `MINIO_BUCKET` (`quillit`),
`MINIO_USE_SSL` (`false`).

> Per-service `svc/.env.example` / `auth/.env.example` / `ui/.env.example` are for
> **native/bare-metal dev only** (Option 3 in the README) — they use different values
> (e.g. `CORS_ORIGIN=http://localhost:5173` for the Vite dev server) and are unrelated
> to the Docker Compose deployment described here.

## 5. HTTPS with Caddy (optional)

Requires a domain's A record pointing at the server, with 80/443 reachable from the
internet (Caddy needs port 80 for the ACME HTTP challenge and uses 443 for TLS +
HTTP/3).

If you answered "y" to the HTTPS prompt in `setup.sh`, this is already done. To enable
it manually / after the fact:

1. Edit `infra/Caddyfile`:
   ```
   your.domain.com {
       reverse_proxy ui:80
   }
   ```
   `ui` here is the **Docker Compose service name**, resolved via Docker's embedded
   DNS on the compose network — it must match the service name in
   `infra/docker-compose.yml`, not the image name (`quillit-ui`).
2. Set in `.env`:
   ```
   CORS_ORIGIN=https://your.domain.com
   COOKIE_SECURE=true
   ```
3. Start with the overlay:
   ```sh
   docker compose -f infra/docker-compose.yml -f infra/docker-compose.caddy.yml --project-directory . up -d
   ```
   `infra/docker-compose.caddy.yml` uses the Compose `!reset []` merge directive to drop
   `ui`'s direct `8080:80` host-port publish (Caddy fronts it instead), and adds the
   `caddy` service publishing `80/tcp`, `443/tcp`, `443/udp`, with `infra/Caddyfile`
   mounted read-only and two named volumes (`caddy-data`, `caddy-config`) for its
   certificate store and config.

If you ran `setup.sh`, `compose.sh` already has the right `-f` flags baked in — just
use `./compose.sh up -d` going forward instead of the raw `docker compose` command
above.

**No public domain, LAN only:** the committed `infra/Caddyfile` already runs in this mode by
default — `tls internal` instead of ACME, and its address (`{$CADDY_HOST}`) resolved
from the `CADDY_HOST` env var rather than hardcoded, so nothing in the file itself
ever needs editing or committing. Set in `.env`:
```
CADDY_HOST=192.168.1.50, quillit.local
```
`infra/Caddyfile.internal.example` documents this same pattern as a reference / reset point.
See the README sections "Stable LAN address" and "HTTPS on your local network
(self-signed, no domain)" for the full walkthrough, including trusting Caddy's
self-signed root CA on client devices. Whichever address you put in `CADDY_HOST` (a
LAN IP or the `$(hostname).local` mDNS alias from step 4 above) should be stable — see
"Stable LAN address" for a router DHCP reservation, which avoids the address drifting
after a DHCP lease renewal.

## 6. Log aggregation with Loki/Grafana (optional)

Adds three containers via a second overlay: `loki` (log storage/index, filesystem-
backed), `promtail` (ships every running container's logs to Loki), and `grafana`
(LogQL query UI, bound to `127.0.0.1` — same access model as the MinIO console).

1. Set in `.env`:
   ```
   LOKI_RETENTION_DAYS=14
   GRAFANA_ADMIN_USER=admin
   GRAFANA_ADMIN_PASSWORD=a-real-password
   ```
2. Start with the overlay (combinable with the Caddy overlay):
   ```sh
   docker compose -f infra/docker-compose.yml -f infra/docker-compose.logging.yml --project-directory . up -d
   ```
3. If you used `setup.sh`, edit the generated `compose.sh` to add
   `-f infra/docker-compose.logging.yml` to its `docker compose ${COMPOSE_FLAGS} "$@"`
   line, so `./compose.sh` includes it on every future invocation.

`ops/promtail-config.yml` uses Docker service discovery (`docker_sd_configs`) against
the Docker socket — it needs no fixed list of container names and needs no access
to `/var/lib/docker/containers`: it pulls logs via the same Docker Engine API that
`docker logs` uses, over the socket. Each log stream is labeled `service=<compose
service name>` (`auth`, `svc`, `minio`, `ui`, `caddy` if that overlay is active,
plus `loki`/`promtail`/`grafana` themselves).

This overlay is **not** part of `setup.sh`'s interactive prompts — enable it
manually, the same way the LAN self-signed-HTTPS path (§5) is manual rather than
wired into the `setup.sh` HTTPS prompt.

**Retention:** Loki's compactor deletes chunks/index entries older than
`LOKI_RETENTION_DAYS` automatically, so unlike Docker's default `json-file` log
driver (which has no size cap in this repo's compose files today), storage stays
bounded.

**Access Grafana:**
```sh
ssh -L 3001:localhost:3001 user@your-server
```
then open `http://localhost:3001`, log in with `GRAFANA_ADMIN_USER` /
`GRAFANA_ADMIN_PASSWORD`. Loki is pre-provisioned as a datasource — use **Explore**
with LogQL, e.g. `{service="svc"} |= "error"`.

### Remote access from anywhere (Tailscale SSH, optional)

The SSH-tunnel access model above assumes you can reach port 22 — which only works
on the LAN, and only if `openssh-server` is actually installed (desktop distros like
Pop!_OS ship without it). [Tailscale SSH](https://tailscale.com/kb/1193/tailscale-ssh)
covers both: `tailscaled` answers SSH on the tailnet interface itself, so there's no
`openssh-server` to install, no keys to manage, and it works from any network — not
just the LAN. Nothing is exposed to the public internet and UFW needs no new rules.

On the server:

```sh
curl -fsSL https://tailscale.com/install.sh | sh
sudo tailscale up --ssh --hostname=quillit
# prints a login URL — sign in with your Tailscale account
```

On each client, install Tailscale (macOS: `brew install --cask tailscale-app`), sign
in with the **same account**, then the tunnels work exactly as documented above from
anywhere, using the MagicDNS name:

```sh
ssh -L 3001:localhost:3001 -L 9001:localhost:9001 user@quillit
```

The default tailnet policy allows SSH between your own devices in "check" mode
(periodic browser re-auth); adjust the `ssh` rule in the Tailscale admin console's
ACLs if you want `accept` instead.

## 7. Always-on / reboot survival

- `sudo systemctl enable docker` so the Docker daemon starts on boot (do this if you
  didn't use `setup.sh`'s package install, which enables it automatically).
- No compose changes needed — all four services already set `restart: unless-stopped`
  in `infra/docker-compose.yml`, so they come back up after a reboot or crash on their own.
- For real power-loss resilience: in BIOS/UEFI, enable **"Restore on AC power loss"**
  so the box powers back on automatically after an outage; a small UPS lets it shut
  down cleanly instead of losing power mid-write.
- Remote access is covered by Tailscale SSH (§6 above). Hardware, dynamic DNS, and
  uptime-monitoring recommendations for running this as a personal always-on box are
  covered in the deploy-pipeline planning notes from the prior session — ask if you'd
  like those folded into this doc as a dedicated section.

## 8. GitHub Actions self-hosted runner

This is what lets `.github/workflows/deploy.yml` deploy to the server automatically.
The runner polls GitHub over **outbound HTTPS only** — no inbound port is opened, so
UFW stays at 22/80/443.

1. **Generate the registration command** — go to
   `https://github.com/wijzijrd/quillit/settings/actions/runners/new`, choose
   **Linux / x64**. GitHub shows a `./config.sh --url ... --token ...` command with a
   short-lived token — copy it fresh from that page each time (it can't be hardcoded
   here).
2. **Install in its own directory**, separate from the app checkout:
   ```sh
   mkdir -p ~/actions-runner && cd ~/actions-runner
   # paste the download + tar -xzf commands from the GitHub page
   ```
   Keep this outside `~/quillit` — and specifically outside `~/quillit/svc/`, which is
   this repo's own Go service directory and would collide in name with the runner's
   `svc.sh` installer script.
3. **Register with the `quillit` label** — the config command GitHub gives you accepts
   `--labels`; make sure it's run with:
   ```sh
   ./config.sh --url https://github.com/wijzijrd/quillit --token <TOKEN> --labels quillit
   ```
   The label must be exactly `quillit` — `deploy.yml`'s `deploy` job targets
   `runs-on: [self-hosted, quillit]`.
4. **Install as a systemd service** so it survives reboots and doesn't need a
   logged-in session:
   ```sh
   sudo ./svc.sh install <username> && sudo ./svc.sh start
   ```
   Replace `<username>` with the deploy user (the one that ran `setup.sh` and owns
   `~/quillit`) — the runner needs to act as that user to reach Docker and the repo.
5. **Docker access** — no extra step needed if `<username>` is the same user
   `setup.sh` added to the `docker` group in step 3 of the app setup above.
6. **Non-interactive git access** — the pipeline runs `git fetch` / `git reset --hard`
   as this user in `~/quillit`. Confirm `git -C ~/quillit remote -v` uses a URL that
   doesn't require an interactive credential prompt (a public HTTPS clone URL is
   fine for a public repo, as used above; use a deploy key or a credential helper if
   the repo is private).

Check runner health any time with `sudo ./svc.sh status` (from `~/actions-runner`), or
in GitHub under **Settings → Actions → Runners**.

## 9. Pipeline secrets & variables

Set these once in the repo's GitHub settings (**Settings → Secrets and variables →
Actions**):

- **Secret `RUNNER_STATUS_TOKEN`** *(required)* — a fine-grained personal access
  token, created at `https://github.com/settings/tokens?type=beta`, scoped to **this
  repository only**, with **Administration: Read** permission. `preflight` in
  `deploy.yml` uses it to check whether the runner is online — the default
  `GITHUB_TOKEN` can't list runners, so this step is required for the health gate to
  work.
- **Variable `QUILLIT_DIR`** *(optional)* — absolute path to the app checkout on the
  server, only needed if it isn't at `$HOME/quillit`.

## 10. Verify end-to-end

1. In GitHub, go to **Actions → Deploy to home server → Run workflow** (manual
   dispatch).
2. Watch `preflight` — it should report the runner `online` and pass.
3. Watch `deploy` — pulls `main`, `./compose.sh build`, `./compose.sh up -d
   --remove-orphans`.
4. Watch `smoke` — checks `ui` (via `compose.sh exec` against its internal port 80,
   so it works whether or not `ui` has a host port published), and `/healthz` on
   `svc` (`:3000`) and `auth` (`:3002`), also via `compose.sh exec`. All three must
   pass for the job to go green.
5. **Negative test** — stop the runner (`sudo ./svc.sh stop` in `~/actions-runner`),
   trigger the workflow again, and confirm `preflight` fails fast with
   `Home server runner is not online` instead of the job hanging or queuing
   indefinitely. Restart the runner (`sudo ./svc.sh start`) afterward.
6. **Rollback test (optional)** — push a commit that breaks the build or `/healthz`,
   confirm `smoke` fails and the `Roll back on failure` step resets to the previous
   commit and redeploys it automatically.

## 11. Day 2 ops

**Backups:**
```sh
./backup.sh
```
Writes a timestamped folder under `backups/` containing `quillit.db`,
`quillit-auth.db`, and a `minio-<date>.tar.gz` of all uploaded files. Safe to run
while the app is live (SQLite WAL mode).

**MinIO console** (bound to `127.0.0.1`, not exposed by the firewall):
```sh
ssh -L 9001:localhost:9001 user@your-server
```
then open `http://localhost:9001` — credentials are `MINIO_USER` / `MINIO_PASSWORD`
from `.env`.

**Grafana / log dashboard** (bound to `127.0.0.1`, if the logging overlay is enabled):
```sh
ssh -L 3001:localhost:3001 user@your-server
```
then open `http://localhost:3001` — credentials are `GRAFANA_ADMIN_USER` /
`GRAFANA_ADMIN_PASSWORD` from `.env`.

**Manual update** (fallback if you're not using the pipeline):
```sh
git pull && ./compose.sh build && ./compose.sh up -d
```

**Troubleshooting:**

| Symptom | Check |
|---|---|
| `preflight` says runner not online | `sudo ./svc.sh status` in `~/actions-runner`; restart with `sudo ./svc.sh start` if stopped. |
| `smoke` fails on a service's `/healthz` | `./compose.sh logs <svc\|auth>` — the handler pings its SQLite DB, so a `503` usually means a DB/volume problem. |
| App unreachable at `:8080` / domain | `sudo ufw status` (only 22/80/443 should be open), `./compose.sh ps` to confirm containers are `Up`. |
| HTTPS not issuing a cert | Confirm the domain's A record resolves to the server and 80/443 are actually reachable from the internet (not just locally); check `./compose.sh logs caddy`. |
| Deploy pipeline can't `git pull` | Confirm `git -C ~/quillit remote -v` and that the runner's user can read it non-interactively (see §8.6). |
