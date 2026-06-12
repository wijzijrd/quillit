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

## MinIO console

MinIO's admin console is bound to `localhost` only and not exposed through the firewall. Access it via an SSH tunnel:

```sh
ssh -L 9001:localhost:9001 user@your-server
```

Then open **http://localhost:9001** in your browser. Default credentials are the `MINIO_USER` / `MINIO_PASSWORD` values from your `.env`.
