import { get, set } from 'idb-keyval'
import type { Entry } from '../types'
import type { QuickViewTemplates } from '../types'

const ENTRIES_KEY = 'quillit:entries'
const QUICK_VIEW_TEMPLATES_KEY = 'quillit:quick-view-templates'

export async function saveEntries(entries: Entry[]) { await set(ENTRIES_KEY, entries) }
export async function loadEntries(): Promise<Entry[] | undefined> { return await get(ENTRIES_KEY) }

export async function saveQuickViewTemplates(data: QuickViewTemplates) { await set(QUICK_VIEW_TEMPLATES_KEY, data) }
export async function loadQuickViewTemplates(): Promise<QuickViewTemplates | undefined> { return await get(QUICK_VIEW_TEMPLATES_KEY) }
