# FormField Required-Asterisk Labels Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a reusable `FormField` component that renders a label with a toggleable required asterisk and a validation-message row, then migrate the 6 auth-form label/input pairs in `LoginView.vue` and `SetupView.vue` to use it.

**Architecture:** New `FormField` component under `ui/src/components/ui/form-field/`, following the existing `input/` and `password-input/` component pattern. It wraps a slotted input with a label (asterisk optional) and a message row driven by an optional `validate` function — either called automatically (watching `modelValue`, optionally debounced) or triggered manually via an exposed `validateNow(value)` method for submit-time-only checks. This PR wires the label/required parts into 6 existing fields; none of those 6 pass `validate` yet — that plumbing ships now for a later feature (debounced username-availability check) to consume without further changes to `FormField`.

**Tech Stack:** Vue 3 `<script setup lang="ts">`, existing `cn()` helper from `@/lib/utils`, no new dependencies.

## Global Constraints

- No test framework in `ui/` (no vitest/jest configured) — verification is `npx vue-tsc --noEmit` (from `ui/`) plus manual/live browser check via `npm run dev` (or Playwright if installed — install+revert the devDependency the same way as the password-toggle feature: `npm install -D playwright` + `npx playwright install chromium`, then `git checkout -- ui/package.json ui/package-lock.json` after verifying, since it's not a real project dependency).
- Follow existing component pattern exactly: see `ui/src/components/ui/input/Input.vue` + `ui/src/components/ui/input/index.ts` for the props/emits/file-layout convention.
- No new CSS color tokens: reuse `--destructive` for error messages and `--primary` for success messages (already defined in all 4 seasonal themes in `ui/src/assets/main.css`).
- Do not touch `svc/` or `auth/` — this is UI-only per `CLAUDE.md`.
- Full design reference: `docs/superpowers/specs/2026-07-28-form-field-required-label-design.md`.

---

### Task 1: Create the `FormField` component

**Files:**
- Create: `ui/src/components/ui/form-field/FormField.vue`
- Create: `ui/src/components/ui/form-field/index.ts`

**Interfaces:**
- Produces: `FormField` component and exported types `ValidationResult` (`{ message: string; type: 'error' | 'success' }`) and `Validate` (`(value: string | number) => ValidationResult | null | undefined | Promise<ValidationResult | null | undefined>`). Props: `label?: string`, `for?: string`, `required?: boolean`, `modelValue?: string | number`, `validate?: Validate`, `debounceMs?: number`, `class?: HTMLAttributes["class"]`. Exposes `validateNow(value: string | number): Promise<ValidationResult | null>` via `defineExpose`. Default slot renders the wrapped input.

- [ ] **Step 1: Write the component**

```vue
<!-- ui/src/components/ui/form-field/FormField.vue -->
<script setup lang="ts">
import type { HTMLAttributes } from "vue"
import type { Validate, ValidationResult } from "."
import { onUnmounted, ref, watch } from "vue"
import { cn } from "@/lib/utils"

const props = defineProps<{
  label?: string
  for?: string
  required?: boolean
  modelValue?: string | number
  validate?: Validate
  debounceMs?: number
  class?: HTMLAttributes["class"]
}>()

const touched = ref(false)
const result = ref<ValidationResult | null>(null)
let debounceTimer: ReturnType<typeof setTimeout> | undefined

async function runValidate(value: string | number) {
  if (!props.validate) return
  result.value = (await props.validate(value)) ?? null
}

async function validateNow(value: string | number) {
  touched.value = true
  if (debounceTimer) clearTimeout(debounceTimer)
  await runValidate(value)
  return result.value
}

defineExpose({ validateNow })

watch(
  () => props.modelValue,
  (newValue) => {
    if (!props.validate || newValue === undefined) return
    touched.value = true
    if (debounceTimer) clearTimeout(debounceTimer)
    if (props.debounceMs && props.debounceMs > 0) {
      debounceTimer = setTimeout(() => runValidate(newValue), props.debounceMs)
    } else {
      runValidate(newValue)
    }
  },
)

onUnmounted(() => {
  if (debounceTimer) clearTimeout(debounceTimer)
})
</script>

<template>
  <div :class="cn('flex flex-col gap-2', props.class)">
    <label v-if="label" :for="props.for" class="auth-label">
      {{ label }}<span v-if="required" class="text-[var(--destructive)]"> *</span>
    </label>
    <slot />
    <p
      v-if="touched && result"
      class="text-xs"
      :class="result.type === 'success' ? 'text-[var(--primary)]' : 'text-[var(--destructive)]'"
    >
      {{ result.message }}
    </p>
  </div>
</template>
```

- [ ] **Step 2: Write the barrel export (with the shared types — write this file BEFORE `FormField.vue` imports from it, since `FormField.vue` does `import type { Validate, ValidationResult } from "."`)**

```ts
// ui/src/components/ui/form-field/index.ts
export { default as FormField } from "./FormField.vue"

export interface ValidationResult {
  message: string
  type: "error" | "success"
}

export type Validate = (
  value: string | number
) => ValidationResult | null | undefined | Promise<ValidationResult | null | undefined>
```

- [ ] **Step 3: Typecheck**

Run (from `ui/`): `npx vue-tsc --noEmit`
Expected: no new errors from these two files. Note the current total error count for comparison in later tasks (it will fluctuate slightly run-to-run due to pre-existing unrelated errors — what matters is that no error references `form-field/`).

- [ ] **Step 4: Commit**

```bash
git add ui/src/components/ui/form-field/FormField.vue ui/src/components/ui/form-field/index.ts
git commit -m "feat(ui): add FormField component with required asterisk and pluggable validation"
```

---

### Task 2: Migrate `LoginView.vue` to `FormField`

**Files:**
- Modify: `ui/src/views/LoginView.vue` (both field blocks, lines 4-11, and the import block, lines 24-30)

**Interfaces:**
- Consumes: `FormField` from Task 1 (`import { FormField } from '../components/ui/form-field'`). Neither field passes `validate`/`modelValue`/`debounceMs` — only `label`, `for`, `required`.

- [ ] **Step 1: Add the import**

In `ui/src/views/LoginView.vue`, change:

```ts
import { Input } from '../components/ui/input'
import { Button } from '../components/ui/button'
```

to:

```ts
import { Input } from '../components/ui/input'
import { FormField } from '../components/ui/form-field'
import { Button } from '../components/ui/button'
```

- [ ] **Step 2: Replace both field blocks**

Change:

```html
<div class="flex flex-col gap-2">
  <label class="auth-label" for="login-email">Email</label>
  <Input id="login-email" v-model="email" type="email" autocomplete="email" placeholder="you@example.com" />
</div>
<div class="flex flex-col gap-2">
  <label class="auth-label" for="login-password">Password</label>
  <Input id="login-password" v-model="password" type="password" autocomplete="current-password" />
</div>
```

to:

```html
<FormField label="Email" for="login-email" required>
  <Input id="login-email" v-model="email" type="email" autocomplete="email" placeholder="you@example.com" />
</FormField>
<FormField label="Password" for="login-password" required>
  <Input id="login-password" v-model="password" type="password" autocomplete="current-password" />
</FormField>
```

- [ ] **Step 3: Manual verification**

Run `npm run dev` (from `ui/`), open `/login`, confirm: both labels ("Email", "Password") render followed by a red/destructive-colored asterisk, clicking each label still focuses its paired input (the `for`/`id` association survived the refactor), and no message row appears under either field (since neither passes `validate`). Confirm the form still submits and shows "Invalid credentials" on a bad login.

- [ ] **Step 4: Typecheck**

Run: `npx vue-tsc --noEmit` — expect no new errors referencing `LoginView.vue` or `form-field/`.

- [ ] **Step 5: Commit**

```bash
git add ui/src/views/LoginView.vue
git commit -m "feat(ui): use FormField on login form"
```

---

### Task 3: Migrate `SetupView.vue` to `FormField`

**Files:**
- Modify: `ui/src/views/SetupView.vue` (all 4 field blocks, lines 11-26, and the import block, lines 39-46)

**Interfaces:**
- Consumes: same `FormField` component from Task 1. All 4 fields pass only `label`, `for`, `required`.

- [ ] **Step 1: Add the import**

In `ui/src/views/SetupView.vue`, change:

```ts
import { Input } from '../components/ui/input'
import { Button } from '../components/ui/button'
```

to:

```ts
import { Input } from '../components/ui/input'
import { FormField } from '../components/ui/form-field'
import { Button } from '../components/ui/button'
```

- [ ] **Step 2: Replace all 4 field blocks**

Change:

```html
<div class="flex flex-col gap-2">
  <label class="auth-label" for="setup-email">Email</label>
  <Input id="setup-email" v-model="email" type="email" autocomplete="email" placeholder="you@example.com" />
</div>
<div class="flex flex-col gap-2">
  <label class="auth-label" for="setup-username">Username</label>
  <Input id="setup-username" v-model="username" type="text" autocomplete="username" placeholder="dungeon_master" />
</div>
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
<FormField label="Email" for="setup-email" required>
  <Input id="setup-email" v-model="email" type="email" autocomplete="email" placeholder="you@example.com" />
</FormField>
<FormField label="Username" for="setup-username" required>
  <Input id="setup-username" v-model="username" type="text" autocomplete="username" placeholder="dungeon_master" />
</FormField>
<FormField label="Password" for="setup-password" required>
  <Input id="setup-password" v-model="password" type="password" autocomplete="new-password" />
</FormField>
<FormField label="Confirm password" for="setup-confirm" required>
  <Input id="setup-confirm" v-model="confirm" type="password" autocomplete="new-password" />
</FormField>
```

- [ ] **Step 3: Manual verification**

Run `npm run dev` (from `ui/`), open `/register`, confirm: all 4 labels ("Email", "Username", "Password", "Confirm password") render with a trailing asterisk, each label still focuses its paired input on click, no message rows appear, and the existing submit-time validation (empty fields, password length, password mismatch) still works exactly as before — `FormField` does not intercept or change `submit()`'s existing checks.

- [ ] **Step 4: Typecheck**

Run: `npx vue-tsc --noEmit` — expect no new errors referencing `SetupView.vue` or `form-field/`.

- [ ] **Step 5: Commit**

```bash
git add ui/src/views/SetupView.vue
git commit -m "feat(ui): use FormField on registration form"
```

---

### Task 4: Open PR

**Files:** none (git/GitHub operation only)

- [ ] **Step 1: Push branch**

```bash
git push -u origin feat/required-field-labels
```

- [ ] **Step 2: Create PR**

```bash
gh pr create --title "feat(ui): required-field asterisk labels" --body "$(cat <<'EOF'
## Summary
- Adds a reusable `FormField` component: label with a toggleable required asterisk, plus a message row driven by an optional `validate` function (sync or async, with optional debounce, or an imperative `validateNow` for submit-time-only checks)
- Migrates all 6 auth-form label/input pairs (Login: email/password; Register: email/username/password/confirm) to `FormField` with `required` set on all 6
- `validate`/`modelValue`/`debounceMs` are not exercised by this PR — they exist for a later debounced username-availability check to consume without further changes to `FormField`

## Test plan
- [ ] `npx vue-tsc --noEmit` shows no new errors
- [ ] `/login` and `/register` labels all show a trailing asterisk
- [ ] Clicking a label still focuses its paired input
- [ ] No message row appears under any field (none pass `validate` yet)
- [ ] Existing submit-time validation on both forms still works unchanged
EOF
)"
```
