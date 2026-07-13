import { ofetch } from 'ofetch'
import router from '../router/index'

export const api = ofetch.create({
  baseURL: '/api',
  credentials: 'include',
  onResponseError({ response }) {
    if (response.status === 401) {
      const current = router.currentRoute.value
      if (!current.meta.public) {
        router.push({ path: '/login', query: { redirect: current.fullPath } })
      }
    }
  },
})
