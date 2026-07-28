# FormField (required-asterisk label + pluggable validation) — Design

## Problem

Auth form labels (`LoginView.vue`, `SetupView.vue`) are plain `<label class="auth-label">` tags with no shared component. Two related needs:

1. A "required" asterisk after a label, toggleable per-field.
2. A uniform error/success message row under a field, driven by a validation function the caller supplies — needed now for the asterisk, and reused unmodified by a later feature (debounced username-availability check) so that feature doesn't have to touch this component again.

Scope: this design covers only the shared wrapper component and its migration into the 6 existing auth-form fields (Login: email, password; Setup: email, username, password, confirm). It does not implement the username-availability check itself — that is a separate, later feature that will consume this component's `validate` contract.

## Component: `FormField`

**Location:** `ui/src/components/ui/form-field/FormField.vue` + `ui/src/components/ui/form-field/index.ts`, following the existing pattern of `ui/src/components/ui/input/` and `ui/src/components/ui/password-input/`.

**Props:**

```ts
interface ValidationResult {
  message: string
  type: 'error' | 'success'
}

type Validate = (value: string | number) =>
  | ValidationResult | null | undefined
  | Promise<ValidationResult | null | undefined>

{
  label?: string
  for?: string                // forwarded to the rendered <label for>
  required?: boolean          // renders a destructive-colored " *" after the label text
  modelValue?: string | number  // presence + `validate` together enables automatic live validation
  validate?: Validate
  debounceMs?: number         // debounces the automatic validate call; ignored by validateNow()
}
```

**Slot:** default slot renders the actual input element (`Input`, `PasswordInput`, etc.) between the label and the message row. `FormField` does not own the input's `v-model` — the parent binds that directly on the slotted input, and separately passes the same value as `modelValue` to `FormField` only if automatic live validation is wanted.

**Exposed:** `defineExpose({ validateNow })` where `validateNow(value: string | number): Promise<ValidationResult | null | undefined>` runs `validate(value)` immediately (no debounce), updates the rendered message the same way the automatic path does, and returns the result so a submit handler can decide whether to block submission.

**Behavior:**

- If `label` is set, render `<label :for="props.for" class="auth-label">{{ label }}<span v-if="required" class="text-[var(--destructive)]"> *</span></label>`.
- Internal `touched` ref (default `false`) and `result` ref<`ValidationResult | null`> (default `null`).
- A `watch` on `() => props.modelValue` is registered only when `props.validate` is also set. On the first change after mount, set `touched = true`. On every change once touched, call `runValidate(newValue)`, debounced by `debounceMs` (via a simple `setTimeout`/`clearTimeout` pair) if `debounceMs` is set and > 0, otherwise called immediately.
- `runValidate(value)` calls `validate(value)`, awaits it (works for both sync and async return), and stores the resolved value (or `null`) into `result`.
- `validateNow(value)` calls `runValidate(value)` directly, bypassing debounce, and also sets `touched = true` (so any later automatic watch renders consistently). Returns the resolved result.
- Render: `<p v-if="touched && result" class="text-xs" :class="result.type === 'success' ? 'text-[var(--primary)]' : 'text-[var(--destructive)]'">{{ result.message }}</p>`.
- No new CSS color tokens: `error` → `--destructive`, `success` → `--primary`. These already exist in all 4 seasonal themes.

**Non-goals for this component:** it does not perform any HTTP calls, does not know about usernames or auth, and does not block form submission itself — callers that need submit-blocking behavior call `validateNow` themselves and inspect the returned `type`.

## Migration

Replace all 6 occurrences of this pattern in `LoginView.vue` and `SetupView.vue`:

```html
<div class="flex flex-col gap-2">
  <label class="auth-label" for="X">Label text</label>
  <Input id="X" v-model="ref" ... />
</div>
```

with:

```html
<FormField label="Label text" for="X" required>
  <Input id="X" v-model="ref" ... />
</FormField>
```

`required` is set on all 6 fields (all are mandatory for login/registration). None of the 6 pass `modelValue`, `validate`, or `debounceMs` in this change — those props exist and are typed, but this migration doesn't exercise them. `FormField`'s own top-level wrapper `<div class="flex flex-col gap-2">` replaces the hand-written one in each usage site.

## Testing

No test framework exists in `ui/` (documented project constraint). Verification is:
- `npx vue-tsc --noEmit` from `ui/` — no new errors.
- Manual/live browser check (Playwright, as used in the password-toggle feature) confirming: all 6 labels render with a trailing asterisk, `label for` still correctly focuses the paired input on click, and the message row does not render (since no field passes `validate` in this change — there is nothing to show).

## Self-review notes

- No placeholders/TBDs.
- No contradiction between the "automatic watch" and "validateNow" paths — they're independent triggers into the same `runValidate` function, and both write to the same `result`/`touched` state so rendering is consistent regardless of which path fired.
- Scope is deliberately narrow: this PR ships the component and label/required migration only. The `validate`/debounce contract is built and typed now so a later username-check feature can consume it without changing `FormField` again — but implementing that check is explicitly out of scope here.
