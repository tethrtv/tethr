import { expect, test } from 'vitest'
import { starWords } from './Stars'

test('scores out of 10 read as stars', () => {
  expect(starWords(1)).toBe('half a star')
  expect(starWords(2)).toBe('1 star')
  expect(starWords(3)).toBe('1½ stars')
  expect(starWords(8)).toBe('4 stars')
  expect(starWords(10)).toBe('5 stars')
})
