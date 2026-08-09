# Quillit

Monorepo, 5 top-level categories:

- `app/` — the 4 deployed services
  - `app/ui/` — Vue 3 + Vite frontend (primary work area)
  - `app/svc/` — Go backend API
  - `app/auth/` — Go auth service
  - `app/messaging/` — Go email/notification service
- `infra/` — docker-compose files + Caddyfile
- `cli/` — quillit CLI (see `cli/README.md`)
- `observability/` — Loki/Promtail/Grafana log-aggregation config
- `operations/` — day-2 scripts (`setup.sh`, `backup.sh`) and server runbooks

Plus `pkg/contentengine/` — shared Go package (parse/filter/render/export), used by `cli/`.

When working on UI tasks, stay in `app/ui/`. Don't read backend services unless explicitly asked.

## Dev commands (run from app/ui/)
- `npm run dev` — start dev server (port 5173, proxies /api to localhost:3000)
- `npx vue-tsc --noEmit` — type check
