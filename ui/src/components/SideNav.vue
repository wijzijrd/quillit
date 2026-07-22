<template>
  <nav class="app-shell-sidebar flex flex-col overflow-hidden h-full">
    <div class="flex items-center justify-center border-b border-[var(--border)] mb-1" style="height: var(--h-xl)">
      <span style="font-size:1.3em; color: var(--primary)">᚛</span>
    </div>

    <div class="px-1 mb-2">
      <RouterLink to="/" class="nav-item" active-class="nav-active" title="Dashboard">
        <LayoutDashboard :size="16" class="flex-shrink-0" />
      </RouterLink>
      <RouterLink to="/notes" class="nav-item" active-class="nav-active" title="Notes">
        <BookOpen :size="16" class="flex-shrink-0" />
      </RouterLink>
      <RouterLink to="/member" class="nav-item" active-class="nav-active" title="Member">
        <BookMarked :size="16" class="flex-shrink-0" />
      </RouterLink>
    </div>

    <div v-if="inProject" class="px-1 mb-2 border-t border-[var(--border)] pt-2">
      <button
        v-for="cat in cats.projectCategories"
        :key="cat.id"
        class="nav-item"
        :class="{ 'nav-active': ui.activeCategory === cat.name }"
        :title="cat.name"
        @click="ui.setCategory(ui.activeCategory === cat.name ? null : cat.name)"
      >
        <component :is="resolveIcon(cat.icon)" :size="16" :style="{ color: cat.color }" />
      </button>
    </div>

    <div class="mt-auto border-t border-[var(--border)] p-1">
      <RouterLink
        v-if="auth.user?.role === 'admin'"
        to="/admin"
        class="nav-item"
        active-class="nav-active"
        title="Admin"
      >
        <Settings :size="16" class="flex-shrink-0" />
      </RouterLink>
      <RouterLink to="/friends" class="nav-item" active-class="nav-active" title="Friends">
        <Users :size="16" class="flex-shrink-0" />
      </RouterLink>
      <RouterLink to="/profile" class="nav-item" active-class="nav-active" title="Profile">
        <UserCircle :size="16" class="flex-shrink-0" />
      </RouterLink>
    </div>
  </nav>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { LayoutDashboard, BookOpen, BookMarked, UserCircle, Users, Settings } from 'lucide-vue-next'
import { useUIStore } from '../stores/useUIStore'
import { useEntriesStore } from '../stores/useEntriesStore'
import { useCategoriesStore } from '../stores/useCategoriesStore'
import { useAuthStore } from '../stores/useAuthStore'
import { resolveIcon } from '../utils/categoryIcons'

const ui = useUIStore()
const entries = useEntriesStore()
const cats = useCategoriesStore()
const auth = useAuthStore()

const route = useRoute()
const inProject = computed(() => !!route.params.projectId)
</script>

<style scoped>
.nav-item {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
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
}
.nav-item:hover, .nav-active {
  background: var(--muted);
  color: var(--foreground);
}
</style>
