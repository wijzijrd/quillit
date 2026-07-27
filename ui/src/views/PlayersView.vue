<template>
  <div class="players-view">
    <header class="players-header">
      <div class="header-left">
        <input
          class="campaign-name-input"
          :value="campaign.name"
          @blur="e => campaign.setName(e.target.value)"
          @keydown.enter="e => e.target.blur()"
        />
        <p class="header-sub">Player roster &amp; share links</p>
      </div>
    </header>

    <div class="players-body">
      <!-- Add player form -->
      <div class="add-player-form">
        <input
          class="add-input"
          v-model="newPlayerName"
          placeholder="Player name…"
          @keydown.enter="addPlayer"
          maxlength="60"
        />
        <button class="btn-add" @click="addPlayer" :disabled="!newPlayerName.trim()">
          Add Player
        </button>
      </div>

      <!-- Empty state -->
      <div class="empty-state" v-if="campaign.players.length === 0">
        <p>No players yet. Add a player above to generate a share link.</p>
      </div>

      <!-- Player cards -->
      <div class="player-list">
        <div class="player-card" v-for="player in campaign.players" :key="player.id">
          <div class="player-info">
            <span class="player-name">{{ player.name }}</span>
            <span class="player-since">Added {{ formatDate(player.createdAt) }}</span>
          </div>

          <div class="share-row">
            <input
              class="share-url-input"
              :value="campaign.shareUrl(player)"
              readonly
              @click="e => e.target.select()"
            />
            <button
              class="btn-copy"
              @click="copyUrl(player)"
              :class="{ copied: copiedId === player.id }"
              :title="copiedId === player.id ? 'Copied!' : 'Copy link'"
            >
              {{ copiedId === player.id ? '✓' : '⧉' }}
            </button>
            <a
              class="btn-preview"
              :href="`/share/${player.token}`"
              target="_blank"
              title="Open player view"
            >👁</a>
          </div>

          <button class="btn-remove" @click="removePlayer(player.id)" title="Remove player">
            Remove
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useCampaignStore } from '../stores/useCampaignStore'
import { formatDate } from '../utils/date'

const campaign = useCampaignStore()

const newPlayerName = ref('')
const copiedId = ref(null)

function addPlayer() {
  if (!newPlayerName.value.trim()) return
  campaign.addPlayer(newPlayerName.value)
  newPlayerName.value = ''
}

function removePlayer(id) {
  if (!confirm('Remove this player? Their share link will stop working.')) return
  campaign.removePlayer(id)
}

async function copyUrl(player) {
  const url = campaign.shareUrl(player)
  await navigator.clipboard.writeText(url)
  copiedId.value = player.id
  setTimeout(() => { copiedId.value = null }, 2000)
}

</script>

<style scoped>
.players-view {
  padding: 40px 48px;
  max-width: 760px;
}

.players-header {
  margin-bottom: 36px;
}

.campaign-name-input {
  background: none;
  border: none;
  font-family: var(--font-display);
  font-size: 2em;
  color: var(--primary);
  letter-spacing: 0.06em;
  outline: none;
  width: 100%;
  padding: 0;
}
.campaign-name-input:focus { border-bottom: 1px solid var(--secondary); }

.header-sub { color: var(--muted-foreground); margin-top: 4px; font-size: 0.95em; }

.players-body { display: flex; flex-direction: column; gap: 20px; }

.add-player-form {
  display: flex;
  gap: 10px;
}

.add-input {
  flex: 1;
  background: var(--muted);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  color: var(--foreground);
  font-family: var(--font-body);
  font-size: 0.95em;
  padding: 9px 14px;
  outline: none;
  transition: border-color var(--transition);
}
.add-input:focus { border-color: var(--secondary); }
.add-input::placeholder { color: var(--muted-foreground); }

.btn-add {
  background: var(--secondary);
  border: none;
  border-radius: var(--radius);
  color: var(--primary);
  font-family: var(--font-body);
  font-size: 0.9em;
  padding: 9px 20px;
  cursor: pointer;
  transition: background var(--transition), color var(--transition);
  white-space: nowrap;
}
.btn-add:hover:not(:disabled) { background: var(--primary); color: var(--background); }
.btn-add:disabled { opacity: 0.4; cursor: default; }

.empty-state {
  padding: 32px;
  text-align: center;
  color: var(--muted-foreground);
  font-size: 0.9em;
  border: 1px dashed var(--border);
  border-radius: var(--radius);
}

.player-list { display: flex; flex-direction: column; gap: 12px; }

.player-card {
  background: var(--card);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 16px 20px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.player-info { display: flex; align-items: baseline; gap: 12px; }
.player-name {
  font-family: var(--font-display);
  font-size: 1.05em;
  color: var(--foreground);
}
.player-since { font-size: 0.78em; color: var(--muted-foreground); }

.share-row { display: flex; gap: 6px; align-items: center; }

.share-url-input {
  flex: 1;
  background: var(--muted);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  color: var(--muted-foreground);
  font-family: monospace;
  font-size: 0.78em;
  padding: 6px 10px;
  outline: none;
  cursor: pointer;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.btn-copy, .btn-preview {
  background: var(--muted);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  color: var(--muted-foreground);
  font-size: 0.88em;
  padding: 6px 10px;
  cursor: pointer;
  transition: background var(--transition), color var(--transition);
  text-decoration: none;
  display: flex;
  align-items: center;
}
.btn-copy:hover, .btn-preview:hover { background: var(--muted); color: var(--foreground); }
.btn-copy.copied { color: #8e8; border-color: rgba(80,200,120,0.4); }

.btn-remove {
  align-self: flex-start;
  background: none;
  border: none;
  color: var(--muted-foreground);
  font-family: var(--font-body);
  font-size: 0.8em;
  cursor: pointer;
  padding: 2px 0;
  transition: color var(--transition);
}
.btn-remove:hover { color: var(--destructive); }
</style>
