import '@fontsource-variable/archivo/wdth.css'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from '@tanstack/react-router'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { ComingSoon } from './ComingSoon'
import { router } from './router'
import './base.css'

const queries = new QueryClient({ defaultOptions: { queries: { retry: 1, refetchOnWindowFocus: false } } })

const root = document.getElementById('root')
if (root) {
  createRoot(root).render(
    <StrictMode>
      {import.meta.env.VITE_COMING_SOON === 'true' ? (
        <ComingSoon />
      ) : (
        <QueryClientProvider client={queries}>
          <RouterProvider router={router} />
        </QueryClientProvider>
      )}
    </StrictMode>,
  )
}
