import { createRootRoute, createRoute, createRouter, Outlet } from '@tanstack/react-router'
import { CheckEmail } from './pages/CheckEmail'
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
  createRoute({
    getParentRoute: () => root,
    path: '/verify',
    component: Verify,
    validateSearch: (search: Record<string, unknown>) => ({
      token: typeof search.token === 'string' ? search.token : '',
    }),
  }),
]

export const router = createRouter({ routeTree: root.addChildren(routes) })

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}
