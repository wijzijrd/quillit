<template>
  <div class="campaigns-view">
    <div class="cv-header">
      <h1 class="cv-title">Campaigns</h1>
      <div class="cv-add-row">
        <input
          class="cv-name-input"
          v-model="newCampaignName"
          placeholder="New campaign name…"
          @keydown.enter="addCampaign"
        />
        <button class="cv-add-btn" @click="addCampaign" :disabled="!newCampaignName.trim()">
          <Plus :size="14" /> Add
        </button>
      </div>
    </div>

    <div class="cv-list">
      <div
        class="cv-campaign"
        v-for="camp in campaign.campaigns"
        :key="camp.id"
      >
        <div class="cvc-header" @click="toggleExpand(camp.id)">
          <div class="cvc-left">
            <ChevronRight class="cvc-chevron" :class="{ expanded: expandedId === camp.id }" :size="14" />
            <span class="cvc-name" v-if="renamingId !== camp.id">{{ camp.name }}</span>
            <input
              v-else
              class="cvc-rename-input"
              v-model="renameValue"
              @keydown.enter="commitRename(camp.id)"
              @keydown.escape="renamingId = null"
              @blur="commitRename(camp.id)"
              @click.stop
            />
          </div>
          <div class="cvc-actions" @click.stop>
            <span class="cvc-player-count">{{ camp.players.length }} player{{ camp.players.length !== 1 ? 's' : '' }}</span>
            <button class="cvc-btn" @click="startRename(camp)" title="Rename campaign"><Pencil :size="13" /></button>
            <button class="cvc-btn danger" @click="removeCampaign(camp.id)" title="Delete campaign"><Trash2 :size="13" /></button>
          </div>
        </div>

        <div class="cvc-players" v-if="expandedId === camp.id">
          <div class="cvp-row" v-for="player in camp.players" :key="player.id">
            <span class="cvp-name">{{ player.name }}</span>
            <div class="cvp-actions">
              <button class="cvc-btn" @click="copyLink(player, camp)" title="Copy share link"><Copy :size="13" /></button>
              <button class="cvc-btn danger" @click="campaign.removePlayer(camp.id, player.id)" title="Remove player"><X :size="13" /></button>
            </div>
          </div>
          <div class="cvp-add-row">
            <input
              class="cvp-input"
              v-model="newPlayerNames[camp.id]"
              :placeholder="`Add player to ${camp.name}…`"
              @keydown.enter="addPlayer(camp.id)"
            />
            <button class="cvp-add-btn" @click="addPlayer(camp.id)" :disabled="!newPlayerNames[camp.id]?.trim()">
              <UserPlus :size="13" />
            </button>
          </div>
        </div>
      </div>

      <p class="cv-empty" v-if="campaign.campaigns.length === 0">No campaigns yet. Create one above.</p>
    </div>

    <div v-if="copiedLink" class="cv-toast">Link copied!</div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive } from 'vue'
import { Plus, ChevronRight, Pencil, Trash2, Copy, X, UserPlus } from 'lucide-vue-next'
import { useCampaignStore } from '../stores/useCampaignStore'

const campaign = useCampaignStore()

const newCampaignName = ref('')
const expandedId = ref(null)
const renamingId = ref(null)
const renameValue = ref('')
const newPlayerNames = reactive({})
const copiedLink = ref(false)

function addCampaign() {
  if (!newCampaignName.value.trim()) return
  campaign.addCampaign(newCampaignName.value)
  newCampaignName.value = ''
}

function toggleExpand(id) {
  expandedId.value = expandedId.value === id ? null : id
}

function startRename(camp) {
  renamingId.value = camp.id
  renameValue.value = camp.name
}

function commitRename(id) {
  if (renameValue.value.trim()) campaign.renameCampaign(id, renameValue.value)
  renamingId.value = null
}

function removeCampaign(id) {
  if (!confirm('Delete this campaign? Players will lose their share links.')) return
  campaign.removeCampaign(id)
  if (expandedId.value === id) expandedId.value = null
}

function addPlayer(campaignId) {
  const name = newPlayerNames[campaignId]?.trim()
  if (!name) return
  campaign.addPlayer(campaignId, name)
  newPlayerNames[campaignId] = ''
}

function copyLink(player, camp) {
  const url = campaign.shareUrl(player)
  navigator.clipboard.writeText(url)
  copiedLink.value = true
  setTimeout(() => { copiedLink.value = false }, 2000)
}
</script>

