import createClient from 'openapi-fetch'
import { checkSignal } from '../signal'
import type { components, paths } from './schema'

export type Me = components['schemas']['Me']
export type Problem = components['schemas']['Problem']

export const api = createClient<paths>({
  baseUrl: import.meta.env.VITE_API_URL ?? 'http://localhost:8080',
  credentials: 'include',
})

api.use({
  onResponse({ response }) {
    if (response.status === 502 || response.status === 503 || response.status === 504) checkSignal()
  },
  onError() {
    checkSignal()
  },
})

export function problemMessage(error: unknown) {
  if (error && typeof error === 'object' && 'message' in error && typeof error.message === 'string') {
    return error.message
  }
  return 'Something went wrong. Try again in a moment.'
}
