# quillit-messaging-svc

Messaging service for Quillit. Sends transactional email (e.g. forgot-password links) over SMTP. Written in Go, stateless.

## Prerequisites

- Go 1.26+
- Docker (for containerised runs)

## Local dev

```bash
make setup        # creates .env from .env.example
# edit .env — set SMTP_* to your mail provider's credentials
make dev          # runs on :3003
```

## API

Messaging has no HTTP API of its own beyond `/healthz` and `/swagger/*`. It
serves `MessagingInternalService` (`SendEmail`) as a connectrpc
server-to-server RPC, reached only by auth's password-reset flow and gated
by the `INTERNAL_RPC_SECRET` shared secret (see `gen/internalauth`) rather
than a per-request header — the connectrpc replacement for the old
`POST /send` HTTP route.

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `3003` | Listen port |
| `SMTP_HOST` | — | SMTP server hostname |
| `SMTP_PORT` | `587` | SMTP server port |
| `SMTP_USERNAME` | — | SMTP auth username |
| `SMTP_PASSWORD` | — | SMTP auth password |
| `SMTP_FROM` | — | From address used on outgoing mail |
| `INTERNAL_RPC_SECRET` | — | Shared secret gating `MessagingInternalService`. Not yet wired into `infra/docker-compose.yml`. |

## Docker

The build context is the repo root, not this directory — the Dockerfile
needs the sibling `gen` module in scope (see the Dockerfile's header
comment). Run from the repo root:

```bash
docker build -f app/messaging/Dockerfile -t quillit-messaging-svc .
docker run -p 3003:3003 \
  -e SMTP_HOST=yourhost -e SMTP_USERNAME=youruser -e SMTP_PASSWORD=yourpass \
  -e SMTP_FROM=noreply@example.com -e INTERNAL_RPC_SECRET=yoursecret quillit-messaging-svc
```
