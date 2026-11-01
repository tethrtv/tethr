import { expect, test } from 'vitest'
import { cleanCode } from '../components/CodeInput'
import { toUsername } from './Join'

test('suggests a username from a name', () => {
  expect(toUsername('Nathan')).toBe('nathan')
  expect(toUsername('Sam Taylor-Jones')).toBe('sam-taylor-jones')
  expect(toUsername('  Zoë  ')).toBe('zoe')
})

test('keeps only digits in a code, six at most', () => {
  expect(cleanCode('12 34-56 78')).toBe('123456')
  expect(cleanCode('abc')).toBe('')
})
