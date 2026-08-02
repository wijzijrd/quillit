<template>
  <AuthLayout title="Sign in">
    <form class="flex flex-col gap-5" @submit.prevent="submit">
      <FormField label="Email" for="login-email" required>
        <Input id="login-email" v-model="email" type="email" autocomplete="email" placeholder="you@example.com" />
      </FormField>
      <FormField label="Password" for="login-password" required>
        <Input id="login-password" v-model="password" type="password" autocomplete="current-password" />
      </FormField>
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
import { ref, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '../stores/useAuthStore'
import AuthLayout from '../layouts/AuthLayout.vue'
import { Input } from '../components/ui/input'
import { FormField } from '../components/ui/form-field'
import { Button } from '../components/ui/button'

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()
const email = ref('')
const password = ref('')
const error = ref('')
const loading = ref(false)

onMounted(() => {
  const prefill = route.query.email
  if (typeof prefill === 'string') {
    email.value = prefill
  }
})

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
