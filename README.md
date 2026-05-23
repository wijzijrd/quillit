# quillit-ui

Vue 3 + Vite frontend for Quillit — a D&D/tabletop RPG campaign manager for Game Masters.

Auth is managed via HTTP-only session cookies set by `quillit-svc`. The browser never handles JWTs directly.

## Prerequisites

- Node.js 22+
- `quillit-svc` running on :3000

## Local dev

```bash
npm install
npm run dev    # Vite dev server on :5173, proxies /api → localhost:3000
```

Or with make:

```bash
make setup    # creates .env from .env.example (optional)
make dev
```

## Build for production

```bash
npm run build    # output in dist/
```

The `dist/` directory contains static files. Serve with any static file host or the provided Dockerfile (nginx).

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `VITE_API_URL` | `http://localhost:3000` | quillit-svc URL (used in production builds) |

In development, the Vite proxy handles API routing automatically — `VITE_API_URL` is only needed for production builds that don't use the proxy.
