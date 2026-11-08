import { expect, test } from 'vitest'
import { playedAgo, playedOn } from './format'

test('says when a play has no date', () => {
  const p = { watched_at: '1970-01-01T00:00:00Z', precision: 'none' } as const
  expect(playedAgo(p)).toBe('date unknown')
  expect(playedOn(p)).toBe('Date unknown')
})

test('keeps dated plays as they were', () => {
  expect(playedOn({ watched_at: '2026-11-08T12:00:00Z', precision: 'day' })).toBe('8 Nov 2026')
  expect(playedOn({ watched_at: '2026-11-08T20:30:00Z', precision: 'time' })).toBe('8 Nov 2026')
})
