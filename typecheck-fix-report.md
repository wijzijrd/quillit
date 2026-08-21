# app/ui typecheck fix report

Starting point: 81 `vue-tsc --noEmit` errors (saved snapshot, now deleted per instructions).
Ending point: 0 errors. Build (`npm run build`) succeeds. `npx vitest run`: 52/52 passing, no regressions.

No `any`, non-null assertion (`!`) used to bypass a real gap, unnarrowed `unknown`, or `@ts-ignore`/`@ts-expect-error` was introduced anywhere in the diff (verified by grep over the full diff).

## 1. `src/types/index.ts` — shared types traced to their Go source of truth

### `Project`

Traced against two Go response structs that both get consumed as `Project` in the frontend:

- `app/svc/internal/handler/projects.go`'s `Project` struct (backs `GET/POST/PATCH /api/projects[...]`, i.e. `useProjectStore.fetchProjects/createProject/updateProject`) — fields `ID, Name, Type, CreatedBy, CreatedAt, MemberCount, MyRole, RoleLabels, Live, Members`. None of `Type/MemberCount/MyRole/RoleLabels/Live` carry `omitempty`, and every handler path (`List`, `Create`, `fetchProject` used by `Update`) sets all of them unconditionally.
- `app/svc/internal/handler/admin.go`'s `AdminProject` struct (backs `GET /api/admin/projects`, i.e. `useAdminStore.fetchProjects`, typed as `Project[]` in the frontend) — fields `ID, Name, Type, CreatedBy, CreatedAt, MemberCount, RoleLabels`. **No `MyRole` field exists on this struct at all**, and no `Live` field either.

