# Quillit

Monorepo: 3 services.

- `ui/` — Vue 3 + Vite frontend (primary work area)
- `svc/` — Go backend API
- `auth/` — Go auth service

When working on UI tasks, stay in `ui/`. Don't read backend services unless explicitly asked.

## Dev commands (run from ui/)
- `npm run dev` — start dev server (port 5173, proxies /api to localhost:3000)
- `npx vue-tsc --noEmit` — type check
