# Quillit

A self-hosted worldbuilding and campaign wiki for tabletop RPGs.

## Structure

```
quillit/
├── ui/      Vue 3 + Vite frontend
├── svc/     Go backend API
├── auth/    Go authentication service
├── docker-compose.yml
└── setup.sh  (Linux server automated setup)
```

---

## Option 1 — Docker on Windows or macOS

**Prerequisites:** [Docker Desktop](https://www.docker.com/products/docker-desktop/), Git

```sh
git clone https://github.com/wijzijrd/quillit.git
cd quillit
cp .env.example .env
```

Edit `.env` and set at minimum:

| Variable | What to set |
|----------|-------------|
| `JWT_SECRET` | Run `openssl rand -hex 32` (or any long random string) |
| `SEED_ADMIN_EMAIL` | Your login email |
| `SEED_ADMIN_PASSWORD` | Your initial password |
| `MINIO_PASSWORD` | Any strong password |
| `CORS_ORIGIN` | `http://localhost:8080` (default, leave as-is for local use) |

Then start everything:

```sh
docker compose up --build -d
```

First build takes 2–5 minutes. Once done, open **http://localhost:8080**.

To stop:

```sh
docker compose down
```

---

## Option 2 — Linux server (Recommended)

**Prerequisites:** Ubuntu 22.04 LTS or Debian 12, Git

```sh
git clone https://github.com/wijzijrd/quillit.git
cd quillit
chmod +x setup.sh && ./setup.sh
```

The script will:
- Install Docker Engine and the Compose plugin
- Configure UFW (opens ports 22, 80, 443 only)
- Prompt for your server IP/domain, admin email, and admin password
- Auto-generate `JWT_SECRET` and `MINIO_PASSWORD`
- Build and start all four services

After it completes, the app is available at **http://YOUR_SERVER_IP:8080**.

### HTTPS with a custom domain

<details>
<summary>Expand HTTPS setup instructions</summary>

**Requirements:** a domain with an A record pointing at your server's public IP.

1. Edit `Caddyfile` and replace `your.domain.com` with your domain:

   ```
   your.domain.com {
       reverse_proxy ui:80
   }
   ```

2. Update `.env`:

   ```
   CORS_ORIGIN=https://your.domain.com
   COOKIE_SECURE=true
   ```

3. Start with the Caddy overlay:

   ```sh
   docker compose -f docker-compose.yml -f docker-compose.caddy.yml up -d
   ```

   Caddy fetches a Let's Encrypt certificate automatically. The app will be available at `https://your.domain.com`.

</details>

### Stable LAN address

<details>
<summary>Expand stable LAN address options</summary>

A plain DHCP-assigned IP can change on lease renewal, which will break a self-signed cert or bookmark pinned to it. Two ways to get an address that doesn't drift:

- **Router DHCP reservation** (recommended baseline): in your router's admin UI, find the DHCP client list / reservation settings and bind the server's current LAN IP to its MAC address (`ip link show` on the server to find the MAC). Steps vary by router.
- **`<hostname>.local` mDNS alias**: `setup.sh` installs and enables `avahi-daemon` automatically, which advertises the box at `$(hostname).local` with zero router config. Works out of the box on macOS/iOS, usually fine on Linux; support on Windows and some Android versions is inconsistent, so keep the DHCP reservation as a fallback for those clients.

</details>

### HTTPS on your local network (self-signed, no domain)

<details>
<summary>Expand LAN self-signed HTTPS instructions</summary>

No domain, no public DNS, no port-forwarding — just a TLS-encrypted connection between your devices and the server over LAN. Caddy mints its own certificate from a locally-generated root CA instead of using Let's Encrypt.

1. Get a stable address first (see "Stable LAN address" above) — a router DHCP reservation, the `<hostname>.local` mDNS alias, or both.

2. Update `.env` — the default `Caddyfile` already reads its address from `CADDY_HOST`, so this is the only file you need to touch (comma-separated if using both a LAN IP and a `.local` alias):

   ```
   CADDY_HOST=192.168.1.50, quillit.local
   CORS_ORIGIN=https://192.168.1.50
   COOKIE_SECURE=true
   ```

   CORS only accepts a single origin, so pick whichever address you'll actually browse to — the `.local` alias is the more durable choice since it survives even if a DHCP reservation is ever misconfigured. `Caddyfile.internal.example` documents this same pattern if you ever need to reset `Caddyfile` back to it.

3. Start with the same Caddy overlay used for the public-domain path:

   ```sh
   docker compose -f docker-compose.yml -f docker-compose.caddy.yml up -d
   ```

   On first run Caddy generates a local root CA and mints a leaf certificate for the address above — no internet access or domain required.

4. Trust Caddy's root CA on every device that will connect (otherwise browsers show a "not secure" warning):

   ```sh
   docker compose -f docker-compose.yml -f docker-compose.caddy.yml cp caddy:/data/caddy/pki/authorities/local/root.crt ./quillit-lan-root.crt
   ```

   Copy `quillit-lan-root.crt` to each client device, then install it as a trusted root:

   - **macOS**: double-click to open Keychain Access, double-click the cert → Trust → "Always Trust" (or `sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain quillit-lan-root.crt`).
   - **Windows**: double-click the file → Install Certificate → Local Machine → Trusted Root Certification Authorities (or, elevated: `certutil -addstore -f ROOT quillit-lan-root.crt`).
   - **Linux**: `sudo cp quillit-lan-root.crt /usr/local/share/ca-certificates/ && sudo update-ca-certificates`. Firefox and some Chrome builds use their own certificate store — import via the browser's certificate settings instead.
   - **iOS**: AirDrop/email the file, tap it to install the profile (Settings → General → VPN & Device Management), then enable full trust at Settings → General → About → Certificate Trust Settings.
   - **Android**: Settings → Security → Encryption & credentials → Install a certificate → CA certificate.

Port 80 isn't required for certificate issuance in this mode (there's no ACME challenge), but Caddy still uses it for automatic HTTP→HTTPS redirects, so there's no need to change `docker-compose.caddy.yml` or your firewall rules.

