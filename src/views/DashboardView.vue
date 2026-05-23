<template>
  <div class="dashboard">
    <header class="dash-header">
      <div class="dash-title-row">
        <div>
          <h1>Quillit</h1>
          <p class="dash-sub">Your campaign, organised.</p>
        </div>
        <div class="dash-actions">
          <button class="btn-icon" @click="exportCampaign" title="Export campaign data as JSON">
            <Download :size="18" />
          </button>
          <label class="btn-icon" title="Import from a backup file">
            <Upload :size="18" />
            <input ref="importInputRef" type="file" accept=".json" @change="handleImport" hidden />
          </label>
        </div>
      </div>

      <div class="dash-search">
        <Search :size="15" class="dash-search-icon" />
        <input
          class="dash-search-input"
          v-model="searchQuery"
          placeholder="Search entries…"
          @keydown.escape="searchQuery = ''"
        />
      </div>
    </header>

    <!-- Category tabs strip -->
    <div class="cat-tabs">
      <button
        v-for="cat in cats.categories"
        :key="cat.id"
        class="cat-tab"
        :class="{ active: activeTab === cat.name }"
        :style="activeTab === cat.name ? { color: cat.color, background: hexToAlpha(cat.color, 0.1) } : {}"
        @click="activeTab = cat.name"
      >
        <component :is="resolveIcon(cat.icon)" :size="14" />
        <span class="cat-tab-name">{{ cat.name }}</span>
        <span class="cat-tab-count">{{ entries.byCategory(cat.name).length }}</span>
      </button>
    </div>

    <!-- Tag chips for the active category -->
    <div class="cat-tag-panel" v-if="activeTab">
      <RouterLink
        v-for="{ tag, count } in tagsForActiveTab"
        :key="tag"
        :to="`/tag/${encodeURIComponent(tag)}`"
        class="tag-pill"
        :style="activeTabColor
          ? { color: activeTabColor, background: hexToAlpha(activeTabColor, 0.12), borderColor: hexToAlpha(activeTabColor, 0.3) }
          : {}"
      >
        <span class="tp-label">{{ tag }}</span>
        <span class="tp-count">{{ count }}</span>
      </RouterLink>
      <span class="tag-pill tag-pill--untagged" v-if="untaggedCount > 0">
        <span class="tp-label">Untagged</span>
        <span class="tp-count">{{ untaggedCount }}</span>
      </span>
      <span class="tag-pill-empty" v-if="tagsForActiveTab.length === 0 && untaggedCount === 0">
        No entries in this category yet.
      </span>
    </div>

    <section class="recent-section">
      <h2>{{ searchResults ? 'Search Results' : 'Recently Updated' }}</h2>
      <div class="recent-list">
        <template v-if="searchResults">
          <RouterLink
            v-for="entry in searchResults"
            :key="entry.id"
            :to="`/quillit/${entry.id}`"
            class="recent-item"
            @click="ui.setActiveEntry(entry.id)"
          >
            <span class="ri-cat" :style="{ color: cats.categoryFor(entry.category)?.color ?? 'var(--text-muted)' }">
              {{ entry.category }}
            </span>
            <span class="ri-title">{{ entry.title }}</span>
          </RouterLink>
          <p v-if="searchResults.length === 0" class="no-recent">No entries match your search.</p>
        </template>
        <template v-else>
          <RouterLink
            v-for="entry in recent"
            :key="entry.id"
            :to="`/quillit/${entry.id}`"
            class="recent-item"
            @click="ui.setActiveEntry(entry.id)"
          >
            <span class="ri-cat" :style="{ color: cats.categoryFor(entry.category)?.color ?? 'var(--text-muted)' }">
              {{ entry.category }}
            </span>
            <span class="ri-title">{{ entry.title }}</span>
          </RouterLink>
          <p v-if="recent.length === 0" class="no-recent">
            No entries yet — head to <RouterLink to="/notes">Notes</RouterLink> to create one.
          </p>
        </template>
      </div>
    </section>

    <p v-if="importError" class="import-error">{{ importError }}</p>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { hexToAlpha } from '../utils/color.js'
import { RouterLink } from 'vue-router'
import { Download, Upload, Search } from 'lucide-vue-next'
import { useEntriesStore } from '../stores/useEntriesStore.js'
import { useCategoriesStore } from '../stores/useCategoriesStore.js'
import { resolveIcon } from '../utils/categoryIcons.js'
import { useAnnotationsStore } from '../stores/useAnnotationsStore.js'
import { useCampaignStore } from '../stores/useCampaignStore.js'
import { useUIStore } from '../stores/useUIStore.js'
import { api } from '../api/client.js'

const entries = useEntriesStore()
const cats = useCategoriesStore()
const annotations = useAnnotationsStore()
const campaign = useCampaignStore()
const ui = useUIStore()

const activeTab = ref(null)
const importInputRef = ref(null)
const importError = ref('')
const searchQuery = ref('')

onMounted(async () => {
  await entries.init()
  await cats.init()
  if (!activeTab.value && cats.categories.length > 0) {
    activeTab.value = cats.categories[0].name
  }
})

const tagsForActiveTab = computed(() => {
  if (!activeTab.value) return []
  const counts = {}
  for (const e of entries.byCategory(activeTab.value)) {
    for (const tag of (e.tags ?? [])) {
      counts[tag] = (counts[tag] ?? 0) + 1
    }
  }
  return Object.entries(counts)
    .map(([tag, count]) => ({ tag, count }))
    .sort((a, b) => b.count - a.count)
})

const activeTabColor = computed(() => cats.categoryFor(activeTab.value)?.color ?? null)

