import {
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  redirect,
  useLocation,
  useParams,
  useSearch,
} from '@tanstack/react-router'
import { ComingSoon } from './ComingSoon'
import { cleanCode } from './components/CodeInput'
import { Plain } from './components/Plain'
import { Shell, useWatching } from './components/Shell'
import { Toasts } from './components/Toast'
import { Broken, LostSignal, NotFound } from './components/Trouble'
import { WatchDates } from './components/WatchDate'
import { Activate } from './pages/Activate'
import { Authorize } from './pages/Authorize'
import { AutoListPage } from './pages/AutoList'
import { Browse } from './pages/Browse'
import { CheckEmail } from './pages/CheckEmail'
import { useMe } from './pages/connect'
import { Developers } from './pages/Developers'
import { History } from './pages/History'
import { Home, isAccountSection } from './pages/Home'
import { Join } from './pages/Join'
import { DeveloperTerms, Privacy, Terms } from './pages/Legal'
import { ListAt } from './pages/List'
import { MediaServers } from './pages/MediaServers'
import { MyLists } from './pages/MyLists'
import { RuleListPage, type RuleSearch } from './pages/RuleList'
import { SignIn } from './pages/SignIn'
import { TitlePage } from './pages/Title'
import { Today } from './pages/Today'
import { Unsubscribe } from './pages/Unsubscribe'
import { UpNext } from './pages/UpNext'
import { UserPage } from './pages/User'
import { Verify } from './pages/Verify'
import { Watchlist } from './pages/Watchlist'
import { YearPage } from './pages/Year'
import { useLostSignal } from './signal'

function Root() {
  return useLostSignal() ? (
    <LostSignal />
  ) : (
    <Toasts>
      <WatchDates>
        <Outlet />
      </WatchDates>
    </Toasts>
  )
}

const root = createRootRoute({ component: Root })

const upNext = () => (
  <Shell>
    <UpNext />
  </Shell>
)

const today = () => (
  <Shell>
    <Today />
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
  return useMe().data ? today() : <ComingSoon />
}

function AccountAt() {
  const { section = '' } = useParams({ strict: false })
  if (!isAccountSection(section)) return <NotFound />
  return (
    <Shell>
      <Home section={section} />
    </Shell>
  )
}

const account = () => (
  <Shell>
    <Home />
  </Shell>
)

const mediaServers = () => (
  <Shell>
    <MediaServers />
  </Shell>
)

function OneYear() {
  const { year = '' } = useParams({ strict: false })
  const n = Number(year)
  if (!Number.isInteger(n) || n <= 1900) return <NotFound />
  return (
    <Shell>
      <YearPage key={n} year={n} />
    </Shell>
  )
}

const routes = [
  createRoute({
    getParentRoute: () => root,
    path: '/',
    component: import.meta.env.VITE_COMING_SOON === 'true' ? Front : today,
  }),
  createRoute({ getParentRoute: () => root, path: '/up-next', component: upNext }),
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
    path: '/year',
    component: () => (
      <Shell>
        <YearPage year={new Date().getFullYear()} />
      </Shell>
    ),
  }),
  createRoute({ getParentRoute: () => root, path: '/year/$year', component: OneYear }),
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
    path: '/lists/rule',
    validateSearch: (search: Record<string, unknown>): RuleSearch => {
      const text = (key: string) =>
        typeof search[key] === 'string' && search[key] ? (search[key] as string) : undefined
      const list = Number(search.list)
      return {
        list: Number.isInteger(list) && list > 0 ? list : undefined,
        kind: search.kind === 'movie' || search.kind === 'show' ? search.kind : undefined,
        genre: text('genre'),
        type: text('type'),
        studio: text('studio'),
        network: text('network'),
      }
    },
    component: function Rule() {
      const search = useSearch({ from: '/lists/rule' })
      return (
        <Shell>
          <RuleListPage key={search.list ?? JSON.stringify(search)} search={search} />
        </Shell>
      )
    },
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
  createRoute({ getParentRoute: () => root, path: '/account', component: account }),
  createRoute({ getParentRoute: () => root, path: '/account/$section', component: AccountAt }),
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
  createRoute({
    getParentRoute: () => root,
    path: '/import',
    beforeLoad: () => {
      throw redirect({ to: '/account/$section', params: { section: 'import' }, replace: true })
    },
  }),
  createRoute({ getParentRoute: () => root, path: '/media-servers', component: mediaServers }),
  createRoute({
    getParentRoute: () => root,
    path: '/unsubscribe',
    component: Unsubscribe,
    validateSearch: (search: Record<string, unknown>) => ({
      token: typeof search.token === 'string' ? search.token : '',
    }),
  }),
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
