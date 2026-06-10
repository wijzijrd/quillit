<template>
  <div class="auth-shell">
    <div class="auth-card">
      <div class="auth-brand">᚛ Quillit</div>
      <h1 class="auth-title">Create account</h1>

      <div class="invite-notice" v-if="inviteToken">
        You've been invited to join a project. Create an account to continue.
      </div>

      <form class="auth-form" @submit.prevent="submit">
        <div class="auth-field">
          <label class="auth-label">Email</label>
          <input class="auth-input" v-model="email" type="email" autocomplete="email" placeholder="you@example.com" />
        </div>
        <div class="auth-field">
          <label class="auth-label">Username</label>
          <input class="auth-input" v-model="username" type="text" autocomplete="username" placeholder="dungeon_master" />
        </div>
        <div class="auth-field">
          <label class="auth-label">Password</label>
          <input class="auth-input" v-model="password" type="password" autocomplete="new-password" />
        </div>
        <div class="auth-field">
          <label class="auth-label">Confirm password</label>
          <input class="auth-input" v-model="confirm" type="password" autocomplete="new-password" />
        </div>
        <p class="auth-error" v-if="error">{{ error }}</p>
        <button class="auth-btn" type="submit" :disabled="loading">
          {{ loading ? 'Creating account…' : 'Create account' }}
        </button>
      </form>
      <p class="auth-footer">
        Already have an account? <router-link class="auth-link" to="/login">Sign in</router-link>
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '../stores/useAuthStore'
import { useProjectStore } from '../stores/useProjectStore'

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

<style scoped>
.auth-shell {
  display: flex; align-items: center; justify-content: center;
  min-height: 100vh; background: var(--background);
}
.auth-card {
  width: 360px; background: var(--card);
  border: 1px solid var(--border); border-radius: calc(var(--radius) * 2);
  padding: var(--space-3xl) var(--space-2xl);
  display: flex; flex-direction: column; gap: var(--space-lg);
}
.auth-brand { font-family: var(--font-display); color: var(--primary); font-size: var(--text-md); letter-spacing: 0.08em; }
.auth-title { font-family: var(--font-display); font-size: var(--text-2xl); color: var(--foreground); font-weight: 400; margin: 0; }
.invite-notice {
  background: color-mix(in srgb, var(--primary) 10%, var(--muted));
  border: 1px solid var(--secondary); border-radius: var(--radius);
  padding: 10px 14px; font-size: var(--text-sm); color: var(--foreground);
}
.auth-form { display: flex; flex-direction: column; gap: var(--space-md); }
.auth-field { display: flex; flex-direction: column; gap: 4px; }
.auth-label { font-size: var(--text-xs); text-transform: uppercase; letter-spacing: 0.08em; color: var(--muted-foreground); }
.auth-input {
  background: var(--muted); border: 1px solid var(--border);
  border-radius: var(--radius); color: var(--foreground);
  font-family: var(--font-body); font-size: var(--text-md);
  height: var(--h-md); padding: 0 var(--space-sm); outline: none;
  transition: border-color var(--transition);
}
.auth-input:focus { border-color: var(--secondary); }
.auth-error { font-size: var(--text-sm); color: var(--destructive); margin: 0; }
.auth-btn {
  height: var(--h-md); background: var(--secondary); border: none;
  border-radius: var(--radius); color: var(--primary);
  font-family: var(--font-body); font-size: var(--text-md); cursor: pointer;
  transition: background var(--transition); margin-top: var(--space-xs);
}
.auth-btn:hover { background: var(--primary); color: var(--background); }
.auth-btn:disabled { opacity: 0.5; cursor: default; }
.auth-footer { font-size: var(--text-sm); color: var(--muted-foreground); margin: 0; text-align: center; }
.auth-link { color: var(--primary); text-decoration: none; }
.auth-link:hover { text-decoration: underline; }
</style>
