<!-- ui/src/views/ForgotPasswordView.vue -->
<template>
  <AuthLayout title="Reset your password">
    <form v-if="!submitted" class="flex flex-col gap-5" @submit.prevent="submit">
      <FormField label="Email" for="forgot-email" required>
        <Input id="forgot-email" v-model="email" type="email" autocomplete="email" placeholder="you@example.com" />
      </FormField>
      <p v-if="error" class="text-sm text-[var(--destructive)]">{{ error }}</p>
      <Button type="submit" :disabled="loading" class="mt-1 w-full">
        {{ loading ? 'Sending…' : 'Send reset link' }}
      </Button>
    </form>
    <p v-else class="text-sm text-[var(--foreground)]">
      If that email is registered, a reset link has been sent. Check your inbox.
    </p>
    <p class="text-center text-sm text-[var(--muted-foreground)]">
      <router-link class="text-[var(--primary)] hover:underline" :to="{ path: '/login', query: { email } }">Back to sign in</router-link>
    </p>
  </AuthLayout>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import AuthLayout from '../layouts/AuthLayout.vue'
import { Input } from '../components/ui/input'
import { FormField } from '../components/ui/form-field'
import { Button } from '../components/ui/button'
import { api, apiErrorMessage } from '../api/client'

const email = ref('')
const error = ref('')
const loading = ref(false)
const submitted = ref(false)

async function submit() {
  if (!email.value) return
  error.value = ''
  loading.value = true
  try {
    await api('/auth/forgot-password', { method: 'POST', body: { email: email.value } })
    submitted.value = true
  } catch (e) {
    error.value = apiErrorMessage(e, 'Something went wrong. Please try again.')
  } finally {
    loading.value = false
  }
}
</script>
