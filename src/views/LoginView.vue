<template>
  <div class="auth-shell">
    <div class="auth-card">
      <div class="auth-brand">᚛ Quillit</div>
      <h1 class="auth-title">Welcome back</h1>
      <form class="auth-form" @submit.prevent="submit">
        <div class="auth-field">
          <label class="auth-label">Email</label>
          <input class="auth-input" v-model="email" type="email" autocomplete="email" placeholder="you@example.com" />
        </div>
        <div class="auth-field">
          <label class="auth-label">Password</label>
          <input class="auth-input" v-model="password" type="password" autocomplete="current-password" />
        </div>
        <p class="auth-error" v-if="error">{{ error }}</p>
        <button class="auth-btn" type="submit" :disabled="loading">
          {{ loading ? 'Signing in…' : 'Sign in' }}
        </button>
      </form>
      <p class="auth-footer">
        No account? <router-link class="auth-link" to="/register">Create one</router-link>
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '../stores/useAuthStore'

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

<style scoped>
.auth-shell {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  background: var(--background);
}
.auth-card {
  width: 360px;
  background: var(--card);
  border: 1px solid var(--border);
  border-radius: calc(var(--radius) * 2);
  padding: var(--space-3xl) var(--space-2xl);
  display: flex;
  flex-direction: column;
  gap: var(--space-lg);
}
.auth-brand {
  font-family: var(--font-display);
  color: var(--primary);
  font-size: var(--text-md);
  letter-spacing: 0.08em;
}
.auth-title {
  font-family: var(--font-display);
  font-size: var(--text-2xl);
  color: var(--foreground);
  font-weight: 400;
  margin: 0;
}
.auth-form { display: flex; flex-direction: column; gap: var(--space-md); }
.auth-field { display: flex; flex-direction: column; gap: 4px; }
.auth-label { font-size: var(--text-xs); text-transform: uppercase; letter-spacing: 0.08em; color: var(--muted-foreground); }
.auth-input {
  background: var(--muted);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  color: var(--foreground);
  font-family: var(--font-body);
  font-size: var(--text-md);
  height: var(--h-md);
  padding: 0 var(--space-sm);
  outline: none;
  transition: border-color var(--transition);
}
.auth-input:focus { border-color: var(--secondary); }
.auth-error { font-size: var(--text-sm); color: var(--destructive); margin: 0; }
.auth-btn {
  height: var(--h-md);
  background: var(--secondary);
  border: none;
  border-radius: var(--radius);
  color: var(--primary);
  font-family: var(--font-body);
  font-size: var(--text-md);
  cursor: pointer;
  transition: background var(--transition);
  margin-top: var(--space-xs);
}
.auth-btn:hover { background: var(--primary); color: var(--background); }
.auth-btn:disabled { opacity: 0.5; cursor: default; }
.auth-footer { font-size: var(--text-sm); color: var(--muted-foreground); margin: 0; text-align: center; }
.auth-link { color: var(--primary); text-decoration: none; }
.auth-link:hover { text-decoration: underline; }
</style>
