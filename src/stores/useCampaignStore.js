import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { api } from '../api/client.js'

export const useCampaignStore = defineStore('campaign', () => {
  const campaigns = ref([])
  const activeCampaignId = ref(null)
  const loaded = ref(false)

  async function init() {
    if (loaded.value) return
    campaigns.value = await api('/campaigns')
    activeCampaignId.value = campaigns.value[0]?.id ?? null
    loaded.value = true
  }

  const activeCampaign = computed(() =>
    campaigns.value.find(c => c.id === activeCampaignId.value) ?? campaigns.value[0] ?? null
  )

  function setActiveCampaign(id) { activeCampaignId.value = id }

  async function addCampaign(campaignName) {
    const campaign = await api('/campaigns', { method: 'POST', body: { name: campaignName.trim() } })
    campaigns.value = [...campaigns.value, campaign]
    return campaign
  }

  async function removeCampaign(id) {
    await api(`/campaigns/${id}`, { method: 'DELETE' })
    campaigns.value = campaigns.value.filter(c => c.id !== id)
    if (activeCampaignId.value === id) activeCampaignId.value = campaigns.value[0]?.id ?? null
  }

  async function renameCampaign(id, newName) {
    const updated = await api(`/campaigns/${id}`, { method: 'PATCH', body: { name: newName.trim() } })
    const idx = campaigns.value.findIndex(c => c.id === id)
    if (idx !== -1) campaigns.value[idx] = updated
  }

  async function addPlayer(campaignId, playerName) {
    const player = await api(`/campaigns/${campaignId}/players`, {
      method: 'POST',
      body: { name: playerName.trim() },
    })
    const idx = campaigns.value.findIndex(c => c.id === campaignId)
    if (idx !== -1) {
      campaigns.value[idx] = {
        ...campaigns.value[idx],
        players: [...(campaigns.value[idx].players ?? []), player],
      }
    }
    return player
  }

  async function removePlayer(campaignId, playerId) {
    await api(`/campaigns/${campaignId}/players/${playerId}`, { method: 'DELETE' })
    const idx = campaigns.value.findIndex(c => c.id === campaignId)
    if (idx !== -1) {
      campaigns.value[idx] = {
        ...campaigns.value[idx],
        players: campaigns.value[idx].players.filter(p => p.id !== playerId),
      }
    }
  }

  function getByToken(token) {
    for (const campaign of campaigns.value) {
      const player = campaign.players?.find(p => p.token === token)
      if (player) return { player, campaign }
    }
    return null
  }

  function shareUrl(player) {
    return `${window.location.origin}/share/${player.token}`
  }

  // Legacy compat
  const players = computed(() => campaigns.value.flatMap(c => c.players ?? []))
  const name = computed(() => activeCampaign.value?.name ?? 'My Campaign')
  function setName(newName) { if (activeCampaign.value) renameCampaign(activeCampaign.value.id, newName) }

  return {
    campaigns, activeCampaignId, activeCampaign, loaded,
    init, setActiveCampaign, addCampaign, removeCampaign, renameCampaign,
    addPlayer, removePlayer, getByToken, shareUrl,
    players, name, setName,
  }
})