Added:
- `type: string` — required. Present, unconditionally, in both structs.
- `memberCount: number` — required. Same reasoning.
- `myRole?: string` — **optional**. Genuinely absent from `AdminProject` (not just sometimes-empty — the field doesn't exist on that struct), so a shared type spanning both endpoints can't claim it's always there.
- `roleLabels: [string, string]` — **required** (not optional, despite the task brief's initial suggested diagnosis flagging it as `roleLabels?`). Both `Project.RoleLabels` and `AdminProject.RoleLabels` are set unconditionally via the shared `roleLabelPair()` helper in every code path that produces either struct — there is no path that omits it. I verified no object literal anywhere in the frontend constructs a `Project` by hand (grepped for `: Project` / `<Project>`; every assignment goes through `api()`, which returns `any`, so this didn't need to be optional for either use-site to compile). Given the task's own standard ("don't force something non-optional... and don't loosen something that's actually always present"), I kept it required since that's what's actually true.

Left `typeId?`/`ownerId?` in place (unused dead fields — confirmed via grep that nothing reads them anywhere in `src/`) since removing them wasn't necessary to reach zero errors and wasn't asked for; flagging here rather than silently deleting them.

### `ProjectType`

This one wasn't explicitly called out in the task's diagnosis but turned out to be a **third** broken shared type, not just a symptom of the other two — the existing interface (`{ id: string; name: string }`) doesn't match anything the backend actually returns. Traced to `app/svc/internal/handler/projects.go`'s `ProjectType` struct (`GET /api/projects/types`, the only place `useProjectStore.types` is populated from):
```go
type ProjectType struct {
	Type       string    `json:"type"`
	Label      string    `json:"label"`
	RoleLabels [2]string `json:"roleLabels"`
}
```
Replaced the frontend interface with `{ type: string; label: string; roleLabels: [string, string] }`. All three fields are set unconditionally in the one place this struct is ever constructed (`Types()` handler). Confirmed via grep this type is used in exactly one place (`DashboardView.vue`'s `<option v-for="t in projectStore.types">`, reading `t.type`/`t.label`), so no other call site depends on the old (wrong) shape.

### `ProjectMember`

Traced to the same struct name in `projects.go`, confirmed identical in `admin.go`'s `ListProjectMembers` (reads `id, project_id, user_id, role, joined_at, username` — same six columns, no `omitempty` on any field):
```go
type ProjectMember struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	UserID    string `json:"userId"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	JoinedAt  int64  `json:"joinedAt"`
}
```
Every endpoint that returns a `ProjectMember` (`ListMembers`, `AddMember`, `Join`'s 200 response, admin's `ListProjectMembers`) populates every field — including `Join`, which sets `Username: ""` explicitly rather than omitting it (no `omitempty` tag, so it serializes as `"username":""`, not absent). Added `id: string`, `projectId: string` (unused by the frontend today but genuinely part of the shape — included for fidelity, costs nothing), and `joinedAt: number` (unix seconds, matching this codebase's existing convention — cross-checked against `formatDate(unixSeconds: number)` in `src/utils/date.ts`, which is exactly what `AdminView.vue` calls it with).

### New: `UserSearchResult`

`useProjectStore.searchUsers()` was completely untyped (`return await api(...)`, implicit `any`). Traced the endpoint it's meant to hit (`/users/search`) to auth-svc's `SearchUsers` handler (`app/auth/internal/handler/auth.go`):
```go
type UserSearchResult struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
}
```
Added this as `UserSearchResult` in `types/index.ts` and typed `searchUsers(): Promise<UserSearchResult[]>`. `ProjectView.vue`'s `searchResults` ref and `addUser()` param now use this canonical type instead of the ad-hoc inline `{ id: string; username: string }` shape that was there before (which was itself a guess, missing `email` even though the template reads `u.email`).

**Flag for your awareness (not fixed, out of scope):** while tracing this I found `app/svc/main.go` does **not** register any `/api/users/search` route at all — `useProjectStore.searchUsers()` calls a route that appears to 404 at runtime. This is a pre-existing routing bug, not a typecheck issue, so I left it alone, but it's worth knowing the "add member by search" feature may be broken end-to-end.

### `User` — left unchanged (see AdminView.vue section below for why)

## 2. `src/api/client.ts`

Widened the error-shape type `apiErrorMessage` already narrows to (via the same kind of `as {...}` cast it already used — not `any`) to also cover `field`, and added a sibling `apiErrorField(e: unknown): string | undefined`. Traced the `field` property to `app/auth/internal/handler/auth.go`'s `RegisterConflictResponse{ Error string; Field string \`json:"field,omitempty"\` }`, returned on a 409 from `/auth/register` (proxied unchanged through svc's `POST /api/auth/register`). Used in `SetupView.vue` in place of the old raw `e?.data?.field`/`e?.data?.error` access on an implicitly-typed catch variable.

## 3. Untyped `ref()`/timer/event fixes (root cause 2)

All per the task's guidance, cross-checked against real usage:

- **`ProjectView.vue`**: `members: ref<ProjectMember[]>([])`, `searchResults: ref<UserSearchResult[]>([])`, `searchTimer: ReturnType<typeof setTimeout> | null`. Also normalized `projectId` from `computed(() => route.params.projectId)` (type `string | string[]`) to `computed(() => String(route.params.projectId))` — this single change resolved ~9 of the 81 baseline errors that were all "`string | string[]` not assignable to `string`" at every one of the many `projectStore.*(projectId.value, ...)` call sites in this file. `String(route.params.projectId)` matches an existing precedent in this codebase (`GameView.vue` uses the identical coercion for the same `projectId` route param).
- **`EntryEditor.vue`**: `entry: ref<ContentEntry | null>(null)` (imported `ContentEntry` from `../stores/useEntryStore`, the canonical shape already used correctly elsewhere in the file via `entryStore.get()`'s return type), `localTags: ref<string[]>([])`, `saveTimer: ReturnType<typeof setTimeout> | null`, `tiptapRef: ref<InstanceType<typeof TiptapEditor> | null>(null)` (confirmed `TiptapEditor.vue`'s `defineExpose({ editor })` at line 83 is the only thing it exposes). Also added a missing `if (!entry.value) return` guard in `confirmDelete()`, which read `entry.value.title`/`.id` without a null-check (the other four places that read `entry.value.*` already guarded — this one didn't, and once `entry` had a real type instead of `never`, that gap became a real possibly-null error rather than being silently masked).
- **`App.vue`**: `shiftTimer: ReturnType<typeof setTimeout> | null`; `onKeydown(e: KeyboardEvent)` — traced to `window.addEventListener('keydown', onKeydown)`, the native DOM event type for a `keydown` listener.
- **`AdminView.vue`**: `userTimer`/`projectTimer: ReturnType<typeof setTimeout> | null`, `expandedProject: ref<string | null>(null)` (needed once `toggleProjectMembers`'s `id` param got a real `string` type — assigning a real string into a `Ref<null>` only became a visible error at that point), `toggleProjectMembers(id: string)`, `confirmDeleteUser(u: User)`.
- **`DashboardView.vue`**: `pendingInvite`/`activeInviteProject: ref<string | null>(null)`; normalized `route.query.invite` with the same `typeof raw === 'string'` idiom already established in `LoginView.vue`/`ResetPasswordView.vue`; added param types `Project` to `isEditorOf`, `confirmDeleteProject`, `openProjectInvite`, and `[string,string]`/`string` to `displayRole`; added a `if (!token) return` guard in `redeemInvite()` since `pendingInvite.value` can be `null`.
- **`SetupView.vue`**: `inviteToken: ref<string | null>(null)`, normalized `route.query.invite` the same way, replaced the raw `e?.data?.field`/`e?.data?.error` access with `apiErrorField(e)`/`apiErrorMessage(e, ...)`.

### `setTimeout`/`clearTimeout` typing gotcha

Typing every timer as `ReturnType<typeof setTimeout> | null` (as instructed) surfaced a follow-on error at every unconditional `clearTimeout(timer)` call: this repo's `@types/node` makes `ReturnType<typeof setTimeout>` resolve to Node's `Timeout` type, and neither of `clearTimeout`'s two overloads (Node's `Timeout | undefined` or DOM's `number | undefined`) accepts `null`. `DashboardView.vue` already had this exact pattern solved (`searchDebounceTimer`) by guarding with `if (timer) clearTimeout(timer)` instead of calling it unconditionally — I applied that same established guard everywhere I introduced a nullable timer (`App.vue`, `EntryEditor.vue` ×2, `AdminView.vue` ×2, `ProjectView.vue`).

## 4. `AdminView.vue` — `User.id` (deviation from the task's suggested fallback)

The task's diagnosis suggested: if `id` isn't always present on admin's user rows, "switch this specific call to `u.sub`... always present." I traced both endpoints instead of assuming:

- `admin.setUserActive`/`admin.users` are backed by auth-svc's `UserResponse` (`ListUsers`, `UpdateUser` in `app/auth/internal/handler/auth.go`) — `{ id, email, username, role, active, createdAt }`. **`id` is always present here; `sub` does not exist on this struct at all.**
- `auth.user` (the *other* consumer of the shared `User` type) is backed by svc's `MeResponse` (`GET /api/auth/me`, `app/svc/internal/handler/auth.go`) — `{ sub, email, role, active }`. **`sub` is always present here; `id` does not exist on this struct at all.**

So `u.sub` would actually have been `undefined` for every row in `admin.users` at runtime — following the suggested fallback literally would have silently broken the admin enable/disable button (passed `undefined` as the id to a `string`-typed param) while still type-checking cleanly, which is worse than the original bug. Neither "make `id` required" nor "switch to `sub`" is honestly correct for the *shared* `User` type, since the two real backend shapes disagree on which identifier field exists.

I left `User.id?: string` and `User.sub: string` as they already were (both already correctly optional/required for their respective real shapes) and instead added a small guard at the two call sites that iterate `admin.users` (`toggleUserActive(u: User)`, `confirmDeleteUser(u: User)`), each doing `if (!u.id) return` before using it. In practice this branch is never taken (every row from `UserResponse` has `id`), but it's the honest expression of what the shared type can actually guarantee, without lying about `User.id`'s general optionality or introducing a field that isn't there.

## Files changed

- `app/ui/src/types/index.ts`
- `app/ui/src/api/client.ts`
- `app/ui/src/stores/useProjectStore.ts`
- `app/ui/src/App.vue`
- `app/ui/src/components/EntryEditor.vue`
- `app/ui/src/views/ProjectView.vue`
- `app/ui/src/views/AdminView.vue`
- `app/ui/src/views/DashboardView.vue`
- `app/ui/src/views/SetupView.vue`

## Verification run

```
$ cd app/ui && npx vue-tsc --noEmit
(0 errors)

$ cd app/ui && npm run build
✓ built in 1.49s

$ cd app/ui && npx vitest run
Test Files  5 passed (5)
     Tests  52 passed (52)
```