<style scoped>
.campaigns-view {
  padding: var(--space-2xl);
}

.cv-header { margin-bottom: var(--space-xl); }
.cv-title {
  font-family: var(--font-display);
  font-size: var(--text-2xl);
  color: var(--foreground);
  margin-bottom: var(--space-md);
}
.cv-add-row { display: flex; gap: var(--space-sm); }
.cv-name-input {
  flex: 1;
  background: var(--muted);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  color: var(--foreground);
  font-family: var(--font-body);
  font-size: var(--text-md);
  height: var(--h-md);
  padding: 0 var(--space-sm);
  outline: none;
}
.cv-name-input:focus { border-color: var(--secondary); }
.cv-add-btn {
  display: inline-flex;
  align-items: center;
  gap: var(--space-xs);
  height: var(--h-md);
  padding: 0 var(--space-md);
  background: var(--secondary);
  border: none;
  border-radius: var(--radius);
  color: var(--primary);
  font-family: var(--font-body);
  font-size: var(--text-sm);
  cursor: pointer;
}
.cv-add-btn:disabled { opacity: 0.4; cursor: default; }

.cv-list { display: flex; flex-direction: column; gap: var(--space-sm); }

.cv-campaign {
  background: var(--muted);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  overflow: hidden;
}

.cvc-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 var(--space-md);
  height: var(--h-md);
  cursor: pointer;
  transition: background var(--transition);
}
.cvc-header:hover { background: var(--muted); }

.cvc-left { display: flex; align-items: center; gap: var(--space-xs); flex: 1; min-width: 0; }
.cvc-chevron {
  color: var(--muted-foreground);
  flex-shrink: 0;
  transition: transform 180ms ease;
}
.cvc-chevron.expanded { transform: rotate(90deg); }
.cvc-name {
  font-size: var(--text-md);
  color: var(--foreground);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.cvc-rename-input {
  flex: 1;
  background: var(--card);
  border: 1px solid var(--secondary);
  border-radius: var(--radius);
  color: var(--foreground);
  font-family: var(--font-body);
  font-size: var(--text-md);
  height: var(--h-sm);
  padding: 0 var(--space-xs);
  outline: none;
}

.cvc-actions { display: flex; align-items: center; gap: var(--space-xs); flex-shrink: 0; }
.cvc-player-count { font-size: var(--text-xs); color: var(--muted-foreground); margin-right: var(--space-xs); }
.cvc-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: var(--h-xs);
  width: var(--h-xs);
  background: none;
  border: none;
  color: var(--muted-foreground);
  cursor: pointer;
  border-radius: var(--radius);
  transition: color var(--transition), background var(--transition);
}
.cvc-btn:hover { color: var(--muted-foreground); background: var(--muted); }
.cvc-btn.danger:hover { color: var(--destructive); }

.cvc-players {
  border-top: 1px solid var(--border);
  padding: var(--space-sm) var(--space-md);
  display: flex;
  flex-direction: column;
  gap: var(--space-xs);
}

.cvp-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: var(--h-sm);
  padding: 0 var(--space-xs);
  border-radius: var(--radius);
}
.cvp-row:hover { background: var(--muted); }
.cvp-name { font-size: var(--text-sm); color: var(--muted-foreground); }
.cvp-actions { display: flex; gap: var(--space-xs); }

.cvp-add-row { display: flex; gap: var(--space-xs); margin-top: var(--space-xs); }
.cvp-input {
  flex: 1;
  background: var(--card);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  color: var(--foreground);
  font-family: var(--font-body);
  font-size: var(--text-sm);
  height: var(--h-sm);
  padding: 0 var(--space-sm);
  outline: none;
}
.cvp-input:focus { border-color: var(--secondary); }
.cvp-add-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: var(--h-sm);
  width: var(--h-sm);
  background: var(--secondary);
  border: none;
  border-radius: var(--radius);
  color: var(--primary);
  cursor: pointer;
}
.cvp-add-btn:disabled { opacity: 0.4; cursor: default; }

.cv-empty { color: var(--muted-foreground); font-size: var(--text-sm); padding: var(--space-md) 0; }

.cv-toast {
  position: fixed;
  bottom: var(--space-xl);
  left: 50%;
  transform: translateX(-50%);
  background: var(--muted);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  color: var(--foreground);
  font-size: var(--text-sm);
  padding: var(--space-xs) var(--space-md);
  pointer-events: none;
}
</style>
