import { get, set, del } from 'idb-keyval'
import type { Entry, Annotation, Campaign } from '../types'
import type { QuickViewTemplates } from '../types'

const ENTRIES_KEY = 'quillit:entries'
const ANNOTATIONS_KEY = 'quillit:annotations'
const CAMPAIGN_KEY = 'quillit:campaign'
const CAMPAIGNS_KEY = 'quillit:campaigns'
const QUICK_VIEW_TEMPLATES_KEY = 'quillit:quick-view-templates'

export async function saveEntries(entries: Entry[]) { await set(ENTRIES_KEY, entries) }
export async function loadEntries(): Promise<Entry[] | undefined> { return await get(ENTRIES_KEY) }

export async function saveAnnotations(annotations: Annotation[]) { await set(ANNOTATIONS_KEY, annotations) }
export async function loadAnnotations(): Promise<Annotation[]> { return (await get(ANNOTATIONS_KEY)) ?? [] }

export async function saveCampaigns(data: Campaign[]) { await set(CAMPAIGNS_KEY, data) }
export async function loadCampaigns(): Promise<Campaign[] | undefined> { return await get(CAMPAIGNS_KEY) }

export async function saveQuickViewTemplates(data: QuickViewTemplates) { await set(QUICK_VIEW_TEMPLATES_KEY, data) }
export async function loadQuickViewTemplates(): Promise<QuickViewTemplates | undefined> { return await get(QUICK_VIEW_TEMPLATES_KEY) }

export async function savePlayerNotes(token: string, notes: unknown[]) {
  await set(`quillit:player-notes:${token}`, notes)
}
export async function loadPlayerNotes(token: string): Promise<unknown[]> {
  return (await get(`quillit:player-notes:${token}`)) ?? []
}

export async function loadLegacyCampaign(): Promise<Campaign | undefined> { return await get(CAMPAIGN_KEY) }
export async function deleteLegacyCampaign() { await del(CAMPAIGN_KEY) }

export async function saveCampaign(data: Campaign) { await set(CAMPAIGN_KEY, data) }
export async function loadCampaign(): Promise<Campaign> {
  return (await get(CAMPAIGN_KEY)) ?? { id: '', name: 'My Campaign', players: [] }
}
