import { ofetch } from 'ofetch'
import router from '../router/index'

export const api = ofetch.create({
  baseURL: '/api',
  credentials: 'include',
  onResponseError({ response }) {
    if (response.status === 401) {
      const path = window.location.pathname
      if (!path.startsWith('/login') && !path.startsWith('/setup')) {
        router.push({ path: '/login', query: { redirect: path } })
      }
    }
  },
})