</details>

### Log aggregation (optional)

<details>
<summary>Expand log aggregation setup instructions (Loki + Grafana)</summary>

Adds Grafana Loki (log storage), Promtail (log shipper), and Grafana (query UI) so
container logs are searchable in one place instead of scattered across `docker
compose logs <service>`. No parsing setup needed — logs stay plain text and are
filtered at query time.

1. Set in `.env` (see `.env.example`):

   ```
   LOKI_RETENTION_DAYS=14
   GRAFANA_ADMIN_USER=admin
   GRAFANA_ADMIN_PASSWORD=a-real-password
   ```

2. Start with the logging overlay:

   ```sh
   docker compose -f docker-compose.yml -f docker-compose.logging.yml up -d
   ```

   Combine with the Caddy overlay if you're also using it:

   ```sh
   docker compose -f docker-compose.yml -f docker-compose.caddy.yml -f docker-compose.logging.yml up -d
   ```

   If you used `setup.sh`, add `-f docker-compose.logging.yml` to the `docker compose ${COMPOSE_FLAGS}` line inside the generated `compose.sh`, so `./compose.sh` keeps including it going forward.

3. Access Grafana via SSH tunnel (same pattern as the MinIO console):

   ```sh
   ssh -L 3001:localhost:3001 user@your-server
   ```

   Then open **http://localhost:3001** and log in with `GRAFANA_ADMIN_USER` / `GRAFANA_ADMIN_PASSWORD`. A "Loki" datasource is pre-configured — go to **Explore** and query, e.g. `{service="svc"}` or `{service="svc"} |= "error"`.

Promtail discovers all running containers automatically via the Docker socket — no container list to maintain. Loki keeps `LOKI_RETENTION_DAYS` days of logs (default 14) and deletes older data automatically via its compactor, so storage doesn't grow unbounded the way Docker's default `json-file` log driver does today.

</details>

---

## Option 3 — Native local development

Use this when actively developing. Each service runs natively; only MinIO runs in Docker.

