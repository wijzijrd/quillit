import type { QuickViewField, QuickViewTemplates } from '../types'

export const DEFAULT_TEMPLATES: QuickViewTemplates = {
  NPC: [
    { key: 'role',        label: 'Role',        type: 'text' },
    { key: 'location',    label: 'Location',    type: 'text' },
    { key: 'motivation',  label: 'Motivation',  type: 'text' },
    { key: 'allegiance',  label: 'Allegiance',  type: 'text' },
    { key: 'status',      label: 'Status',      type: 'select', options: ['alive', 'dead', 'unknown', 'missing'] },
  ],
  Location: [
    { key: 'type',        label: 'Type',             type: 'text' },
    { key: 'region',      label: 'Region',           type: 'text' },
    { key: 'notable',     label: 'Notable Features', type: 'textarea' },
    { key: 'controlledBy',label: 'Controlled By',    type: 'text' },
  ],
  Faction: [
    { key: 'leader',      label: 'Leader',      type: 'text' },
    { key: 'goal',        label: 'Goal',        type: 'text' },
    { key: 'alignment',   label: 'Alignment',   type: 'text' },
    { key: 'territory',   label: 'Territory',   type: 'text' },
  ],
  Event: [
    { key: 'when',        label: 'When',        type: 'text' },
    { key: 'location',    label: 'Location',    type: 'text' },
    { key: 'outcome',     label: 'Outcome',     type: 'text' },
    { key: 'keyNpcs',     label: 'Key NPCs',    type: 'text' },
  ],
  Item: [
    { key: 'type',        label: 'Type',        type: 'text' },
    { key: 'rarity',      label: 'Rarity',      type: 'select', options: ['common', 'uncommon', 'rare', 'very rare', 'legendary'] },
    { key: 'owner',       label: 'Owner',       type: 'text' },
    { key: 'properties',  label: 'Properties',  type: 'text' },
  ],
  Lore: [
    { key: 'source',      label: 'Source',      type: 'text' },
    { key: 'era',         label: 'Era',         type: 'text' },
    { key: 'relatedFactions', label: 'Related Factions', type: 'text' },
  ],
}
