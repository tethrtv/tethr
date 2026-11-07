import { createRootRoute, createRoute, createRouter, Outlet, useLocation, useParams } from '@tanstack/react-router'
import { ComingSoon } from './ComingSoon'
import { cleanCode } from './components/CodeInput'
import { Plain } from './components/Plain'
import { Shell, useWatching } from './components/Shell'
import { Broken, LostSignal, NotFound } from './components/Trouble'
import { Activate } from './pages/Activate'
import { Authorize } from './pages/Authorize'
import { CheckEmail } from './pages/CheckEmail'
import { useMe } from './pages/connect'
import { Developers } from './pages/Developers'
import { History } from './pages/History'
import { Home } from './pages/Home'
import { Import } from './pages/Import'
import { Join } from './pages/Join'
import { ListPage } from './pages/List'
import { MediaServers } from './pages/MediaServers'
import { MyLists } from './pages/MyLists'
import { SignIn } from './pages/SignIn'
import { TitlePage } from './pages/Title'
import { UpNext } from './pages/UpNext'
import { Verify } from './pages/Verify'
import { Watchlist } from './pages/Watchlist'
import { useLostSignal } from './signal'

function Root() {
  return useLostSignal() ? <LostSignal /> : <Outlet />
}

const root = createRootRoute({ component: Root })

const upNext = () => (
  <Shell>
    <UpNext />
  </Shell>
)

function Titled() {
  const { slug = '' } = useParams({ strict: false })
  const kind = useLocation().pathname.startsWith('/films/') ? 'movie' : 'show'
  return (
    <Shell>
      <TitlePage key={`${kind}/${slug}`} slug={slug} kind={kind} />
    </Shell>
  )
}

function OneList() {
  const { list = '' } = useParams({ strict: false })
  const me = useMe()
  if (me.isPending) return null
  if (me.data) {
    return (
      <Shell>
        <WatchingList key={list} param={list} />
      </Shell>
    )
  }
  if (import.meta.env.VITE_COMING_SOON === 'true') return <ComingSoon />
  return (
    <Plain wide>
      <ListPage key={list} param={list} />
    </Plain>
  )
}

function WatchingList({ param }: { param: string }) {
  return <ListPage param={param} profileID={useWatching().profile.id} />
}

function Front() {
  return useMe().data ? upNext() : <ComingSoon />
}

const routes = [
  createRoute({
    getParentRoute: () => root,
    path: '/',
    component: import.meta.env.VITE_COMING_SOON === 'true' ? Front : upNext,
  }),
  createRoute({
    getParentRoute: () => root,
    path: '/history',
    component: () => (
      <Shell>
        <History />
      </Shell>
    ),
  }),
  createRoute({
    getParentRoute: () => root,
    path: '/watchlist',
    component: () => (
      <Shell>
        <Watchlist />
      </Shell>
    ),
  }),
  createRoute({
    getParentRoute: () => root,
    path: '/lists',
    component: () => (
      <Shell>
        <MyLists />
      </Shell>
    ),
  }),
  createRoute({ getParentRoute: () => root, path: '/lists/$list', component: OneList }),
  createRoute({
    getParentRoute: () => root,
    path: '/shows/$slug',
    component: Titled,
  }),
  createRoute({
    getParentRoute: () => root,
    path: '/films/$slug',
    component: Titled,
  }),
  createRoute({ getParentRoute: () => root, path: '/account', component: Home }),
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
  createRoute({ getParentRoute: () => root, path: '/import', component: Import }),
  createRoute({ getParentRoute: () => root, path: '/media-servers', component: MediaServers }),
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

export const router = createRouter({
  routeTree: root.addChildren(routes),
  defaultNotFoundComponent: NotFound,
  defaultErrorComponent: Broken,
})

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}
