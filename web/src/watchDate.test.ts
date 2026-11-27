import { expect, test } from 'vitest'
import { dated, moved, problem } from './watchDate'

const film = { type: 'movie' as const, ids: { tmdb: 27205 } }

test('asks the server for the release date', () => {
  expect(dated([film], { kind: 'release' })).toEqual([{ item: film, released: true }])
})

test('keeps a day at noon UTC and a time as given', () => {
  expect(dated([film], { kind: 'day', day: '2024-06-02', time: '' })).toEqual([
    { item: film, watched_at: '2024-06-02T12:00:00Z', precision: 'day' },
  ])
  const [timed] = dated([film], { kind: 'day', day: '2024-06-02', time: '21:15' })
  expect(timed.precision).toBe('time')
  expect(timed.watched_at).toBe(new Date('2024-06-02T21:15').toISOString())
})

test('logs an unknown date with no time at all', () => {
  expect(dated([film], { kind: 'unknown' })).toEqual([{ item: film, precision: 'none' }])
})

test('wants a day that has happened', () => {
  const now = new Date('2026-11-27T12:00')
  expect(problem({ kind: 'day', day: '', time: '' }, now)).toBe('Pick the day you watched it.')
  expect(problem({ kind: 'day', day: '2026-11-28', time: '' }, now)).toMatch(/still to come/)
  expect(problem({ kind: 'day', day: '2026-11-27', time: '13:00' }, now)).toMatch(/still to come/)
  expect(problem({ kind: 'day', day: '2026-11-27', time: '' }, now)).toBeUndefined()
  expect(problem({ kind: 'release' }, now)).toBeUndefined()
})

test('says where the play went', () => {
  expect(moved({ kind: 'release' }, '2010-07-08')).toBe('Moved to 8 Jul 2010, when it came out.')
  expect(moved({ kind: 'release' })).toBe('Moved to the day each one aired.')
  expect(moved({ kind: 'unknown' })).toBe('Kept with no date.')
  expect(moved({ kind: 'day', day: '2024-06-02', time: '' })).toBe('Moved to 2 Jun 2024.')
})
