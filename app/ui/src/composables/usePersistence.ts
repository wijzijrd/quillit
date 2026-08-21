import { get, set } from 'idb-keyval'
import type { Entry } from '../types'

const ENTRIES_KEY = 'quillit:entries'

export async function saveEntries(entries: Entry[]) { await set(ENTRIES_KEY, entries) }
export async function loadEntries(): Promise<Entry[] | undefined> { return await get(ENTRIES_KEY) }
