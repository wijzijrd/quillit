# Design: Invite Flow Fix, Project-Scoped Categories, Account-Search Invite

**Date:** 2026-05-27

---

## Context

Three related UX problems in Quillit:

1. **Invite link context loss** — when a user (logged-in or not) follows an invite link, they get shown the login/register screen without the invite token being preserved. After auth they land on the dashboard with no indication they were invited anywhere.

2. **Categories always visible** — categories appear in the sidebar, entry editor, and notes view regardless of whether the user is working inside a campaign/book. Outside a project, notes should be plain and uncategorised.

3. **No account-search invite** — the only way to invite someone is by generating a token link and sending it manually. There's no way to search for an existing user and add them directly.

---

## Feature 1 — Fix Invite Context Through Auth

### Problem

`LoginView` always redirects to `/` after login. `SetupView` renders the registration form even when the user is already authenticated. The router guard redirects to `/login` without preserving the original URL or its query params (including `?invite=TOKEN`).

### Design

**Router guard (`router/index.js`):** When redirecting unauthenticated users, preserve the original destination:
```js
if (!auth.isLoggedIn) return { path: '/login', query: { redirect: to.fullPath } }
```

**`LoginView.vue`:** Read `useRoute`. After successful login, navigate to `route.query.redirect` if present, otherwise `/`:
```js
router.push(route.query.redirect || '/')
```

**`SetupView.vue`:** On mount, `await auth.fetchMe()`. Three cases:
- Already logged in + invite token → auto-call `projects.join(token)` → redirect to `/projects/:id/notes`
- Already logged in, no invite → redirect to `/`
- Not logged in → render the form as normal (existing behaviour)

After `projects.join(token)` the returned membership contains `project_id`, which is used as the redirect target. The `/projects/:id/notes` route is introduced in Feature 2.

**Invite link format stays the same:** `/register?invite=TOKEN`. When an already-logged-in user follows this, they skip the form entirely.

---

## Feature 2 — Project-Scoped Routes and Hybrid Categories

### New Routes

```
/projects/:id          → ProjectView.vue   (members, invite, category management)
/projects/:id/notes    → QuillitView.vue   (notes inside project context)
```

Both routes are protected (non-public). The existing `/notes` and `/` routes remain global and category-free.

### Project Context Detection

The route param `id` inside `/projects/:id/...` is the single source of truth. Components use `const inProject = computed(() => !!route.params.id)`. No additional store state is needed.

### Category Visibility

When `inProject` is false (global notes, dashboard, tag view):
- `SideNav.vue` — `.nav-section--cats` block hidden
- `EntryEditor.vue` — category `<select>` hidden (entries created outside a project have no category)
- `QuillitView.vue` — no grouping by category; plain chronological list
- `DashboardView.vue` — category tab strip hidden

When `inProject` is true:
- All of the above are shown, scoped to the active project's categories

### Hybrid Category Data Model

**Backend migration (quillit-svc `db.go`):**

1. Seed a system project row on startup if it doesn't exist:
   ```sql
   INSERT OR IGNORE INTO projects (id, name, type, created_by, created_at)
   VALUES ('global', 'Global Categories', 'global', 'system', <unix_ts>)
   ```

2. Add `project_id` column to `categories` (additive, defaults to `'global'` so existing rows are migrated automatically):
   ```sql
   ALTER TABLE categories ADD COLUMN project_id TEXT NOT NULL DEFAULT 'global'
     REFERENCES projects(id) ON DELETE CASCADE;
   ```

3. Add junction table for projects opting-in to global categories:
   ```sql
   CREATE TABLE IF NOT EXISTS project_global_categories (
     project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
     category_id TEXT NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
     PRIMARY KEY (project_id, category_id)
   );
   ```

**Category ownership rules:**
- `project_id = 'global'` → admin-managed global category, reusable by any project
- `project_id = <project-id>` → project-specific category, only visible within that project
- A project's full category set = its own categories + global ones it has opted into (via junction)

**New backend API (`projects.go`):**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/projects/:id/categories` | Returns project's own + opted-in global categories |
| `POST` | `/api/projects/:id/categories` | Create project-specific category (editor only) |
| `POST` | `/api/projects/:id/categories/global/:catId` | Opt-in a global category |
| `DELETE` | `/api/projects/:id/categories/:catId` | Remove own category or opt-out global |

Existing `GET/POST/DELETE /api/categories` endpoints remain unchanged — they operate implicitly on `project_id='global'` and are admin-only.

**Frontend store change (`useCategoriesStore.js`):**

Add `initForProject(projectId)` that calls `GET /api/projects/:id/categories` and stores results separately from the global `init()`. The global `init()` is only called by the admin view.

**`QuillitView.vue`:** When `route.params.id` is present, call `cats.initForProject(id)` instead of `cats.init()`. The view still shows all user notes — filtering notes by project membership is out of scope for this spec. Existing entries that already have a category value keep that value in storage; the field is simply not shown or editable when outside a project context (no data loss).

---

## Feature 3 — Account-Search Invite in ProjectView

### New `ProjectView.vue`

Route: `/projects/:id`

Sections:

**1. Project header** — name, type badge (Campaign / Book), edit button for editors.

**2. Members list** — current members with role label, remove button for editors.

**3. Add member via search**
- Text input with 280ms debounce (matches existing pattern in `NoteSharePanel.vue`)
- Calls existing `member.searchUsers(q)` (min 2 chars) from `useMemberStore`
- Results dropdown shows username + email
- Clicking "Add" calls `projects.addMember(projectId, userId, role)` — auto-adds immediately
- Role defaults to the project's member role (e.g. `player` for campaigns, `collaborator` for books)
- Small footnote: "In a future update, invitations will require the user to accept."

**4. Invite link** — existing generate-link flow moved here from the dashboard project card. Shows the token URL with a copy button.

### Existing infrastructure reused

- `member.searchUsers(q)` — `useMemberStore` → `GET /api/users/search?q=`
- `projects.addMember(projectId, userId, role)` — `useProjectStore` → `POST /api/projects/:id/members`
- `projects.generateInvite(projectId, role)` — `useProjectStore` → `POST /api/projects/:id/invite`
- `projects.fetchMembers(projectId)` — `useProjectStore` → `GET /api/projects/:id/members`

No new backend endpoints needed for this feature.

---

## Verification

1. **Invite flow (logged-out):** Open invite URL in an incognito window → register → lands on `/projects/:id/notes` of the joined project.
2. **Invite flow (logged-in):** While logged in, open invite URL → no form shown → auto-joined → redirected to project notes.
3. **Invite flow (login path):** Open invite URL while logged out, click "Sign in" link → after login, redirect chain preserves original URL → joined → redirected to project.
4. **Category visibility:** Navigate to `/notes` → no category sections visible. Navigate to `/projects/:id/notes` → categories shown, scoped to that project.
5. **Account search invite:** Open `/projects/:id` → type username in Add Member field → user appears → click Add → member appears in members list.
6. **Global category opt-in:** In project category management, opt in a global category → it appears in the project's notes view category section.
