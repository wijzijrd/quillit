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
      <div class="settings-row">
        <div class="settings-label-group">
          <span class="settings-label">Dark mode</span>
          <span class="settings-desc">Switch between parchment and midnight.</span>
        </div>
        <button
          class="theme-toggle"
          :class="{ active: ui.theme === 'dark' }"
          :aria-checked="ui.theme === 'dark'"
          role="switch"
          @click="ui.toggleTheme()"
        >
          <span class="theme-toggle-thumb" />
        </button>
      </div>
    </section>

    <button class="btn-logout" @click="logout">Log out</button>
  </div>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/useAuthStore'
import { useUIStore } from '../stores/useUIStore'

const auth = useAuthStore()
const ui = useUIStore()
const router = useRouter()

async function logout() {
  await auth.logout()
  window.location.assign('/login')
}
</script>

<style scoped>
.profile { padding: 40px 48px; }
.profile-header { margin-bottom: 32px; }
.profile-header h1 { font-family: var(--font-display); font-size: 2em; color: var(--primary); letter-spacing: 0.06em; }
.profile-sub { color: var(--muted-foreground); margin-top: 4px; }

.profile-card {
  background: var(--card);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 20px 24px;
  margin-bottom: 16px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.profile-row { display: flex; align-items: center; gap: 24px; }
.profile-label { font-size: 0.8em; text-transform: uppercase; letter-spacing: 0.1em; color: var(--muted-foreground); width: 60px; flex-shrink: 0; }
.profile-value { color: var(--foreground); font-size: 0.95em; }

.role-badge {
  font-size: 0.75em; font-weight: 600; text-transform: uppercase; letter-spacing: 0.08em;
  padding: 2px 10px; border-radius: 99px;
  background: var(--muted); color: var(--muted-foreground); border: 1px solid var(--border);
}
.role-badge.admin { background: color-mix(in srgb, var(--primary) 15%, transparent); color: var(--primary); border-color: var(--secondary); }
.role-badge.gm { background: color-mix(in srgb, var(--cat-faction) 15%, transparent); color: var(--cat-faction); border-color: var(--cat-faction); }

.settings-row { display: flex; align-items: center; justify-content: space-between; gap: 24px; }
.settings-label-group { display: flex; flex-direction: column; gap: 2px; }
.settings-label { font-size: 0.88em; font-weight: 600; color: var(--foreground); }
.settings-desc { font-size: 0.8em; color: var(--muted-foreground); }

.theme-toggle {
  position: relative;
  width: 40px;
  height: 22px;
  border-radius: 99px;
  background: var(--muted);
  border: 1px solid var(--border);
  cursor: pointer;
  flex-shrink: 0;
  transition: background var(--transition), border-color var(--transition);
}
.theme-toggle.active { background: var(--primary); border-color: var(--primary); }
.theme-toggle-thumb {
  position: absolute;
  top: 2px;
  left: 2px;
  width: 16px;
  height: 16px;
  border-radius: 50%;
  background: var(--popover);
  transition: transform var(--transition);
}
.theme-toggle.active .theme-toggle-thumb { transform: translateX(18px); }

.btn-logout {
  margin-top: 8px;
  background: none;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  color: var(--muted-foreground);
  font-family: var(--font-body);
  font-size: 0.88em;
  padding: 7px 20px;
  cursor: pointer;
  transition: background var(--transition), color var(--transition), border-color var(--transition);
}
.btn-logout:hover { background: color-mix(in srgb, var(--destructive) 10%, transparent); color: var(--destructive); border-color: var(--destructive); }
</style>
