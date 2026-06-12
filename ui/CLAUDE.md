# quillit-ui

Vue 3 + Vite + TypeScript + Tailwind v4 + shadcn-vue frontend.

## Stack
- Framework: Vue 3 (Composition API, `<script setup lang="ts">`)
- Build: Vite
- State: Pinia (stores in `src/stores/`)
- Router: Vue Router (`src/router/index.ts`)
- Styling: Tailwind v4 + shadcn-vue CSS vars (Parchment & Ink theme, light + dark)
- UI primitives: shadcn-vue (components in `src/components/ui/`)
- Rich text: Tiptap with custom extensions (`src/extensions/`)
- Icons: lucide-vue-next
- HTTP: ofetch (`src/api/`)
- Storage: idb-keyval (IndexedDB)

## Structure
- `src/components/` — reusable Vue components
- `src/components/ui/` — shadcn-vue generated components (owned source)
- `src/views/` — page-level components
- `src/stores/` — Pinia stores (entries, categories, auth, campaign, project, annotations, ui, quickView, admin, member, entryRelations)
- `src/composables/` — composables
- `src/api/` — API layer (typed with ofetch)
- `src/types/index.ts` — shared domain interfaces (Entry, Category, Campaign, etc.)
- `src/assets/main.css` — Tailwind import + shadcn CSS vars (Moon Dust palette) + Tiptap @layer
- `src/extensions/` — custom Tiptap extensions

## Key patterns
- Props: `defineProps<{ title: string }>()` — no withDefaults unless needed
- Emits: `defineEmits<{ close: []; save: [entry: Entry] }>()`
- No Options API — Composition API only
- shadcn-vue components auto-imported from `src/components/ui/`
- Category colors via CSS vars: `var(--cat-npc)`, `var(--cat-location)`, etc.
- Tiptap editor content styles in `@layer components` in main.css — do NOT convert to Tailwind utilities

## Parchment & Ink colour palette

**Light mode:**
- `#8B2E2E` — primary (deep burgundy — buttons, links, focus ring)
- `#C4844A` — accent (terracotta — hover states)
- `#D4C9B0` — secondary surface (warm tan)
- `#C8BDA8` — border/input
- Background: `#F5F0E8` (parchment), Foreground: `#1C1410` (near-black ink)

**Dark mode (.dark on `<html>`):**
- `#C4956A` — primary (amber/candlelight)
- `#7B4F3A` — accent (dark burgundy hover)
- Background: `#14141F` (midnight navy), Foreground: `#E8E0D0` (warm cream)

**Category colors (light / dark):**
- NPC: `#1E4D8C` / `#6B9ED4` — steel blue
- Location: `#1A5C35` / `#52A874` — forest green
- Faction: `#5B1F8A` / `#A66DD4` — royal violet
- Event: `#8C5A0A` / `#D4A44C` — amber ochre
- Item: `#8B1A1A` / `#D46464` — deep crimson
- Lore: `#2C2E8C` / `#7070D4` — indigo

Category colors: text/border only, never fill. Dark variants activate via `.dark` on `<html>`.

## Don't read
- `node_modules/`
- `dist/`
- `*.lock`
