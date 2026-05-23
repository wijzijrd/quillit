<template>
  <div class="profile">
    <header class="profile-header">
      <h1>Profile</h1>
      <p class="profile-sub">Account details and settings.</p>
    </header>

    <section class="profile-card">
      <div class="profile-row">
        <span class="profile-label">Email</span>
        <span class="profile-value">{{ auth.user?.email ?? '—' }}</span>
      </div>
      <div class="profile-row">
        <span class="profile-label">Role</span>
        <span class="role-badge" :class="auth.user?.role">{{ auth.user?.role ?? '—' }}</span>
      </div>
    </section>

    <section class="profile-card settings-card">
      <p class="settings-hint">More settings coming soon.</p>
    </section>

    <button class="btn-logout" @click="logout">Log out</button>
  </div>
</template>

<script setup>
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/useAuthStore.js'

const auth = useAuthStore()
const router = useRouter()

async function logout() {
  await auth.logout()
  router.push('/login')
}
</script>

<style scoped>
.profile { padding: 40px 48px; max-width: 560px; }
.profile-header { margin-bottom: 32px; }
.profile-header h1 { font-family: var(--font-display); font-size: 2em; color: var(--accent); letter-spacing: 0.06em; }
.profile-sub { color: var(--text-muted); margin-top: 4px; }

.profile-card {
  background: var(--bg-surface);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 20px 24px;
  margin-bottom: 16px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.profile-row { display: flex; align-items: center; gap: 24px; }
.profile-label { font-size: 0.8em; text-transform: uppercase; letter-spacing: 0.1em; color: var(--text-faint); width: 60px; flex-shrink: 0; }
.profile-value { color: var(--text-primary); font-size: 0.95em; }

.role-badge {
  font-size: 0.75em; font-weight: 600; text-transform: uppercase; letter-spacing: 0.08em;
  padding: 2px 10px; border-radius: 99px;
  background: var(--bg-raised); color: var(--text-muted); border: 1px solid var(--border-light);
}
.role-badge.admin { background: color-mix(in srgb, var(--accent) 15%, transparent); color: var(--accent); border-color: var(--accent-dim); }
.role-badge.gm { background: color-mix(in srgb, var(--cat-faction) 15%, transparent); color: var(--cat-faction); border-color: var(--cat-faction); }

.settings-card { opacity: 0.6; }
.settings-hint { color: var(--text-faint); font-size: 0.9em; }

.btn-logout {
  margin-top: 8px;
  background: none;
  border: 1px solid var(--border-light);
  border-radius: var(--radius);
  color: var(--text-muted);
  font-family: var(--font-body);
  font-size: 0.88em;
  padding: 7px 20px;
  cursor: pointer;
  transition: background var(--transition), color var(--transition), border-color var(--transition);
}
.btn-logout:hover { background: color-mix(in srgb, var(--danger) 10%, transparent); color: var(--danger); border-color: var(--danger); }
</style>
