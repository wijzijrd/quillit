import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api } from '../api/client'
import type { Friend, FriendRequest } from '../types'

export const useFriendsStore = defineStore('friends', () => {
  const friends = ref<Friend[]>([])
  const incoming = ref<FriendRequest[]>([])
  const outgoing = ref<FriendRequest[]>([])
  const loaded = ref(false)

  async function init() {
    if (loaded.value) return
    loaded.value = true
    try {
      const [friendsRes, incomingRes, outgoingRes] = await Promise.all([
        api('/friends'),
        api('/friends/requests/incoming'),
        api('/friends/requests/outgoing'),
      ])
      friends.value = friendsRes
      incoming.value = incomingRes
      outgoing.value = outgoingRes
    } catch (e) {
      loaded.value = false
      throw e
    }
  }

  async function sendRequest(userId: string, username: string): Promise<FriendRequest> {
    const request: FriendRequest = await api('/friends/requests', {
      method: 'POST',
      body: { userId, username },
    })
    outgoing.value = [...outgoing.value, request]
    return request
  }

  async function acceptRequest(id: string) {
    await api(`/friends/requests/${id}/accept`, { method: 'POST' })
    incoming.value = incoming.value.filter(r => r.id !== id)
    friends.value = await api('/friends')
  }

  async function removeRequest(id: string) {
    await api(`/friends/requests/${id}`, { method: 'DELETE' })
    incoming.value = incoming.value.filter(r => r.id !== id)
    outgoing.value = outgoing.value.filter(r => r.id !== id)
    friends.value = friends.value.filter(f => f.id !== id)
  }

  return {
    friends, incoming, outgoing, loaded,
    init, sendRequest, acceptRequest, removeRequest,
  }
})
