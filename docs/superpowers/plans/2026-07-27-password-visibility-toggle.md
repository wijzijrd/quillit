# Password Visibility Toggle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an eye icon to password input fields that toggles the field between masked (`type="password"`) and plain-text (`type="text"`) display.

**Architecture:** New `PasswordInput` component under `ui/src/components/ui/password-input/`, built from the existing (currently unused) `InputGroup` / `InputGroupInput` / `InputGroupButton` shadcn-vue primitives. Drop-in replacement for `<Input type="password">` with the same `v-model` contract. Swapped into `LoginView.vue` and `SetupView.vue` (2 fields).

**Tech Stack:** Vue 3 `<script setup lang="ts">`, `lucide-vue-next` (`Eye`/`EyeOff` icons), existing shadcn-vue `input-group` primitives, `@vueuse/core` `useVModel`.

## Global Constraints

- No test framework in `ui/` (no vitest/jest configured) — verification is `npx vue-tsc --noEmit` (from `ui/`) plus manual browser check via `npm run dev`.
- Follow existing component pattern exactly: see `ui/src/components/ui/input/Input.vue` + `ui/src/components/ui/input/index.ts`.
- Props/emits use the codebase convention: `defineProps<{...}>()` with no `withDefaults` unless needed; `defineEmits<{...}>()`.
- Do not touch `svc/` or `auth/` — this is UI-only per `CLAUDE.md`.

---

### Task 1: Create the `PasswordInput` component

**Files:**
- Create: `ui/src/components/ui/password-input/PasswordInput.vue`
- Create: `ui/src/components/ui/password-input/index.ts`

**Interfaces:**
- Produces: `PasswordInput` component, props `{ defaultValue?: string | number, modelValue?: string | number, class?: HTMLAttributes["class"] }`, emits `update:modelValue`. Renders a single `<input>` (via `InputGroupInput`) that other attrs (`id`, `autocomplete`, `placeholder`, `aria-invalid`) fall through to via Vue's automatic attribute inheritance (do not set `inheritAttrs: false`).

- [ ] **Step 1: Write the component**

```vue
<!-- ui/src/components/ui/password-input/PasswordInput.vue -->
<script setup lang="ts">
import type { HTMLAttributes } from "vue"
import { ref } from "vue"
import { Eye, EyeOff } from "lucide-vue-next"
import { useVModel } from "@vueuse/core"
import { cn } from "@/lib/utils"
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from "@/components/ui/input-group"

const props = defineProps<{
  defaultValue?: string | number
  modelValue?: string | number
  class?: HTMLAttributes["class"]
}>()

const emits = defineEmits<{
  (e: "update:modelValue", payload: string | number): void
}>()

const modelValue = useVModel(props, "modelValue", emits, {
  passive: true,
  defaultValue: props.defaultValue,
})

const visible = ref(false)
</script>

<template>
  <InputGroup :class="props.class">
    <InputGroupInput
      v-model="modelValue"
      :type="visible ? 'text' : 'password'"
    />
    <InputGroupAddon align="inline-end">
      <InputGroupButton
        type="button"
        size="icon-xs"
        aria-label="Toggle password visibility"
        @click="visible = !visible"
      >
        <EyeOff v-if="visible" class="size-4" />
        <Eye v-else class="size-4" />
      </InputGroupButton>
    </InputGroupAddon>
  </InputGroup>
</template>
```

- [ ] **Step 2: Write the barrel export**

```ts
// ui/src/components/ui/password-input/index.ts
export { default as PasswordInput } from "./PasswordInput.vue"
```

- [ ] **Step 3: Typecheck**

Run (from `ui/`): `npx vue-tsc --noEmit`
Expected: no new errors introduced by these two files (compare against the ~285 pre-existing baseline error count — do not fix unrelated pre-existing errors).

- [ ] **Step 4: Commit**

```bash
git add ui/src/components/ui/password-input/PasswordInput.vue ui/src/components/ui/password-input/index.ts
git commit -m "feat(ui): add PasswordInput component with visibility toggle"
```

---

### Task 2: Use `PasswordInput` in `LoginView.vue`

