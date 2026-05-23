import { get, set, del } from 'idb-keyval'

const ENTRIES_KEY = 'quillit:entries'
const ANNOTATIONS_KEY = 'quillit:annotations'
const CAMPAIGN_KEY = 'quillit:campaign'
const CAMPAIGNS_KEY = 'quillit:campaigns'
const QUICK_VIEW_TEMPLATES_KEY = 'quillit:quick-view-templates'

export async function saveEntries(entries) { await set(ENTRIES_KEY, entries) }
export async function loadEntries() { return await get(ENTRIES_KEY) }

export async function saveAnnotations(annotations) { await set(ANNOTATIONS_KEY, annotations) }
export async function loadAnnotations() { return (await get(ANNOTATIONS_KEY)) ?? [] }

export async function saveCampaigns(data) { await set(CAMPAIGNS_KEY, data) }
export async function loadCampaigns() { return await get(CAMPAIGNS_KEY) }

export async function saveQuickViewTemplates(data) { await set(QUICK_VIEW_TEMPLATES_KEY, data) }
export async function loadQuickViewTemplates() { return await get(QUICK_VIEW_TEMPLATES_KEY) }

export async function savePlayerNotes(token, notes) {
  await set(`quillit:player-notes:${token}`, notes)
}
export async function loadPlayerNotes(token) {
  return (await get(`quillit:player-notes:${token}`)) ?? []
}

// Legacy single-campaign key — used only by migration in useCampaignStore
export async function loadLegacyCampaign() { return await get(CAMPAIGN_KEY) }
export async function deleteLegacyCampaign() { await del(CAMPAIGN_KEY) }

// Keep for DashboardView export/import backward compat
export async function saveCampaign(data) { await set(CAMPAIGN_KEY, data) }
export async function loadCampaign() { return (await get(CAMPAIGN_KEY)) ?? { name: 'My Campaign', players: [] } }
