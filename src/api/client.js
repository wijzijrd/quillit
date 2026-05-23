import { ofetch } from 'ofetch'

export const api = ofetch.create({
  baseURL: '/api',
  credentials: 'include',
  onResponseError({ response }) {
    if (response.status === 401) {
      if (!window.location.pathname.startsWith('/login') && !window.location.pathname.startsWith('/setup')) {
        window.location.href = '/login'
      }
    }
  },
})
