# quillit-svc

Backend for Frontend + application service for Quillit. Handles sessions (HTTP-only cookies), all GM and player API routes, and proxies auth requests to `quillit-auth-svc`. Written in Go with SQLite.

## Architecture

```
Browser (quillit-ui)
  └──► quillit-svc :3000   (this service)
           └──► quillit-auth-svc :3002
```

All browser traffic goes through `quillit-svc`. The session cookie is HTTP-only — the browser never sees the JWT directly.

## Prerequisites

- Go 1.26+
- `quillit-auth-svc` running (for auth endpoints)
- Docker (for containerised runs)

## Local dev

```bash
make setup        # creates .env from .env.example
# edit .env — set JWT_SECRET (must match quillit-auth-svc)
make dev          # runs on :3000
```

## Running all services together (Docker)

From this directory:

```bash
cp .env.example .env
# edit .env — set JWT_SECRET
docker compose up --build
```

This starts `quillit-auth-svc` (:3002), `quillit-svc` (:3000), and `quillit-ui` (:8080).

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `3000` | Listen port |
| `DB_PATH` | `./quillit.db` | SQLite file path |
| `AUTH_SERVICE_URL` | `http://localhost:3002` | URL of quillit-auth-svc |
| `JWT_SECRET` | — | **Required.** Must match `quillit-auth-svc`. |
| `COOKIE_SECURE` | `false` | Set to `true` in production (HTTPS only) |
| `CORS_ORIGIN` | `http://localhost:5173` | Allowed origin for CORS |

## API reference

### Auth (no session required)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/auth/status` | Is the GM account set up? |
| POST | `/auth/setup` | `{ "password" }` → create account + session |
| POST | `/auth/login` | `{ "password" }` → verify + session |
| POST | `/auth/logout` | Clear session cookie |
| GET | `/auth/me` | Confirm active session |

### GM routes (session cookie required)

| Method | Path | Description |
|--------|------|-------------|
| GET/POST | `/api/entries` | List / create entries |
| GET/PATCH/DELETE | `/api/entries/:id` | Get / update / delete entry |
| GET/POST | `/api/campaigns` | List / create campaigns |
| PATCH/DELETE | `/api/campaigns/:id` | Update / delete campaign |
| POST | `/api/campaigns/:id/players` | Add player |
| DELETE | `/api/campaigns/:id/players/:playerId` | Remove player |
| GET/POST | `/api/annotations` | List / create annotations |
| PATCH/DELETE | `/api/annotations/:id` | Update / delete annotation |
| GET | `/api/quickview` | List quick-view templates |
| PUT/DELETE | `/api/quickview/:category` | Upsert / delete template |
| POST | `/api/migrate/import` | Bulk import (clears existing data) |

### Player routes (public — token-based, no session)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/share/:token` | Public entries for this player |
| GET/POST | `/api/share/:token/notes` | Player's notes |
| PATCH/DELETE | `/api/share/:token/notes/:noteId` | Edit / delete note |
