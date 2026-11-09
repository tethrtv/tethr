import { createRootRoute, createRoute, createRouter, Outlet, useLocation, useParams } from '@tanstack/react-router'
import { ComingSoon } from './ComingSoon'
import { cleanCode } from './components/CodeInput'
import { Plain } from './components/Plain'
import { Shell, useWatching } from './components/Shell'
import { Toasts } from './components/Toast'
import { Broken, LostSignal, NotFound } from './components/Trouble'
import { Activate } from './pages/Activate'
import { Authorize } from './pages/Authorize'
import { AutoListPage } from './pages/AutoList'
import { Browse } from './pages/Browse'
import { CheckEmail } from './pages/CheckEmail'
import { useMe } from './pages/connect'
import { Developers } from './pages/Developers'
import { History } from './pages/History'
import { Home } from './pages/Home'
import { Import } from './pages/Import'
import { Join } from './pages/Join'
import { DeveloperTerms, Privacy, Terms } from './pages/Legal'
import { ListAt } from './pages/List'
import { MediaServers } from './pages/MediaServers'
import { MyLists } from './pages/MyLists'
import { SignIn } from './pages/SignIn'
import { TitlePage } from './pages/Title'
import { UpNext } from './pages/UpNext'
import { UserPage } from './pages/User'
import { Verify } from './pages/Verify'
import { Watchlist } from './pages/Watchlist'
import { useLostSignal } from './signal'

function Root() {
  return useLostSignal() ? (
    <LostSignal />
  ) : (
    <Toasts>
      <Outlet />
    </Toasts>
  )
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
  const { username = '', slug = '' } = useParams({ strict: false })
  const me = useMe()
  if (me.isPending) return null
  if (me.data) {
    return (
      <Shell>
        <WatchingList key={`${username}/${slug}`} username={username} slug={slug} />
      </Shell>
    )
  }
  if (import.meta.env.VITE_COMING_SOON === 'true') return <ComingSoon />
  return (
    <Plain wide>
      <ListAt key={`${username}/${slug}`} username={username} slug={slug} />
    </Plain>
  )
}

function WatchingList({ username, slug }: { username: string; slug: string }) {
  const { profile } = useWatching()
  return <ListAt username={username} slug={slug} profileID={profile.id} profileKind={profile.kind} />
}

function OneUser() {
  const { username = '' } = useParams({ strict: false })
  const me = useMe()
  if (me.isPending) return null
  if (me.data) {
    return (
      <Shell>
        <WatchingUser key={username} username={username} />
      </Shell>
    )
  }
  if (import.meta.env.VITE_COMING_SOON === 'true') return <ComingSoon />
  return (
    <Plain wide>
      <UserPage key={username} username={username} />
    </Plain>
  )
}

function WatchingUser({ username }: { username: string }) {
  return <UserPage username={username} profileID={useWatching().profile.id} />
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
  createRoute({
    getParentRoute: () => root,
    path: '/lists/browse',
    component: () => (
      <Shell>
        <Browse />
      </Shell>
    ),
  }),
  createRoute({ getParentRoute: () => root, path: '/@{$username}', component: OneUser }),
  createRoute({ getParentRoute: () => root, path: '/@{$username}/lists/$slug', component: OneList }),
  createRoute({
    getParentRoute: () => root,
    path: '/lists/auto/$kind',
    component: function Auto() {
      const { kind = '' } = useParams({ strict: false })
      return (
        <Shell>
          <AutoListPage key={kind} kind={kind} />
        </Shell>
      )
    },
  }),
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
  createRoute({ getParentRoute: () => root, path: '/terms', component: Terms }),
  createRoute({ getParentRoute: () => root, path: '/developer-terms', component: DeveloperTerms }),
  createRoute({ getParentRoute: () => root, path: '/privacy', component: Privacy }),
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