const untaggedCount = computed(() => {
  if (!activeTab.value) return 0
  return entries.byCategory(activeTab.value)
    .filter(e => !e.tags || e.tags.length === 0).length
})

const recent = computed(() =>
  [...entries.entries].sort((a, b) => b.updatedAt - a.updatedAt).slice(0, 8)
)

const searchResults = computed(() =>
  searchQuery.value.trim() ? entries.search(searchQuery.value).slice(0, 12) : null
)


function exportCampaign() {
  const data = {
    version: 1,
    exportedAt: Date.now(),
    entries: entries.entries,
    annotations: annotations.annotations,
    campaigns: campaign.campaigns,
  }
  const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `${(campaign.name || 'campaign').replace(/\s+/g, '-').toLowerCase()}-export.json`
  a.click()
  URL.revokeObjectURL(url)
}

async function handleImport(e) {
  importError.value = ''
  const file = e.target.files?.[0]
  if (!file) return
  try {
    const text = await file.text()
    const data = JSON.parse(text)
    if (data.version !== 1) { importError.value = 'Unrecognised export format.'; return }
    if (!confirm('This will replace all current data. Continue?')) return
    const result = await api('/migrate/import', { method: 'POST', body: data })
    const c = result.imported
    alert(`Imported: ${c.entries} entries, ${c.annotations} annotations, ${c.campaigns} campaigns, ${c.players} players`)
    window.location.reload()
  } catch {
    importError.value = 'Failed to import — the file may be corrupted.'
  } finally {
    e.target.value = ''
  }
}
</script>

<style scoped>
.dashboard { padding: 40px 48px; max-width: 860px; margin: 0 auto; }
.dash-header { margin-bottom: 0; }
.dash-title-row { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; }
.dash-header h1 { font-family: var(--font-display); font-size: 2em; color: var(--accent); letter-spacing: 0.06em; }
.dash-sub { color: var(--text-muted); margin-top: 4px; }
.dash-actions { display: flex; gap: 8px; align-items: center; padding-top: 6px; }
.btn-icon {
  display: inline-flex; align-items: center; justify-content: center;
  width: 32px; height: 32px;
  background: var(--bg-raised);
  border: 1px solid var(--border-light);
  border-radius: var(--radius);
  color: var(--text-muted); cursor: pointer;
  transition: background var(--transition), color var(--transition);
}
.btn-icon:hover { background: var(--bg-hover); color: var(--text-primary); }

.dash-search {
  position: relative; display: flex; align-items: center;
  gap: 8px; margin-top: 16px; margin-bottom: 28px;
  background: var(--bg-raised); border: 1px solid var(--border-light);
  border-radius: var(--radius); padding: 6px 12px;
}
.dash-search-icon { color: var(--text-faint); flex-shrink: 0; }
.dash-search-input {
  flex: 1; background: none; border: none; outline: none;
  color: var(--text-primary); font-family: var(--font-body); font-size: 0.9em;
}
.dash-search-input::placeholder { color: var(--text-faint); }

.cat-tabs {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
  border-bottom: 1px solid var(--border);
  margin-bottom: 16px;
}
.cat-tab {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 14px;
  border: none;
  border-bottom: 2px solid transparent;
  background: none;
  color: var(--text-muted);
  font-size: 0.85em;
  font-family: var(--font-body);
  cursor: pointer;
  border-radius: var(--radius) var(--radius) 0 0;
  transition: color var(--transition), border-color var(--transition);
  margin-bottom: -1px;
}
.cat-tab:hover { color: var(--text-primary); }
.cat-tab.active {
  border-bottom-color: currentColor;
  font-weight: 600;
}
.cat-tab-count {
  font-size: 0.8em;
  color: var(--text-faint);
  background: var(--bg-raised);
  border-radius: 10px;
  padding: 1px 6px;
}
.cat-tab.active .cat-tab-count {
  color: inherit;
  background: color-mix(in srgb, currentColor 18%, transparent);
}

.cat-tag-panel {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  min-height: 48px;
  margin-bottom: 32px;
  align-items: flex-start;
}
.tag-pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 5px 12px;
  background: var(--bg-raised);
  border: 1px solid var(--border-light);
  border-radius: 20px;
  text-decoration: none;
  color: var(--text-primary);
  font-size: 0.85em;
  transition: background var(--transition), border-color var(--transition);
  cursor: pointer;
}
.tag-pill:hover { background: var(--bg-hover); border-color: var(--accent); }
.tp-count {
  font-size: 0.8em;
  background: var(--accent-dim);
  color: var(--accent);
  border-radius: 10px;
  padding: 1px 6px;
  font-weight: 600;
}
.tag-pill--untagged { opacity: 0.6; cursor: default; }
.tag-pill-empty { color: var(--text-faint); font-size: 0.88em; padding: 8px 0; }

.recent-section h2 { font-family: var(--font-display); font-size: 0.9em; letter-spacing: 0.08em; color: var(--text-muted); text-transform: uppercase; margin-bottom: 12px; }
.recent-list { display: flex; flex-direction: column; gap: 2px; }
.recent-item {
  display: flex; align-items: center; gap: 14px;
  padding: 10px 14px; border-radius: var(--radius);
  text-decoration: none; transition: background var(--transition);
}
.recent-item:hover { background: var(--bg-raised); }
.ri-cat { font-size: 0.72em; text-transform: uppercase; letter-spacing: 0.1em; font-weight: 600; width: 64px; flex-shrink: 0; }
.ri-title { color: var(--text-primary); font-size: 0.95em; }
.no-recent { color: var(--text-faint); font-size: 0.9em; padding: 8px 14px; }
.no-recent a { color: var(--accent); }
.import-error { margin-top: 12px; color: var(--danger); font-size: 0.88em; }
</style>
