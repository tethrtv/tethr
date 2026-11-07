import { useSyncExternalStore } from 'react'

const apiURL = import.meta.env.VITE_API_URL ?? 'http://localhost:8080'
const listeners = new Set<() => void>()
let lost = false
let checking: Promise<void> | undefined
let retry = 0

function set(next: boolean) {
  if (lost && !next) {
    window.location.reload()
    return
  }
  lost = next
  clearTimeout(retry)
  if (lost) retry = window.setTimeout(checkSignal, 5000)
  for (const l of listeners) l()
}

export function checkSignal() {
  checking ??= fetch(`${apiURL}/v1/me`, { credentials: 'include', cache: 'no-store' })
    .then((r) => set(r.status >= 500))
    .catch(() => set(true))
    .finally(() => {
      checking = undefined
    })
  return checking
}

export function useLostSignal() {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
    () => lost,
  )
}

window.addEventListener('offline', () => set(true))
window.addEventListener('online', () => checkSignal())
