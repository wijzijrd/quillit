# quillit-messaging-svc

Messaging service for Quillit. Sends transactional email (e.g. forgot-password links) over SMTP. Written in Go, stateless.

## Prerequisites

- Go 1.26+
- Docker (for containerised runs)

## Local dev

```bash
make setup        # creates .env from .env.example
# edit .env — set SMTP_* to your mail provider's credentials and MESSAGING_SECRET to a long random string
make dev          # runs on :3003
```

## API

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/send` | — | Sends an email via SMTP. |

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `3003` | Listen port |
| `SMTP_HOST` | — | SMTP server hostname |
| `SMTP_PORT` | `587` | SMTP server port |
| `SMTP_USERNAME` | — | SMTP auth username |
| `SMTP_PASSWORD` | — | SMTP auth password |
| `SMTP_FROM` | — | From address used on outgoing mail |
| `MESSAGING_SECRET` | — | **Required.** Shared secret for authenticating callers of this service. |

## Docker

```bash
docker build -t quillit-messaging-svc .
docker run -p 3003:3003 \
  -e SMTP_HOST=yourhost -e SMTP_USERNAME=youruser -e SMTP_PASSWORD=yourpass \
  -e SMTP_FROM=noreply@example.com -e MESSAGING_SECRET=yoursecret quillit-messaging-svc
```
