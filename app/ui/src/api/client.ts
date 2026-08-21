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

/**
 * Shape of the JSON body ofetch attaches to a caught rejection's `.data` for
 * this API's error responses — see e.g. svc/auth's generic `ErrorResponse{error}`
 * and auth-svc's `RegisterConflictResponse{error,field}` (app/auth/internal/handler/auth.go),
 * proxied through unchanged by svc's POST /api/auth/register.
 */
interface ApiErrorData {
  error?: string
  field?: string
}

/** Extracts the API's { error } message from a caught ofetch rejection, falling back otherwise. */
export function apiErrorMessage(e: unknown, fallback: string): string {
  const data = (e as { data?: ApiErrorData } | undefined)?.data
  return data?.error ?? fallback
}

/** Extracts the API's { field } hint from a caught ofetch rejection (e.g. RegisterConflictResponse's "email"/"username" on a 409), or undefined if absent. */
export function apiErrorField(e: unknown): string | undefined {
  return (e as { data?: ApiErrorData } | undefined)?.data?.field
}
