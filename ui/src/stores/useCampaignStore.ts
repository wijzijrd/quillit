import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { api } from '../api/client'
import { loadCampaigns, saveCampaigns } from '../composables/usePersistence'
import { playerShareLink } from '../utils/links'
import type { Campaign, Player } from '../types'

export const useCampaignStore = defineStore('campaign', () => {
  const campaigns = ref<Campaign[]>([])
  const activeCampaignId = ref<string | null>(null)
  const loaded = ref(false)

  async function init() {
    if (loaded.value) return
    loaded.value = true
    try {
      const cached = await loadCampaigns()
      if (cached?.length) {
        campaigns.value = cached
        activeCampaignId.value = cached[0]?.id ?? null
      }
      const fresh = await api('/campaigns')
      campaigns.value = fresh
      activeCampaignId.value = fresh[0]?.id ?? null
      await saveCampaigns(fresh)
    } catch (e) {
      loaded.value = false
      throw e
    }
  }

  const activeCampaign = computed(() =>
    campaigns.value.find(c => c.id === activeCampaignId.value) ?? campaigns.value[0] ?? null
  )

  function setActiveCampaign(id: string) { activeCampaignId.value = id }

  async function addCampaign(campaignName: string): Promise<Campaign> {
    const campaign: Campaign = await api('/campaigns', { method: 'POST', body: { name: campaignName.trim() } })
    campaigns.value = [...campaigns.value, campaign]
    await saveCampaigns(campaigns.value)
    return campaign
  }

  async function removeCampaign(id: string) {
    await api(`/campaigns/${id}`, { method: 'DELETE' })
    campaigns.value = campaigns.value.filter(c => c.id !== id)
    if (activeCampaignId.value === id) activeCampaignId.value = campaigns.value[0]?.id ?? null
    await saveCampaigns(campaigns.value)
  }

  async function renameCampaign(id: string, newName: string) {
    const updated: Campaign = await api(`/campaigns/${id}`, { method: 'PATCH', body: { name: newName.trim() } })
    const idx = campaigns.value.findIndex(c => c.id === id)
    if (idx !== -1) campaigns.value[idx] = updated
    await saveCampaigns(campaigns.value)
  }

  async function addPlayer(campaignId: string, playerName: string): Promise<Player> {
    const player: Player = await api(`/campaigns/${campaignId}/players`, {
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
    await saveCampaigns(campaigns.value)
    return player
  }

  async function removePlayer(campaignId: string, playerId: string) {
    await api(`/campaigns/${campaignId}/players/${playerId}`, { method: 'DELETE' })
    const idx = campaigns.value.findIndex(c => c.id === campaignId)
    if (idx !== -1) {
      campaigns.value[idx] = {
        ...campaigns.value[idx],
        players: campaigns.value[idx].players.filter(p => p.id !== playerId),
      }
    }
    await saveCampaigns(campaigns.value)
  }

  function getByToken(token: string): { player: Player; campaign: Campaign } | null {
    for (const campaign of campaigns.value) {
      const player = campaign.players?.find(p => p.token === token)
      if (player) return { player, campaign }
    }
    return null
  }

  function shareUrl(player: Player): string {
    return playerShareLink(player.token)
  }

  const players = computed(() => campaigns.value.flatMap(c => c.players ?? []))
  const name = computed(() => activeCampaign.value?.name ?? 'My Campaign')
  function setName(newName: string) { if (activeCampaign.value) renameCampaign(activeCampaign.value.id, newName) }

  return {
    campaigns, activeCampaignId, activeCampaign, loaded,
    init, setActiveCampaign, addCampaign, removeCampaign, renameCampaign,
    addPlayer, removePlayer, getByToken, shareUrl,
    players, name, setName,
  }
})
