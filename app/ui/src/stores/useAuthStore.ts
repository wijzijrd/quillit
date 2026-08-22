import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { api } from '../api/client'
import type { AuthUser } from '../types'

export const useAuthStore = defineStore('auth', () => {
  const _loggedIn = ref<boolean | null>(null)
  const _user = ref<AuthUser | null>(null)
  const isLoggedIn = computed(() => _loggedIn.value === true)
  const user = computed(() => _user.value)

  async function fetchMe() {
    try {
      const data: AuthUser = await api('/auth/me')
      _user.value = data
      _loggedIn.value = true
    } catch {
      _user.value = null
      _loggedIn.value = false
    }
  }

  async function login(email: string, password: string) {
    await api('/auth/login', { method: 'POST', body: { email, password } })
    await fetchMe()
  }

  async function register(email: string, username: string, password: string) {
    await api('/auth/register', { method: 'POST', body: { email, username, password } })
    await fetchMe()
  }

  async function logout() {
    await api('/auth/logout', { method: 'POST' }).catch(() => {})
    _loggedIn.value = false
    _user.value = null
  }

  return { isLoggedIn, user, fetchMe, login, register, logout }
})
