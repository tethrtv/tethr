import { expect, test } from 'vitest'
import { choiceOf, describe, forSaving, nextChoice, setChoice, setSpan } from './rules'

test('a value goes from any to none to off', () => {
  let v = setChoice(undefined, 'comedy', nextChoice[choiceOf(undefined, 'comedy')])
  expect(v).toEqual({ any: ['comedy'] })
  v = setChoice(v, 'comedy', nextChoice[choiceOf(v, 'comedy')])
  expect(v).toEqual({ none: ['comedy'] })
  v = setChoice(v, 'comedy', nextChoice[choiceOf(v, 'comedy')])
  expect(v).toBeUndefined()
})

test('a range keeps whole numbers and drops empty ends', () => {
  let s = setSpan(undefined, 'from', '1989')
  s = setSpan(s, 'to', '1999.7')
  expect(s).toEqual({ from: 1989, to: 1999 })
  expect(setSpan(setSpan(s, 'from', ''), 'to', ' ')).toBeUndefined()
})

test('describes a rule in words', () => {
  const rule = {
    kind: 'movie' as const,
    studios: { any: ['Q127552', 'Q1047410'], none: ['Q1'] },
    types: { any: ['animated'] },
    year: { from: 1989, to: 1999 },
    runtime: { to: 99 },
    watched: false,
  }
  const said = Object.fromEntries(describe(rule, { Q127552: 'Pixar', Q1047410: 'Walt Disney Animation Studios' }))
  expect(said.Kind).toBe('Films')
  expect(said.Studio).toBe('Pixar or Walt Disney Animation Studios, not one with no name yet')
  expect(said.Type).toBe('Animated')
  expect(said.Year).toBe('1989 to 1999')
  expect(said.Runtime).toBe('Up to 99 minutes')
  expect(said.Watched).toBe('Not yet, by whoever’s looking')
  expect(said.Genre).toBeUndefined()
})

test('names and taken-out titles never go back to the server with the rule', () => {
  expect(forSaving({ studios: { any: ['Q1'] }, names: { Q1: 'Pixar' }, left_out: [5] })).toEqual({
    studios: { any: ['Q1'] },
  })
})
