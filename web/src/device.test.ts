import { expect, test } from 'vitest'
import { describeDevice } from './device'

test('names devices the way people would', () => {
  expect(
    describeDevice(
      'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15',
    ),
  ).toBe('Safari on a Mac')
  expect(
    describeDevice(
      'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1',
    ),
  ).toBe('Safari on an iPhone')
  expect(
    describeDevice(
      'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36 Edg/130.0',
    ),
  ).toBe('Edge on Windows')
  expect(describeDevice(undefined)).toBe('An unknown device')
})
