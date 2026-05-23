import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { api } from '../api/client.js'

export const useAuthStore = defineStore('auth', () => {
  const _loggedIn = ref(null) // null = unknown, true/false after check
  const _user = ref(null)     // { sub, email, role } or null
  const isLoggedIn = computed(() => _loggedIn.value === true)
  const user = computed(() => _user.value)

  async function fetchMe() {
    try {
      const data = await api('/auth/me')
      _user.value = data
      _loggedIn.value = true
    } catch {
      _user.value = null
      _loggedIn.value = false
    }
  }

  async function login(email, password) {
    await api('/auth/login', {
      method: 'POST',
      body: { email, password },
    })
    await fetchMe()
  }

  async function register(email, username, password) {
    await api('/auth/register', {
      method: 'POST',
      body: { email, username, password },
    })
    await fetchMe()
  }

  async function logout() {
    await api('/auth/logout', { method: 'POST' }).catch(() => {})
    _loggedIn.value = false
    _user.value = null
  }

  return { isLoggedIn, user, fetchMe, login, register, logout }
})
