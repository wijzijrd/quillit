<template>
  <AuthLayout title="Create account">
    <div
      v-if="inviteToken"
      class="rounded-lg border border-[var(--secondary)] bg-[color-mix(in_srgb,var(--primary)_10%,var(--muted))] px-3.5 py-2.5 text-sm text-[var(--foreground)]"
    >
      You've been invited to join a project. Create an account to continue.
    </div>

    <form class="flex flex-col gap-5" @submit.prevent="submit">
      <FormField label="Email" for="setup-email" required>
        <Input id="setup-email" v-model="email" type="email" autocomplete="email" placeholder="you@example.com" />
      </FormField>
      <FormField
        label="Username"
        for="setup-username"
        required
        ref="usernameField"
        :model-value="username"
        :validate="checkUsernameAvailable"
        :debounce-ms="400"
      >
        <Input id="setup-username" v-model="username" type="text" autocomplete="username" placeholder="dungeon_master" required aria-required="true" />
      </FormField>
      <FormField label="Password" for="setup-password" required>
        <Input id="setup-password" v-model="password" type="password" autocomplete="new-password" />
      </FormField>
      <FormField label="Confirm password" for="setup-confirm" required>
        <Input id="setup-confirm" v-model="confirm" type="password" autocomplete="new-password" />
      </FormField>
      <p v-if="emailTaken" class="text-sm text-[var(--destructive)]">
        This email is already registered.
        <router-link class="text-[var(--primary)] hover:underline" :to="{ path: '/login', query: { email } }">Sign in instead?</router-link>
      </p>
      <p v-else-if="error" class="text-sm text-[var(--destructive)]">{{ error }}</p>
      <Button type="submit" :disabled="loading" class="mt-1 w-full">
        {{ loading ? 'Creating account…' : 'Create account' }}
      </Button>
    </form>
    <p class="text-center text-sm text-[var(--muted-foreground)]">
      Already have an account?
      <router-link class="text-[var(--primary)] hover:underline" to="/login">Sign in</router-link>
    </p>
  </AuthLayout>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '../stores/useAuthStore'
import { useProjectStore } from '../stores/useProjectStore'
import AuthLayout from '../layouts/AuthLayout.vue'
import { Input } from '../components/ui/input'
import { FormField } from '../components/ui/form-field'
import type { ValidationResult } from '../components/ui/form-field'
import { Button } from '../components/ui/button'
import { api } from '../api/client'

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()
const projects = useProjectStore()

const email = ref('')
const username = ref('')
const password = ref('')
const confirm = ref('')
const error = ref('')
const emailTaken = ref(false)
const loading = ref(false)
const inviteToken = ref(null)

const usernameField = ref<{ validateNow: (v: string | number) => Promise<ValidationResult | null> } | null>(null)

async function checkUsernameAvailable(value: string | number): Promise<ValidationResult | null> {
  const value_ = String(value)
  if (value_.length < 3) return null
  try {
    const { available } = await api<{ available: boolean }>('/auth/users/available', { query: { username: value_ } })
    return available
      ? { message: 'Username is available', type: 'success' }
      : { message: 'Username is already taken', type: 'error' }
  } catch {
    return { message: "Couldn't check username right now", type: 'error' }
  }
}

onMounted(async () => {
  inviteToken.value = route.query.invite ?? null
  await auth.fetchMe()

  if (auth.isLoggedIn) {
    if (inviteToken.value) {
      // Already logged in — redeem the invite and go straight to the project.
      try {
        const membership = await projects.join(inviteToken.value)
        router.push(`/projects/${membership.projectId}/entries`)
      } catch {
        router.push('/')
      }
    } else {
      router.push('/')
    }
  }
})

async function submit() {
  error.value = ''
  emailTaken.value = false
  if (!email.value || !username.value || !password.value) {
    error.value = 'All fields are required'
    return
  }
  if (password.value.length < 8) {
    error.value = 'Password must be at least 8 characters'
    return
  }
  if (password.value !== confirm.value) {
    error.value = 'Passwords do not match'
    return
  }
  const usernameCheck = await usernameField.value?.validateNow(username.value)
  if (usernameCheck?.type === 'error') {
    error.value = usernameCheck.message
    return
  }
  loading.value = true
  try {
    await auth.register(email.value, username.value, password.value)
    if (inviteToken.value) {
      try {
        const membership = await projects.join(inviteToken.value)
        router.push(`/projects/${membership.projectId}/entries`)
        return
      } catch {
        // Join failed — navigate home; user can join manually
      }
    }
    router.push('/')
  } catch (e) {
    if (e?.data?.field === 'email') {
      emailTaken.value = true
    } else {
      error.value = e?.data?.error ?? 'Registration failed'
    }
  } finally {
    loading.value = false
  }
}
</script>
