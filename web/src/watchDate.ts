import type { components } from './api/schema'
import { clock, date } from './format'

type ItemRef = components['schemas']['ItemRef']
type NewWatch = components['schemas']['NewWatches']['items'][number]

export type When = { kind: 'release' } | { kind: 'day'; day: string; time: string } | { kind: 'unknown' }

const at = (day: string, time: string) => (time ? new Date(`${day}T${time}`).toISOString() : `${day}T12:00:00Z`)

export function dated(items: ItemRef[], when: When): NewWatch[] {
  switch (when.kind) {
    case 'release':
      return items.map((item) => ({ item, released: true }))
    case 'unknown':
      return items.map((item) => ({ item, precision: 'none' }))
  }
  const watched_at = at(when.day, when.time)
  return items.map((item) => ({ item, watched_at, precision: when.time ? 'time' : 'day' }))
}

export function problem(when: When, now = new Date()): string | undefined {
  if (when.kind !== 'day') return
  if (!/^\d{4}-\d{2}-\d{2}$/.test(when.day)) return 'Pick the day you watched it.'
  if (new Date(`${when.day}T${when.time || '00:00'}`) > now) return 'That’s still to come. Pick a day up to today.'
}

export function moved(when: When, released?: string): string {
  switch (when.kind) {
    case 'release':
      return released ? `Moved to ${date(released)}, when it came out.` : 'Moved to the day each one aired.'
    case 'unknown':
      return 'Kept with no date.'
  }
  const stamp = at(when.day, when.time)
  return when.time ? `Moved to ${date(stamp)} at ${clock(stamp)}.` : `Moved to ${date(stamp)}.`
}
