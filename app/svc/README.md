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
| GET | `/api/auth/status` | Is the deployment set up? |
| GET | `/api/auth/users/available` | List usernames available for login |
| POST | `/api/auth/register` | Create account + session |
| POST | `/api/auth/login` | Verify credentials + session |
| POST | `/api/auth/logout` | Clear session cookie |
| GET | `/api/auth/me` | Confirm active session |
| POST | `/api/auth/forgot-password` | Request a password reset |
| POST | `/api/auth/reset-password` | Complete a password reset |

### Session routes (session cookie required)

| Method | Path | Description |
|--------|------|-------------|
| GET/PATCH | `/api/me/settings` | Get / update the current user's settings |
| GET | `/api/projects/types` | List available project types |
| GET/POST | `/api/projects` | List / create projects |
| POST | `/api/projects/join` | Join a project via invite |
| PATCH/DELETE | `/api/projects/:id` | Update / delete a project |
| GET/POST | `/api/projects/:id/members` | List / add members |
| DELETE | `/api/projects/:id/members/:userId` | Remove a member |
| POST | `/api/projects/:id/invite` | Create an invite |
| DELETE | `/api/projects/:id/invite/:token` | Revoke an invite |
| GET/POST/DELETE | `/api/content/facets*` | Proxy to `quillit-content`: facet vocabulary |
| GET/PATCH/DELETE | `/api/content/entries/:id` | Proxy to `quillit-content`: get / update / delete an entry |
| POST | `/api/content/projects/:id/entries` | Proxy to `quillit-content`: create an entry |
| GET | `/api/content/entries/:id/render` | Proxy to `quillit-content`: rendered entry HTML |
| GET | `/api/content/entries/:id/images/:filename` | Proxy to `quillit-content`: entry image |
| POST | `/api/content/projects/:id/import` | Proxy to `quillit-content`: bulk import |
| POST/GET | `/api/projects/:projectId/session/*` | Game Mode session actions |
| GET | `/api/projects/:projectId/sessions` | List game sessions |
| GET | `/api/projects/:projectId/session/socket` | Game Mode WebSocket |

### Admin routes (admin session required)

| Method | Path | Description |
|--------|------|-------------|
| GET/PATCH/DELETE | `/api/admin/users*` | Manage users |
| GET | `/api/admin/projects*` | Manage projects |
