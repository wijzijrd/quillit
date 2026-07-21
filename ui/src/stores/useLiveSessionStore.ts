import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api } from '../api/client'
import { connect, type WSConnection } from '../api/ws'
import type { GameSession, GameSessionStatus, ChatMessage, WSInboundFrame } from '../types'

// GET .../session/status returns either the running GameSession row, or a
// bare `{ status: 'stopped' }` when nothing is running (see game_sessions.go
// Status handler).
type StatusResponse = GameSession | { status: GameSessionStatus }

export const useLiveSessionStore = defineStore('liveSession', () => {
  const status = ref<GameSessionStatus>('stopped')
  const sessionId = ref<string | null>(null)
  const messages = ref<ChatMessage[]>([])
  const connected = ref(false)
  const error = ref<string | null>(null)

  let socket: WSConnection | null = null
  let connectedProjectId: string | null = null

  function applySession(session: GameSession) {
    status.value = session.status
    sessionId.value = session.id
  }

  async function fetchStatus(projectId: string) {
    const res = await api<StatusResponse>(`/projects/${projectId}/session/status`)
    // A socket may still be open for a different project (e.g. the user
    // navigated away from a running session's project without stopping it).
    // Status is scoped per-project, so once we know which project this
    // refresh is for, don't leave a stale cross-project connection silently
    // receiving messages into the shared state. Same-project refreshes
    // (including the socket's own shouldReconnect check, and NoteSharePanel/
    // LiveSessionPanel both polling the same project) are a no-op here.
    if (connectedProjectId && connectedProjectId !== projectId) {
      disconnect()
    }
    if ('id' in res) {
      applySession(res)
    } else {
      status.value = res.status
      sessionId.value = null
    }
  }

  async function start(projectId: string) {
    const session = await api<GameSession>(`/projects/${projectId}/session/start`, { method: 'POST' })
    applySession(session)
  }

  async function stop(projectId: string) {
    const session = await api<GameSession>(`/projects/${projectId}/session/stop`, { method: 'POST' })
    applySession(session)
    disconnect()
  }

  async function fetchHistory(projectId: string, sid: string) {
    messages.value = await api<ChatMessage[]>(`/projects/${projectId}/session/${sid}/messages`)
  }

  function handleFrame(data: WSInboundFrame) {
    if (data.type === 'system' && data.event === 'session_ended') {
      status.value = 'stopped'
      disconnect()
      return
    }
    if (data.type === 'error') {
      error.value = data.message
      return
    }
    if (data.type === 'text' || data.type === 'note_card') {
      messages.value.push(data)
    }
  }

  function connectSocket(projectId: string) {
    // Already connected to this project's room — no-op. If connected to a
    // different project's session (user navigated between projects without
    // stopping the prior one), swap connections.
    if (socket && connectedProjectId === projectId) return
    if (socket) disconnect()

    connectedProjectId = projectId
    socket = connect(`/api/projects/${projectId}/session/socket`, {
      // Only reconnect if the session is still running server-side — an
      // ended session shouldn't keep retrying forever.
      shouldReconnect: async () => {
        try {
          await fetchStatus(projectId)
        } catch {
          return false
        }
        return status.value === 'running'
      },
    })
    connected.value = true

    socket.onMessage((data: unknown) => {
      if (!data || typeof data !== 'object' || !('type' in data)) return
      handleFrame(data as WSInboundFrame)
    })
  }

  function disconnect() {
    socket?.close()
    socket = null
    connectedProjectId = null
    connected.value = false
  }

  function sendText(body: string) {
    socket?.send({ type: 'chat', body })
  }

  function shareEntry(entryId: string) {
    socket?.send({ type: 'share_card', entryId })
  }

  function clearError() {
    error.value = null
  }

  return {
    status, sessionId, messages, connected, error,
    fetchStatus, start, stop, fetchHistory,
    connect: connectSocket, disconnect, sendText, shareEntry, clearError,
  }
})
