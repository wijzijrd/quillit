<template>
  <nav class="sidenav">
    <div class="brand">
      <span class="brand-icon">᚛</span>
      <span class="brand-name">Quillit</span>
    </div>

    <div class="nav-section">
      <RouterLink to="/" class="nav-item" active-class="active">
        <LayoutDashboard :size="16" class="nav-icon" />
        <span class="nav-label">Dashboard</span>
      </RouterLink>
      <RouterLink to="/notes" class="nav-item" active-class="active">
        <BookOpen :size="16" class="nav-icon" />
        <span class="nav-label">Notes</span>
      </RouterLink>
      <RouterLink to="/member" class="nav-item" active-class="active">
        <BookMarked :size="16" class="nav-icon" />
        <span class="nav-label">Member</span>
      </RouterLink>
    </div>

    <div v-if="inProject" class="nav-section nav-section--cats">
      <button
        v-for="cat in cats.projectCategories"
        :key="cat.id"
        class="nav-item cat-item"
        :class="{ active: ui.activeCategory === cat.name }"
        :title="`${cat.name} (${entries.byCategory(cat.name).length})`"
        @click="ui.setCategory(ui.activeCategory === cat.name ? null : cat.name)"
      >
        <component :is="resolveIcon(cat.icon)" :size="16" :style="{ color: cat.color }" />
        <span class="cat-count">{{ entries.byCategory(cat.name).length }}</span>
      </button>
    </div>

    <div class="nav-bottom">
      <RouterLink
        v-if="auth.user?.role === 'admin'"
        to="/admin"
        class="nav-item"
        active-class="active"
      >
        <Settings :size="16" class="nav-icon" />
        <span class="nav-label">Admin</span>
      </RouterLink>
      <RouterLink to="/profile" class="nav-item" active-class="active">
        <UserCircle :size="16" class="nav-icon" />
        <span class="nav-label">Profile</span>
      </RouterLink>
    </div>
  </nav>
</template>

<script setup>
import { computed } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { LayoutDashboard, BookOpen, BookMarked, UserCircle, Settings } from 'lucide-vue-next'
import { useUIStore } from '../stores/useUIStore.js'
import { useEntriesStore } from '../stores/useEntriesStore.js'
import { useCategoriesStore } from '../stores/useCategoriesStore.js'
import { useAuthStore } from '../stores/useAuthStore.js'
import { resolveIcon } from '../utils/categoryIcons.js'

const ui = useUIStore()
const entries = useEntriesStore()
const cats = useCategoriesStore()
const auth = useAuthStore()

const route = useRoute()
const inProject = computed(() => !!route.params.projectId)
</script>

<style scoped>
.sidenav {
  background: var(--bg-surface);
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0;
  overflow: hidden;
}
.brand {
  display: flex;
  align-items: center;
  gap: var(--space-sm);
  padding: var(--space-xl) var(--space-lg) var(--space-lg);
  border-bottom: 1px solid var(--border);
  margin-bottom: var(--space-sm);
  height: var(--h-xl);
}
.brand-icon { font-size: 1.3em; color: var(--accent); }
.brand-name { font-family: var(--font-display); font-size: var(--text-md); letter-spacing: 0.08em; color: var(--accent); }
.nav-section { padding: var(--space-xs) var(--space-sm); margin-bottom: var(--space-sm); }
.nav-section--cats { border-top: 1px solid var(--border); padding-top: var(--space-sm); }
.nav-item {
  display: flex;
  align-items: center;
  gap: var(--space-sm);
  padding: 0 var(--space-sm);
  height: var(--h-md);
  border-radius: var(--radius);
  color: var(--text-muted);
  text-decoration: none;
  font-size: var(--text-md);
  transition: background var(--transition), color var(--transition);
  cursor: pointer;
  background: none;
  border: none;
  width: 100%;
  text-align: left;
}
.nav-item:hover, .nav-item.active {
  background: var(--bg-hover);
  color: var(--text-primary);
}
.nav-icon { flex-shrink: 0; }
.cat-count { margin-left: auto; font-size: var(--text-xs); color: var(--text-faint); }

.nav-bottom {
  margin-top: auto;
  border-top: 1px solid var(--border);
  padding: var(--space-sm);
}
</style>
