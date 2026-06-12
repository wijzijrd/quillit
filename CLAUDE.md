# Quillit

Monorepo: 3 services.

- `quillit-ui/` — Vue 3 + Vite frontend (primary work area)
- `quillit-svc/` — Go backend API
- `quillit-auth-svc/` — Go auth service

When working on UI tasks, stay in `quillit-ui/`. Don't read backend services unless explicitly asked.

## Dev commands (run from quillit-ui/)
- `npm run dev` — start dev server (port 5173, proxies /api to localhost:3000)
- `npx vue-tsc --noEmit` — type check
