<template>
  <AuthLayout title="Create account">
    <div
      v-if="inviteToken"
      class="rounded-lg border border-[var(--secondary)] bg-[color-mix(in_srgb,var(--primary)_10%,var(--muted))] px-3.5 py-2.5 text-sm text-[var(--foreground)]"
    >
      You've been invited to join a project. Create an account to continue.
    </div>

    <form class="flex flex-col gap-5" @submit.prevent="submit">
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
      <p v-if="error" class="text-sm text-[var(--destructive)]">{{ error }}</p>
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
import { Button } from '../components/ui/button'

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()
const projects = useProjectStore()

const email = ref('')
const username = ref('')
const password = ref('')
const confirm = ref('')
const error = ref('')
const loading = ref(false)
const inviteToken = ref(null)

onMounted(async () => {
  inviteToken.value = route.query.invite ?? null
  await auth.fetchMe()

  if (auth.isLoggedIn) {
    if (inviteToken.value) {
      // Already logged in — redeem the invite and go straight to the project.
      try {
        const membership = await projects.join(inviteToken.value)
        router.push(`/projects/${membership.projectId}/notes`)
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
  loading.value = true
  try {
    await auth.register(email.value, username.value, password.value)
    if (inviteToken.value) {
      try {
        const membership = await projects.join(inviteToken.value)
        router.push(`/projects/${membership.projectId}/notes`)
        return
      } catch {
        // Join failed — navigate home; user can join manually
      }
    }
    router.push('/')
  } catch (e) {
    error.value = e?.data?.error ?? 'Registration failed'
  } finally {
    loading.value = false
  }
}
</script>