**Files:**
- Modify: `ui/src/views/LoginView.vue:9-11` (password field block), `ui/src/views/LoginView.vue:29` (import)

**Interfaces:**
- Consumes: `PasswordInput` from Task 1 (`import { PasswordInput } from '../components/ui/password-input'`), same `v-model` contract as `Input`.

- [ ] **Step 1: Swap the import**

In `ui/src/views/LoginView.vue`, change:

```ts
import { Input } from '../components/ui/input'
```

to:

```ts
import { Input } from '../components/ui/input'
import { PasswordInput } from '../components/ui/password-input'
```

- [ ] **Step 2: Swap the password field**

Change:

```html
<Input id="login-password" v-model="password" type="password" autocomplete="current-password" />
```

to:

```html
<PasswordInput id="login-password" v-model="password" autocomplete="current-password" />
```

- [ ] **Step 3: Manual verification**

Run `npm run dev` (from `ui/`), open `/login`, confirm: password field renders with eye icon, clicking it reveals plain text, clicking again re-masks, form still submits with correct value.

- [ ] **Step 4: Typecheck**

Run: `npx vue-tsc --noEmit` — expect no new errors.

- [ ] **Step 5: Commit**

```bash
git add ui/src/views/LoginView.vue
git commit -m "feat(ui): use PasswordInput on login form"
```

---

### Task 3: Use `PasswordInput` in `SetupView.vue`

**Files:**
- Modify: `ui/src/views/SetupView.vue:20-22` (password field), `ui/src/views/SetupView.vue:23-26` (confirm field), `ui/src/views/SetupView.vue:45` (import)

**Interfaces:**
- Consumes: same `PasswordInput` component from Task 1.

- [ ] **Step 1: Swap the import**

In `ui/src/views/SetupView.vue`, change:

```ts
import { Input } from '../components/ui/input'
```

to:

```ts
import { Input } from '../components/ui/input'
import { PasswordInput } from '../components/ui/password-input'
```

- [ ] **Step 2: Swap both password fields**

Change:

```html
<div class="flex flex-col gap-2">
  <label class="auth-label" for="setup-password">Password</label>
  <Input id="setup-password" v-model="password" type="password" autocomplete="new-password" />
</div>
<div class="flex flex-col gap-2">
  <label class="auth-label" for="setup-confirm">Confirm password</label>
  <Input id="setup-confirm" v-model="confirm" type="password" autocomplete="new-password" />
</div>
```

to:

```html
<div class="flex flex-col gap-2">
  <label class="auth-label" for="setup-password">Password</label>
  <PasswordInput id="setup-password" v-model="password" autocomplete="new-password" />
</div>
<div class="flex flex-col gap-2">
  <label class="auth-label" for="setup-confirm">Confirm password</label>
  <PasswordInput id="setup-confirm" v-model="confirm" autocomplete="new-password" />
</div>
```

- [ ] **Step 3: Manual verification**

Run `npm run dev` (from `ui/`), open `/register`, confirm: both password fields have independent eye toggles (toggling one does not affect the other), form validation (length check, match check) still works, account creation still succeeds.

- [ ] **Step 4: Typecheck**

Run: `npx vue-tsc --noEmit` — expect no new errors.

- [ ] **Step 5: Commit**

```bash
git add ui/src/views/SetupView.vue
git commit -m "feat(ui): use PasswordInput on registration form"
```

---

### Task 4: Open PR

**Files:** none (git/GitHub operation only)

- [ ] **Step 1: Push branch**

```bash
git push -u origin feat/password-visibility-toggle
```

- [ ] **Step 2: Create PR**

```bash
gh pr create --title "feat(ui): password visibility toggle" --body "$(cat <<'EOF'
## Summary
- Adds a reusable `PasswordInput` component (eye icon toggles masked/plain text) built on the existing InputGroup primitives
- Swaps `Input type="password"` for `PasswordInput` on Login (1 field) and Setup/Register (2 fields)

## Test plan
- [ ] `npx vue-tsc --noEmit` shows no new errors
- [ ] `/login` password field toggles visibility correctly
- [ ] `/register` password + confirm fields toggle independently
- [ ] Login and registration still submit/validate correctly
EOF
)"
```
