import { expect, test } from 'vitest'
import { destination } from './Authorize'

test('names where an app sends people after they choose', () => {
  expect(destination('https://zuno.app/tethr/callback')).toBe('zuno.app')
  expect(destination('http://localhost:3000/cb')).toBe('localhost:3000')
  expect(destination('zuno://tethr')).toBe('zuno://')
  expect(destination('not a url')).toBe('not a url')
})
