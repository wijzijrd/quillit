export type AnnotationVisibility = 'gm' | 'shared' | 'player'
export type EntryVisibility = 'private' | 'public'

export interface Entry {
  id: string
  title: string
  body: string
  category: string
  visibility: EntryVisibility
  tags: string[]
  linkedEntries: string[]
  campaignIds?: string[]
  projectId?: string
  metadata?: Record<string, string>
}

export interface Category {
  id: string
  name: string
  icon: string
  color?: string
  isGlobal?: boolean
  defaultTags: DefaultTag[]
  projectId?: string
}

export interface DefaultTag {
  id: string
  label: string
}

export interface Campaign {
  id: string
  name: string
  players: Player[]
}

export interface Player {
  id: string
  name: string
  token: string
  campaignId?: string
}

export interface Project {
  id: string
  name: string
  typeId?: string
  ownerId?: string
}

export interface ProjectType {
  id: string
  name: string
}

export interface ProjectMember {
  userId: string
  username: string
  role: string
}

export interface ProjectInvite {
  token: string
  role: string
  expiresAt?: string
}

export interface Annotation {
  id: string
  entryId: string
  text: string
  visibility: AnnotationVisibility
  sharedWith: string[]
}

export interface User {
  id?: string
  sub: string
  email: string
  username?: string
  role: 'admin' | 'user'
  active?: boolean
}

export interface UserSettings {
  theme?: 'light' | 'dark'
  [key: string]: unknown
}

export interface QuickViewField {
  key: string
  label: string
  type: 'text' | 'textarea' | 'select'
  options?: string[]
}

export type QuickViewTemplates = Record<string, QuickViewField[]>

export interface EntryRelation {
  id: string
  fromId: string
  toId: string
  label: string
}

export interface Folder {
  id: string
  name: string
}

export interface SessionNote {
  id: string
  title: string
  body: string
  category: string
  visibility: string
  tags: string[]
}

export interface SharedNote {
  id: string
  entryId: string
  title: string
}
