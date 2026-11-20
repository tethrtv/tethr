import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { expect, test, vi } from 'vitest'

const get = vi.fn(async () => ({ data: { app_name: 'Tethr for Jellyfin', official: true }, error: undefined }))

vi.mock('../api/client', () => ({ api: { GET: get, POST: vi.fn() }, problemMessage: () => 'problem' }))
vi.mock('@tanstack/react-router', () => ({
  useSearch: () => ({ code: 'ABCD2345' }),
  Link: ({ children }: { children: ReactNode }) => <a href="/">{children}</a>,
}))
vi.mock('./connect', () => ({
  useSignedIn: () => ({ profiles: [{ id: 1, name: 'Nathan' }] }),
  ProfilePicker: () => null,
}))

const { Activate } = await import('./Activate')

test('a code from a link waits for the person to say it is on their screen', async () => {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <Activate />
    </QueryClientProvider>,
  )
  expect((screen.getByLabelText('Code from your screen') as HTMLInputElement).value).toBe('ABCD2345')
  await new Promise((settle) => setTimeout(settle, 50))
  expect(get).not.toHaveBeenCalled()
  expect(screen.queryByRole('button', { name: /Connect/ })).toBeNull()

  await userEvent.click(screen.getByRole('button', { name: 'Carry on' }))
  await waitFor(() => expect(get).toHaveBeenCalledTimes(1))
  expect(await screen.findByRole('button', { name: 'Connect Tethr for Jellyfin' })).toBeTruthy()
})
