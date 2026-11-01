import { createRootRoute, createRoute, createRouter, Outlet } from '@tanstack/react-router'
import { cleanCode } from './components/CodeInput'
import { Activate } from './pages/Activate'
import { Authorize } from './pages/Authorize'
import { CheckEmail } from './pages/CheckEmail'
import { Developers } from './pages/Developers'
import { Home } from './pages/Home'
import { Join } from './pages/Join'
import { SignIn } from './pages/SignIn'
import { Verify } from './pages/Verify'

const root = createRootRoute({ component: Outlet })

const routes = [
  createRoute({ getParentRoute: () => root, path: '/', component: Home }),
  createRoute({ getParentRoute: () => root, path: '/sign-in', component: SignIn }),
  createRoute({
    getParentRoute: () => root,
    path: '/join',
    component: Join,
    validateSearch: (search: Record<string, unknown>): { invite?: string } =>
      typeof search.invite === 'string' ? { invite: search.invite.toUpperCase() } : {},
  }),
  createRoute({ getParentRoute: () => root, path: '/check-email', component: CheckEmail }),
  createRoute({ getParentRoute: () => root, path: '/developers', component: Developers }),
  createRoute({
    getParentRoute: () => root,
    path: '/verify',
    component: Verify,
    validateSearch: (search: Record<string, unknown>) => ({
      token: typeof search.token === 'string' ? search.token : '',
    }),
  }),
  createRoute({
    getParentRoute: () => root,
    path: '/activate',
    component: Activate,
    validateSearch: (search: Record<string, unknown>): { code?: string } =>
      typeof search.code === 'string' ? { code: cleanCode(search.code, 8, true) } : {},
  }),
  createRoute({
    getParentRoute: () => root,
    path: '/authorize',
    component: Authorize,
    validateSearch: (search: Record<string, unknown>) => {
      const text = (key: string) => (typeof search[key] === 'string' ? (search[key] as string) : undefined)
      return {
        client_id: text('client_id') ?? '',
        redirect_uri: text('redirect_uri') ?? '',
        state: text('state'),
        code_challenge: text('code_challenge'),
        code_challenge_method: text('code_challenge_method'),
      }
    },
  }),
]

export const router = createRouter({ routeTree: root.addChildren(routes) })

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}
