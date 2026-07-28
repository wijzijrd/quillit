<template>
  <AuthLayout title="Sign in">
    <form class="flex flex-col gap-5" @submit.prevent="submit">
      <div class="flex flex-col gap-2">
        <label class="auth-label" for="login-email">Email</label>
        <Input id="login-email" v-model="email" type="email" autocomplete="email" placeholder="you@example.com" />
      </div>
      <div class="flex flex-col gap-2">
        <label class="auth-label" for="login-password">Password</label>
        <PasswordInput id="login-password" v-model="password" autocomplete="current-password" />
      </div>
      <p v-if="error" class="text-sm text-[var(--destructive)]">{{ error }}</p>
      <Button type="submit" :disabled="loading" class="mt-1 w-full">
        {{ loading ? 'Signing in…' : 'Sign in' }}
      </Button>
    </form>
    <p class="text-center text-sm text-[var(--muted-foreground)]">
      No account?
      <router-link class="text-[var(--primary)] hover:underline" to="/register">Create one</router-link>
    </p>
  </AuthLayout>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '../stores/useAuthStore'
import AuthLayout from '../layouts/AuthLayout.vue'
import { Input } from '../components/ui/input'
import { PasswordInput } from '../components/ui/password-input'
import { Button } from '../components/ui/button'

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()
const email = ref('')
const password = ref('')
const error = ref('')
const loading = ref(false)

async function submit() {
  if (!email.value || !password.value) return
  loading.value = true
  error.value = ''
  try {
    await auth.login(email.value, password.value)
    const dest = route.query.redirect
    const safe = typeof dest === 'string' && dest.startsWith('/') && !dest.startsWith('//') ? dest : '/'
    router.push(safe)
  } catch {
    error.value = 'Invalid credentials'
  } finally {
    loading.value = false
  }
}
</script>
