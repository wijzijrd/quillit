# quillit-auth-svc

Authentication service for Quillit. Handles GM account setup, login, and JWT issuance. Written in Go with SQLite.

## Prerequisites

- Go 1.26+
- Docker (for containerised runs)

## Local dev

```bash
make setup        # creates .env from .env.example
# edit .env — set JWT_SECRET to a long random string
make dev          # runs on :3002
```

On first run, call setup to create the GM account:

```bash
curl -X POST http://localhost:3002/auth/setup \
  -H 'Content-Type: application/json' \
  -d '{"password":"yourpassword"}'
```

## API

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/auth/status` | — | Returns `{ "setup": true/false }` |
| POST | `/auth/setup` | — | `{ "password" }` → create account, returns JWT. 409 if already done. |
| POST | `/auth/login` | — | `{ "password" }` → verify, returns JWT |
| POST | `/auth/verify` | — | `{ "token" }` → validate JWT, returns claims |

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `3002` | Listen port |
| `DB_PATH` | `./quillit-auth.db` | SQLite file path |
| `JWT_SECRET` | — | **Required.** Must match `quillit-svc`. |

## Docker

```bash
docker build -t quillit-auth-svc .
docker run -p 3002:3002 -v auth-data:/data \
  -e JWT_SECRET=yoursecret quillit-auth-svc
```
