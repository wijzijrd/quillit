export interface Project {
  id: string
  name: string
  typeId?: string
  ownerId?: string
  /**
   * Project type, e.g. "campaign". Always present — svc's own Project and
   * AdminProject response structs both set it unconditionally (see
   * app/svc/internal/handler/projects.go's `Project` and
   * app/svc/internal/handler/admin.go's `AdminProject`).
   */
  type: string
  /** Always present in both the member's own project list and the admin's all-projects list (same two Go structs as `type`). */
  memberCount: number
  /**
   * The caller's role in this project (e.g. "gm", "player"). Present on
   * responses from GET/POST/PATCH /api/projects (svc's `Project` struct,
   * which always sets it from the caller's own membership row). Absent
   * from the admin all-projects listing (GET /api/admin/projects, svc's
   * `AdminProject` struct has no MyRole field at all — that endpoint isn't
   * scoped to a single caller's membership).
   */
  myRole?: string
  /** [editorLabel, memberLabel] display pair for this project's type, e.g. ["GM","Player"] for campaigns. Always present — set unconditionally by `roleLabelPair()` in both Project and AdminProject responses. */
  roleLabels: [string, string]
  /** True while the project has a running game session (as of last fetch). Present on svc's own `Project` struct; absent from `AdminProject` (admin listing has no Live field). */
  live?: boolean
}

/** Matches svc's ProjectType response struct (app/svc/internal/handler/projects.go, GET /api/projects/types) — not { id, name }, which nothing in that endpoint's response ever produces. */
export interface ProjectType {
  type: string
  label: string
  /** [editorLabel, memberLabel] pair for this project type. */
  roleLabels: [string, string]
}

/** Matches svc's ProjectMember response struct (app/svc/internal/handler/projects.go) — every field is always populated (no omitempty tags), on every endpoint that returns it (list/add/join, and the admin equivalent in admin.go). */
export interface ProjectMember {
  id: string
  projectId: string
  userId: string
  username: string
  role: string
  /** Unix seconds. */
  joinedAt: number
}

/** Matches auth-svc's UserSearchResult (app/auth/internal/handler/auth.go), the shape behind the user-search lookup used to add project members. */
export interface UserSearchResult {
  id: string
  email: string
  username: string
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
