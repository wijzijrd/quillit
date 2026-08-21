# quillit-ui

Vue 3 + Vite + TypeScript + Tailwind v4 + shadcn-vue frontend.

## Stack
- Framework: Vue 3 (Composition API, `<script setup lang="ts">`)
- Build: Vite
- State: Pinia (stores in `src/stores/`)
- Router: Vue Router (`src/router/index.ts`)
- Styling: Tailwind v4 + shadcn-vue CSS vars (4 seasonal themes + glass mode, see below)
- UI primitives: shadcn-vue (components in `src/components/ui/`)
- Rich text: Tiptap with custom extensions (`src/extensions/`)
- Icons: lucide-vue-next
- HTTP: ofetch (`src/api/`)
- Storage: idb-keyval (IndexedDB)

## Structure
- `src/components/` — reusable Vue components
- `src/components/ui/` — shadcn-vue generated components (owned source)
- `src/views/` — page-level components
- `src/stores/` — Pinia stores (entries, entry, auth, project, facets, import, liveSession, ui, admin)
- `src/composables/` — composables
- `src/api/` — API layer (typed with ofetch)
- `src/types/index.ts` — shared domain interfaces (Entry, Category, Campaign, etc.)
- `src/assets/main.css` — Tailwind import + seasonal CSS vars + surface layering system + Tiptap @layer
- `src/extensions/` — custom Tiptap extensions

## Key patterns
- Props: `defineProps<{ title: string }>()` — no withDefaults unless needed
- Emits: `defineEmits<{ close: []; save: [entry: Entry] }>()`
- No Options API — Composition API only
- shadcn-vue components auto-imported from `src/components/ui/`
- Category colors via CSS vars: `var(--cat-npc)`, `var(--cat-location)`, etc.
- Tiptap editor content styles in `@layer components` in main.css — do NOT convert to Tailwind utilities

## Seasonal themes

Four themes selected via `data-season` on `<html>` (+ `.dark` for autumn/winter so `dark:` variants work). Managed by `useUIStore` (`season`, `glass`, `setSeason`, `setGlass`); synced to `/me/settings` as `{ season, glass }`; localStorage keys `quillit-season`, `quillit-glass`. Default = northern-hemisphere calendar season. Legacy `theme: 'light'|'dark'` values migrate brightness-preserving.

| Season | bg | fg | primary | accent | dark? |
|---|---|---|---|---|---|
| Spring | `#F2F5EC` | `#25301F` | fern `#3F7D5A` | lilac `#8E7CC3` | no |
| Summer | `#FBF6EA` | `#2A2418` | sea `#1F6E8C` | marigold `#E8A13D` | no |
| Autumn | `#221A14` | `#EDE3D0` | ember `#C96F32` | oxblood `#A04848` | yes |
| Winter | `#131722` | `#E4E9F0` | glacial `#7FA6C9` | aurora `#5FB3A1` | yes |

CSS structure in `main.css`: `:root` = spring (default), `.dark` = autumn (fallback), `[data-season='summer']` and `[data-season='winter']` override after. Each block also sets `--backdrop-gradient` (placeholder artwork for `SeasonBackdrop.vue`; real images slot in via its `src` prop).

Fonts: `--font-display: 'Fraunces'`, `--font-body: 'Instrument Sans'`. Radius: `--radius: 8px`.

**Category colors (light seasons / dark seasons):**
- NPC: `#1E4D8C` / `#6B9ED4` — steel blue
- Location: `#1A5C35` / `#52A874` — forest green
- Faction: `#5B1F8A` / `#A66DD4` — royal violet
- Event: `#8C5A0A` / `#D4A44C` — amber ochre
- Item: `#8B1A1A` / `#D46464` — deep crimson
- Lore: `#2C2E8C` / `#7070D4` — indigo

Category colors: text/border only, never fill.

## Surface layering contract (glass mode)

Full contract documented in `main.css`. Summary:
- **Depth 0**: `SeasonBackdrop.vue` (`.season-backdrop`) — always opaque; the only thing glass blurs.
- **Depth 1**: `.surface` — sits directly on the backdrop (SideNav + main panel in `AppShell.vue`). Translucent + `backdrop-filter` when `html.glass` is set; collapses to opaque when glass is off (via `--surface-alpha`).
- **Depth 2**: floating overlays (Dialog/Popover/Dropdown/Sheet/Command) use `bg-popover`; `--popover` is always an opaque hex. Never put `.surface` on overlay content; never make `--popover` translucent.
- **Depth 3**: components inside an opaque overlay may use `.surface` again.

Rule: glass is only legal when the layer directly beneath is opaque. Cards/panels **inside** the glass main panel stay opaque (`var(--card)`) — do not glassify them.

Public routes (`meta.public`: login, register, share) render chromeless — no SideNav, no `.surface` main. Auth pages use `src/layouts/AuthLayout.vue` (split seasonal splash + form panel).

## Don't read
- `node_modules/`
- `dist/`
- `*.lock`
