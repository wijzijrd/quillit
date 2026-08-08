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
      <div class="settings-row settings-row-stack">
        <div class="settings-label-group">
          <span class="settings-label">Season</span>
          <span class="settings-desc">Sets the palette and backdrop on all your devices.</span>
        </div>
        <div class="season-picker" role="radiogroup" aria-label="Season">
          <button
            v-for="s in seasons"
            :key="s"
            class="season-option"
            :class="{ active: ui.season === s }"
            role="radio"
            :aria-checked="ui.season === s"
            @click="ui.setSeason(s)"
          >
            <span class="season-chip" :data-chip="s" />
            <span class="season-name">{{ s }}</span>
          </button>
        </div>
      </div>
      <div class="settings-row">
        <div class="settings-label-group">
          <span class="settings-label">Glass</span>
          <span class="settings-desc">Frosted panels that let the backdrop shine through.</span>
        </div>
        <button
          class="theme-toggle"
          :class="{ active: ui.glass }"
          :aria-checked="ui.glass"
          role="switch"
          @click="ui.setGlass(!ui.glass)"
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
import type { Season } from '../types'

const seasons: readonly Season[] = ['spring', 'summer', 'autumn', 'winter']
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
.settings-row-stack { flex-direction: column; align-items: flex-start; gap: 12px; }

.season-picker { display: flex; gap: 8px; flex-wrap: wrap; }
.season-option {
  display: flex; align-items: center; gap: 8px;
  padding: 6px 14px 6px 8px;
  background: var(--muted);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  color: var(--foreground);
  font-family: var(--font-body);
  font-size: 0.85em;
  text-transform: capitalize;
  cursor: pointer;
  transition: border-color var(--transition), background var(--transition);
}
.season-option:hover { border-color: var(--primary); }
.season-option.active {
  border-color: var(--primary);
  background: color-mix(in srgb, var(--primary) 12%, transparent);
}
.season-chip { width: 18px; height: 18px; border-radius: 50%; border: 1px solid var(--border); }
.season-chip[data-chip='spring'] { background: linear-gradient(160deg, #EEF4E0, #A8C89A); }
.season-chip[data-chip='summer'] { background: linear-gradient(165deg, #FDF3DA, #F4DFAC 55%, #CFE3E8); }
.season-chip[data-chip='autumn'] { background: linear-gradient(160deg, #2A1E13, #6B4020); }
.season-chip[data-chip='winter'] { background: linear-gradient(165deg, #0F1420, #29405C); }
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
