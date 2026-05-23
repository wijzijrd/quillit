# Running Quillit Locally

Quillit is split into three services. This guide covers getting all of them running for local testing.

```
Browser
  └──► quillit-svc   :3000   (Go — API + session management)
           └──► quillit-auth-svc  :3002   (Go — auth)

quillit-ui   :5173 / :8080   (Vue 3 — frontend)
```

---

## Prerequisites

| Tool | Version | Required for |
|------|---------|-------------|
| Docker + Docker Compose | v2+ | Option A |
| Go | 1.26+ | Option B |
| Node.js | 22+ | Option B |

---

## Option A — Docker Compose (recommended for testing)

All three services build and start with a single command.

**1. Create the shared `.env` file**

```bash
cd ~/repositories/quillit-svc
make setup          # copies .env.example → .env
```

**2. Generate a safe JWT secret and edit `.env`:**

```bash
openssl rand -hex 32   # generates a safe secret — copy the output
```

Then open `.env` and set:

```
JWT_SECRET=<output from above>
SEED_ADMIN_EMAIL=admin@quillit.dev
SEED_ADMIN_PASSWORD=changeme
```

> **Warning:** `JWT_SECRET` must not contain `$`, backticks, or other shell-special characters. Docker Compose expands them as variable references and silently corrupts the value, breaking all auth. `openssl rand -hex 32` is safe by construction.

The other values in `.env` can stay as-is for local use.

**3. Cross-compile the Go services for Linux**

The Docker images use pre-built binaries — no compilation happens inside Docker.

```bash
cd ~/repositories/quillit-auth-svc && make build-linux
cd ~/repositories/quillit-svc     && make build-linux
```

Each completes in a few seconds. Re-run whenever you change Go source code.

**4. Start everything**

```bash
cd ~/repositories/quillit-svc
docker compose up --build
```

| Service | URL |
|---------|-----|
| UI | http://localhost:8080 |
| quillit-svc API | http://localhost:3000 |
| quillit-auth-svc | http://localhost:3002 |

**4. First login**

Go to http://localhost:8080 — you'll be redirected to `/login`. Sign in with the `SEED_ADMIN_EMAIL` and `SEED_ADMIN_PASSWORD` you set above.

To create a GM account instead, visit http://localhost:8080/register.

**Stopping:**

```bash
docker compose down          # stop containers
docker compose down -v       # stop + delete database volumes (full reset)
```

---

## Option B — Native processes (for active development)

Run each service in its own terminal so you can edit code and see changes immediately. The Vite dev server hot-reloads the UI; the Go services require a restart on code changes (`go run .`).

**Terminal 1 — quillit-auth-svc**

```bash
cd ~/repositories/quillit-auth-svc
make setup          # creates .env from .env.example
# Edit .env: set JWT_SECRET, SEED_ADMIN_EMAIL, SEED_ADMIN_PASSWORD
make dev            # starts on :3002
```

**Terminal 2 — quillit-svc**

```bash
cd ~/repositories/quillit-svc
make setup          # creates .env from .env.example
# Edit .env: set JWT_SECRET to the SAME value as quillit-auth-svc
make dev            # starts on :3000
```

**Terminal 3 — quillit-ui**

```bash
cd ~/repositories/quillit-ui
npm install
npm run dev         # starts Vite dev server on :5173
```

| Service | URL |
|---------|-----|
| UI (Vite) | http://localhost:5173 |
| quillit-svc API | http://localhost:3000 |
| quillit-auth-svc | http://localhost:3002 |

The Vite dev server automatically proxies `/api/*` requests to `quillit-svc` on port 3000 — no extra config needed.

> **Important:** `JWT_SECRET` must be identical in both `quillit-auth-svc/.env` and `quillit-svc/.env`. If they differ, sessions will fail to verify.

---

## Environment variable reference

### quillit-auth-svc

| Variable | Default | Notes |
|----------|---------|-------|
| `PORT` | `3002` | Listen port |
| `DB_PATH` | `./quillit-auth.db` | SQLite file |
| `JWT_SECRET` | — | **Required. Must match `quillit-svc`.** |
| `SEED_ADMIN_EMAIL` | — | If set, creates an admin account on startup |
| `SEED_ADMIN_PASSWORD` | — | Required if `SEED_ADMIN_EMAIL` is set |

### quillit-svc

| Variable | Default | Notes |
|----------|---------|-------|
| `PORT` | `3000` | Listen port |
| `DB_PATH` | `./quillit.db` | SQLite file |
| `AUTH_SERVICE_URL` | `http://localhost:3002` | URL of quillit-auth-svc |
| `JWT_SECRET` | — | **Required. Must match `quillit-auth-svc`.** |
| `COOKIE_SECURE` | `false` | Set to `true` in production (HTTPS only) |
| `CORS_ORIGIN` | `http://localhost:5173` | Allowed browser origin |

### quillit-ui

| Variable | Default | Notes |
|----------|---------|-------|
| `VITE_API_URL` | `http://localhost:3000` | Only needed for production builds |

In dev (`npm run dev`), the Vite proxy handles API routing — this var is not read at runtime.

---

## Accounts and roles

| Role | Created by | Access |
|------|-----------|--------|
| `admin` | Seed on startup (`SEED_ADMIN_*`) | Full access, for testing |
| `gm` | `/register` screen | Full GM access |
| `player` | Reserved for future use | — |

Players currently access campaigns via share tokens (no account required).
