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

/** Extracts the API's { error } message from a caught ofetch rejection, falling back otherwise. */
export function apiErrorMessage(e: unknown, fallback: string): string {
  const data = (e as { data?: { error?: string } } | undefined)?.data
  return data?.error ?? fallback
}
