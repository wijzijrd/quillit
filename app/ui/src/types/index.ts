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

export interface Project {
  id: string
  name: string
  typeId?: string
  ownerId?: string
  /** True while the project has a running game session (as of last fetch). */
  live?: boolean
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

export interface User {
  id?: string
  sub: string
  email: string
  username?: string
  role: 'admin' | 'user'
  active?: boolean
}

export type Season = 'spring' | 'summer' | 'autumn' | 'winter'

export interface UserSettings {
  season?: Season
  glass?: boolean
  /** Legacy light/dark setting; read once to migrate to `season`. */
  theme?: 'light' | 'dark'
  [key: string]: unknown
}

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

export type GameSessionStatus = 'running' | 'stopped'

export interface GameSession {
  id: string
  projectId: string
  status: GameSessionStatus
  startedBy: string
  startedAt: number
  stoppedBy?: string
  stoppedAt?: number
}

export type ChatMessageType = 'text' | 'note_card'

export interface ChatMessage {
  id: string
  sessionId: string
  projectId: string
  senderId: string
  type: ChatMessageType
  body: string
  entryId?: string
  cardTitle: string
  cardBody: string
  createdAt: number
}

/** Server→client `{"type":"system","event":"session_ended"}` frame. */
export interface SessionEndedFrame {
  type: 'system'
  event: 'session_ended'
}

/** Per-sender `{"type":"error","message":"..."}` frame (never broadcast). */
export interface WSErrorFrame {
  type: 'error'
  message: string
}

/** Roster snapshot broadcast on every room membership change. */
export interface PresenceFrame {
  type: 'presence'
  users: string[]
}

export type WSInboundFrame = ChatMessage | SessionEndedFrame | WSErrorFrame | PresenceFrame
