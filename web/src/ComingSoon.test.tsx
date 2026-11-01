import { render, screen } from '@testing-library/react'
import { expect, test } from 'vitest'
import { ComingSoon } from './ComingSoon'

test('says what Tethr is and how to get in touch', () => {
  render(<ComingSoon />)
  expect(screen.getByRole('heading', { level: 1 }).textContent).toBe(
    'A watch history that follows you between players.',
  )
  expect(screen.getByRole('link', { name: 'GitHub' }).getAttribute('href')).toBe('https://github.com/tethrtv')
  expect(screen.getByRole('link', { name: 'hello@tethr.tv' }).getAttribute('href')).toBe('mailto:hello@tethr.tv')
})