**Prerequisites:** [Go 1.23+](https://go.dev/dl/), [Node.js 22+](https://nodejs.org/), Docker

### 1. Start MinIO

```sh
docker compose up minio -d
```

### 2. Auth service (port 3002)

```sh
cd auth
make setup   # copies .env.example → .env
# Edit auth/.env: set JWT_SECRET to any string (must match svc/.env)
make dev
```

### 3. Main API (port 3000)

```sh
cd svc
make setup   # copies .env.example → .env
# Edit svc/.env: set JWT_SECRET (same value as auth/.env)
make dev
```

### 4. Frontend (port 5173)

```sh
cd ui
npm install
npm run dev
```

Open **http://localhost:5173**. The Vite dev server proxies `/api` requests to `localhost:3000`.

---

## Configuration reference

All variables live in `.env` at the repo root (Docker) or in `svc/.env` / `auth/.env` (native dev).

| Variable | Default | Description |
|----------|---------|-------------|
| `JWT_SECRET` | *(required)* | Shared secret for signing JWTs — must match in both services |
| `SEED_ADMIN_EMAIL` | `admin@quillit.local` | Admin account created on first launch |
| `SEED_ADMIN_PASSWORD` | `changeme` | Admin password — change before exposing to a network |
| `MINIO_USER` | `quillit` | MinIO root username |
| `MINIO_PASSWORD` | *(required)* | MinIO root password |
| `CORS_ORIGIN` | `http://localhost:8080` | URL the browser uses to reach the app |
| `COOKIE_SECURE` | `false` | Set to `true` when serving over HTTPS |

---

## Backup

```sh
./backup.sh
```

Creates a timestamped snapshot in `backups/` containing:
- `quillit.db` — main SQLite database
- `quillit-auth.db` — auth SQLite database
- `minio-<date>.tar.gz` — all uploaded files and entry bodies

Safe to run while the app is live (SQLite WAL mode).

---

## Updating

**Docker (Windows/macOS):**

```sh
git pull
docker compose up --build -d
```

**Linux server (after using `setup.sh`):**

```sh
git pull
./compose.sh build
./compose.sh up -d
```

---

## Automated deploys (CI/CD)

`.github/workflows/deploy.yml` deploys to the home server on every push to `main`
(or via the **Run workflow** button). Because a home server may be off, a
GitHub-hosted **health gate** first checks that the server's runner is online and
fails fast if it isn't — no half-applied deploys, no jobs stuck in a queue.

A **self-hosted runner** on the server polls GitHub over outbound HTTPS, so nothing
new is exposed inbound (UFW still only allows 22/80/443). The pipeline pulls `main`
in the `setup.sh` repo dir, rebuilds, restarts, runs `/healthz` smoke tests, and
**rolls back** to the previous commit if they fail.

**One-time setup on the server** (as the deploy user, in the repo dir):

1. Register a runner labelled `quillit` — see **Settings → Actions → Runners → New**.
2. Install it as a service so it survives reboots:
   ```sh
   sudo ./svc.sh install <username> && sudo ./svc.sh start
   ```
3. Add repo secret **`RUNNER_STATUS_TOKEN`** — a fine-grained PAT scoped to this repo
   with **Administration: Read** (used only to check runner status).
4. Optional: repo variable **`QUILLIT_DIR`** if the repo isn't at `$HOME/quillit`.

Health endpoints (`GET /healthz` on `svc` and `auth`) also make good targets for an
uptime monitor such as [Uptime Kuma](https://github.com/louislam/uptime-kuma).

See **[docs/SERVER_SETUP.md](docs/SERVER_SETUP.md)** for the full zero-to-production
runbook (OS prep, Docker, UFW, HTTPS, and the exact runner install/registration steps).

---

## MinIO console

MinIO's admin console is bound to `localhost` only and not exposed through the firewall. Access it via an SSH tunnel:

```sh
ssh -L 9001:localhost:9001 user@your-server
```

Then open **http://localhost:9001** in your browser. Default credentials are the `MINIO_USER` / `MINIO_PASSWORD` values from your `.env`.
