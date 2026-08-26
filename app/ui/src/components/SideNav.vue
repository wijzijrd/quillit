<template>
  <nav class="app-shell-sidebar flex flex-col overflow-hidden h-full">
    <div class="flex items-center gap-2 border-b border-[var(--border)] mb-1 px-3" style="height: var(--h-xl)">
      <span style="font-size:1.3em; color: var(--primary)">᚛</span>
      <span class="brand-label">Quillit</span>
    </div>

    <div class="px-2 mb-2">
      <RouterLink to="/" class="nav-item" active-class="nav-active" title="Dashboard">
        <LayoutDashboard :size="16" class="flex-shrink-0" />
        <span class="nav-label">Dashboard</span>
      </RouterLink>
      <RouterLink to="/entries" class="nav-item" active-class="nav-active" title="Entries">
        <BookOpen :size="16" class="flex-shrink-0" />
        <span class="nav-label">Entries</span>
      </RouterLink>
    </div>

    <div class="mt-auto border-t border-[var(--border)] p-2">
      <RouterLink
        v-if="auth.user?.role === 'admin'"
        to="/admin"
        class="nav-item"
        active-class="nav-active"
        title="Admin"
      >
        <Settings :size="16" class="flex-shrink-0" />
        <span class="nav-label">Admin</span>
      </RouterLink>
      <RouterLink to="/profile" class="nav-item" active-class="nav-active" title="Profile">
        <UserCircle :size="16" class="flex-shrink-0" />
        <span class="nav-label">Profile</span>
      </RouterLink>
    </div>
  </nav>
</template>

<script setup lang="ts">
import { RouterLink } from 'vue-router'
import { LayoutDashboard, BookOpen, UserCircle, Settings } from 'lucide-vue-next'
import { useAuthStore } from '../stores/useAuthStore'

const auth = useAuthStore()
</script>

<style scoped>
.brand-label {
  font-family: var(--font-display);
  font-size: 0.95em;
  letter-spacing: 0.04em;
  color: var(--foreground);
}
.nav-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 10px;
  height: var(--h-md);
  border-radius: var(--radius);
  color: var(--muted-foreground);
  text-decoration: none;
  transition: background var(--transition), color var(--transition);
  cursor: pointer;
  background: none;
  border: none;
  width: 100%;
  font-family: var(--font-body);
  font-size: 0.88em;
}
.nav-label { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.nav-item:hover {
  background: var(--sidebar-accent);
  color: var(--sidebar-accent-foreground);
}
.nav-active {
  background: var(--sidebar-primary);
  color: var(--sidebar-primary-foreground);
}
</style>
