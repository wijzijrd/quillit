<!-- ui/src/views/ResetPasswordView.vue -->
<template>
  <AuthLayout title="Set a new password">
    <p v-if="!token" class="text-sm text-[var(--destructive)]">
      This reset link is invalid or missing.
      <router-link class="text-[var(--primary)] hover:underline" to="/forgot-password">Request a new one</router-link>
    </p>
    <form v-else-if="!submitted" class="flex flex-col gap-5" @submit.prevent="submit">
      <FormField label="New password" for="reset-password" required>
        <Input id="reset-password" v-model="password" type="password" autocomplete="new-password" />
      </FormField>
      <FormField label="Confirm new password" for="reset-confirm" required>
        <Input id="reset-confirm" v-model="confirm" type="password" autocomplete="new-password" />
      </FormField>
      <p v-if="error" class="text-sm text-[var(--destructive)]">{{ error }}</p>
      <Button type="submit" :disabled="loading" class="mt-1 w-full">
        {{ loading ? 'Resetting…' : 'Reset password' }}
      </Button>
    </form>
    <p v-else class="text-sm text-[var(--foreground)]">
      Your password has been reset.
      <router-link class="text-[var(--primary)] hover:underline" to="/login">Sign in</router-link>
    </p>
  </AuthLayout>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import AuthLayout from '../layouts/AuthLayout.vue'
import { Input } from '../components/ui/input'
import { FormField } from '../components/ui/form-field'
import { Button } from '../components/ui/button'
import { api, apiErrorMessage } from '../api/client'

const route = useRoute()

const token = ref('')
const password = ref('')
const confirm = ref('')
const error = ref('')
const loading = ref(false)
const submitted = ref(false)

onMounted(() => {
  const raw = route.query.token
  if (typeof raw === 'string') {
    token.value = raw
  }
})

async function submit() {
  error.value = ''
  if (!password.value) return
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
    await api('/auth/reset-password', { method: 'POST', body: { token: token.value, newPassword: password.value } })
    submitted.value = true
  } catch (e) {
    error.value = apiErrorMessage(e, 'Something went wrong. Please try again.')
  } finally {
    loading.value = false
  }
}
</script>
