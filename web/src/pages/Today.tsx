import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { type ReactNode, useEffect, useState } from 'react'
import { api } from '../api/client'
import type { components } from '../api/schema'
import { Cover } from '../components/Cover'
import { ListCover } from '../components/ListCover'
import { useWatching } from '../components/Shell'
import { Television } from '../components/Television'
import { TitleLink } from '../components/TitleLink'
import { ago, ep, playedAgo, plural } from '../format'
import { image, useDetails } from '../tmdb'
import { listParams, ownerName } from './MyLists'
import styles from './Today.module.css'
import { useYear } from './Year'

type Title = components['schemas']['Title']
type Profile = components['schemas']['Me']['profiles'][number]

type Pick = { key: string; kind: 'movie' | 'show'; title: Title; detail: string }

const kindOf = (t: { type?: string }) => (t.type === 'movie' ? 'movie' : 'show')
const hourName = (h: number) => (h === 0 ? 'midnight' : h === 12 ? 'noon' : h < 12 ? `${h}am` : `${h - 12}pm`)

async function load<T>(request: Promise<{ data?: T; error?: unknown }>) {
  const { data, error } = await request
  if (error) throw error
  return data as T
}

function airsIn(day: string) {
  const now = new Date()
  return Math.round((Date.parse(day) - Date.UTC(now.getFullYear(), now.getMonth(), now.getDate())) / 86400000)
}

function airsOn(days: number, day: string) {
  if (days === 0) return 'Tonight'
  if (days === 1) return 'Tomorrow'
  return new Date(`${day}T12:00:00Z`).toLocaleDateString('en-GB', { weekday: 'long' })
}

export function Today() {
  const { me, profile } = useWatching()
  const params = { query: { profile_id: profile.id } }
  const paused = useQuery({
    queryKey: ['playback', profile.id],
    queryFn: () => load(api.GET('/v1/playback', { params })),
  })
  const next = useQuery({
    queryKey: ['up-next', profile.id],
    queryFn: () => load(api.GET('/v1/up-next', { params })),
  })
  const lists = useQuery({
    queryKey: ['up-next-lists', profile.id],
    queryFn: () => load(api.GET('/v1/up-next/lists', { params })),
  })
  const upcoming = useQuery({
    queryKey: ['upcoming', profile.id],
    queryFn: () => load(api.GET('/v1/upcoming', { params })),
  })
  const recent = useQuery({
    queryKey: ['history', profile.id, 'today'],
    queryFn: () => load(api.GET('/v1/history', { params: { query: { profile_id: profile.id, limit: 200 } } })),
  })
  const now = new Date()
  const year = useYear(profile.id, now.getFullYear())

  const heading = `Today, ${now.toLocaleDateString('en-GB', { weekday: 'long', day: 'numeric', month: 'long' })}`
  const loading = paused.isPending || next.isPending || recent.isPending
  const failed = paused.isError || next.isError || recent.isError
  const fresh = !loading && !failed && recent.data?.total === 0 && !paused.data?.length && !next.data?.length

  const picks: Pick[] = []
  const seen = new Set<number>()
  for (const p of paused.data ?? []) {
    const id = p.item.title.ids.tethr ?? 0
    if (seen.has(id)) continue
    seen.add(id)
    const where = p.item.episode ? `${ep(p.item.episode.season, p.item.episode.number)}, ` : ''
    picks.push({
      key: `paused-${p.id}`,
      kind: kindOf(p.item),
      title: p.item.title,
      detail: `${where}paused ${Math.round(p.progress)}% in`,
    })
  }
  for (const e of next.data ?? []) {
    if (picks.length >= 4) break
    const id = e.show.ids.tethr ?? 0
    if (seen.has(id)) continue
    seen.add(id)
    picks.push({
      key: `next-${id}`,
      kind: 'show',
      title: e.show,
      detail: `Next is ${ep(e.next.season, e.next.number)}`,
    })
  }

  const week = (upcoming.data ?? []).filter((u) => {
    const days = u.episode.first_aired ? airsIn(u.episode.first_aired) : -1
    return days >= 0 && days < 7
  })
  const others = me.profiles.filter((p) => p.id !== profile.id)
  const hours = year.data?.hours ?? []
  const peak = hours.some((n) => n > 0) ? hours.indexOf(Math.max(...hours)) : -1

  return (
    <div className={styles.page}>
      <title>Today · Tethr</title>
      <header className={styles.head}>
        <div className={styles.words}>
          <h1 className={styles.title}>{heading}</h1>
          {!loading && (
            <p className={styles.lead}>
              {failed ? (
                'Tethr couldn’t load everything for today. Try again in a moment.'
              ) : fresh ? (
                'Nothing on yet. Bring your history over or connect a player, and this page fills up with what to watch next.'
              ) : picks[0] ? (
                <Tonight pick={picks[0]} peak={peak} />
              ) : (
                'Nothing on the go. Start something in any connected app and it’ll show up here.'
              )}
            </p>
          )}
        </div>
        <Room pick={fresh ? undefined : picks[0]} />
      </header>

      {fresh && <GetStarted />}

      {picks.length > 0 && (
        <Block title="Pick up where you left off" to="/up-next" more="All of Up next">
          {picks.slice(0, 4).map((p) => (
            <Row key={p.key} kind={p.kind} title={p.title} detail={p.detail} />
          ))}
        </Block>
      )}

      {week.length > 0 && (
        <Block title="On this week" to="/up-next" more="Everything coming up">
          {week.slice(0, 5).map((u) => {
            const day = u.episode.first_aired ?? ''
            return (
              <Row
                key={u.episode.ids.tethr}
                kind="show"
                title={u.show}
                detail={`${airsOn(airsIn(day), day)}, ${ep(u.episode.season, u.episode.number)}${u.watchlist ? ', from your watchlist' : ''}`}
              />
            )
          })}
        </Block>
      )}

      {!!lists.data?.length && (
        <Block title="From your lists" to="/lists" more="Your lists">
          {lists.data.slice(0, 4).map((l) => {
            const film = l.next.type === 'movie'
            const where = `${film ? 'Film' : 'Show'} ${l.position} of ${l.list.item_count ?? '?'} in ${l.list.name}`
            const episode = l.next.episode ? `, ${ep(l.next.episode.season, l.next.episode.number)}` : ''
            return (
              <Row key={l.list.id} kind={film ? 'movie' : 'show'} title={l.next.title} detail={`${where}${episode}`} />
            )
          })}
        </Block>
      )}

      <OnYourWatchlist profileID={profile.id} skip={new Set([...seen, ...week.map((u) => u.show.ids.tethr ?? 0)])} />

      {others.length > 0 && (
        <Block title="Household">
          {others.map((p) => (
            <Housemate key={p.id} profile={p} />
          ))}
        </Block>
      )}

      {year.data && year.data.plays > 0 && recent.data && <Month y={year.data} recent={recent.data} />}

      <PopularOnTethr profileID={profile.id} />
      <ListsPeopleFollow profileID={profile.id} />
    </div>
  )
}

function Tonight({ pick, peak }: { pick: Pick; peak: number }) {
  const meta = useDetails(pick.kind, pick.title.ids.tmdb)
  const name = meta?.name ?? pick.title.name ?? 'Untitled'
  return (
    <>
      {peak >= 0 ? `You usually press play around ${hourName(peak)}, and` : 'Next up is'} {peak >= 0 && 'next up is '}
      <strong>{name}</strong>
      {pick.detail.startsWith('Next is ') ? `, ${pick.detail.slice(8)}` : `, ${pick.detail}`}.
    </>
  )
}

function useEvening() {
  const [hour, setHour] = useState(() => new Date().getHours())
  useEffect(() => {
    const tick = setInterval(() => setHour(new Date().getHours()), 5 * 60 * 1000)
    return () => clearInterval(tick)
  }, [])
  return hour >= 17 || hour < 6
}

function Room({ pick }: { pick?: Pick }) {
  const meta = useDetails(pick?.kind ?? 'show', pick?.title.ids.tmdb)
  const name = pick ? (meta?.name ?? pick.title.name ?? 'Untitled') : 'Nothing on yet'
  const evening = useEvening()
  const [on, setOn] = useState(false)
  useEffect(() => {
    const t = setTimeout(() => setOn(true), 150)
    return () => clearTimeout(t)
  }, [])
  const label = `A television on a sideboard${pick ? ` showing ${name}` : ''}, with a lamp ${evening ? 'on and the moon' : 'off and the sun'} in the window.`
  const art = (
    <svg
      viewBox="0 0 440 300"
      className={evening ? `${styles.room} ${styles.evening}` : styles.room}
      role="img"
      aria-label={label}
    >
      <g className={styles.window}>
        <rect x="18" y="26" width="112" height="132" rx="3" />
        <path d="M74 26v132M18 92h112" />
        {evening ? (
          <>
            <path d="M50 46a14 14 0 1 0 14 20a11 11 0 1 1 -14 -20z" className={styles.moon} />
            <path d="M100 48v8M96 52h8M108 112v6M105 115h6" />
          </>
        ) : (
          <>
            <circle cx="48" cy="58" r="11" className={styles.sun} />
            <path d="M48 40v-5M48 76v5M30 58h-5M66 58h5M35 45l-3-3M61 71l3 3M61 45l3-3M35 71l-3 3" />
          </>
        )}
      </g>
      <g className={on ? styles.lit : undefined}>
        <path d="M398 272v-136M384 272h28" />
        <path d="M374 136h48l-10 -40h-28z" className={styles.shade} />
        <path d="M384 148l-8 14M398 150v16M412 148l8 14" className={styles.rays} />
      </g>
      <path d="M8 272h424" className={styles.floor} />
      <g className={styles.sideboard}>
        <rect x="132" y="200" width="214" height="56" rx="3" />
        <path d="M239 200v56M150 228h70M258 228h70M146 256l-4 16M332 256l4 16" />
      </g>
      <g transform="translate(150 38) scale(0.75)">
        <Television still={image(meta?.backdrop, 'w300')} poster={image(meta?.poster, 'w342')} name={name} on={on} />
      </g>
    </svg>
  )
  if (!pick?.title.slug) return <div className={styles.art}>{art}</div>
  return (
    <TitleLink type={pick.kind} slug={pick.title.slug} className={styles.art}>
      {art}
    </TitleLink>
  )
}

function Block({ title, to, more, children }: { title: string; to?: string; more?: string; children: ReactNode }) {
  return (
    <section className={styles.section}>
      <h2 className={styles.sectionTitle}>{title}</h2>
      <ol className={styles.rows}>{children}</ol>
      {to && more && (
        <Link to={to} className={styles.more}>
          {more}
        </Link>
      )}
    </section>
  )
}

function Row({ kind, title, detail }: { kind: 'movie' | 'show'; title: Title; detail: string }) {
  const meta = useDetails(kind, title.ids.tmdb)
  const name = meta?.name ?? title.name ?? 'Untitled'
  return (
    <li className={styles.row}>
      <Cover id={title.ids.tethr ?? 0} name={name} poster={meta?.poster} size="xs" />
      <TitleLink type={kind} slug={title.slug} className={styles.name}>
        {name}
      </TitleLink>
      <span className={styles.detail}>{detail}</span>
    </li>
  )
}

function Housemate({ profile }: { profile: Profile }) {
  const last = useQuery({
    queryKey: ['history', profile.id, 'latest'],
    queryFn: async () =>
      (await load(api.GET('/v1/history', { params: { query: { profile_id: profile.id, limit: 1 } } }))).items[0] ??
      null,
  })
  const w = last.data
  const kind = w ? kindOf(w.item) : 'show'
  const meta = useDetails(kind, w?.item.title.ids.tmdb)
  if (last.isPending || last.isError) return null
  if (!w) {
    return (
      <li className={styles.row}>
        <span className={styles.initial} aria-hidden="true">
          {profile.name.slice(0, 1)}
        </span>
        <span className={styles.name}>{profile.name}</span>
        <span className={styles.detail}>Hasn’t watched anything yet</span>
      </li>
    )
  }
  const name = meta?.name ?? w.item.title.name ?? 'Untitled'
  return (
    <li className={styles.row}>
      <span className={styles.initial} aria-hidden="true">
        {profile.name.slice(0, 1)}
      </span>
      <span className={styles.sentence}>
        <strong>{profile.name}</strong> watched{' '}
        <TitleLink type={kind} slug={w.item.title.slug} className={styles.inline}>
          {name}
        </TitleLink>
        {w.item.episode ? `, ${ep(w.item.episode.season, w.item.episode.number)}` : ''}
      </span>
      <span className={styles.detail}>{playedAgo(w)}</span>
    </li>
  )
}

function Month({
  y,
  recent,
}: {
  y: components['schemas']['Year']
  recent: { items: components['schemas']['Watch'][]; total: number }
}) {
  const now = new Date()
  const first = Math.round(
    (Date.UTC(now.getFullYear(), now.getMonth(), 1) - Date.UTC(now.getFullYear(), 0, 1)) / 86400000,
  )
  const days = y.days.slice(first)
  const plays = days.reduce((n, d) => n + d.plays, 0)
  const counts = new Map<number, { n: number; watch: components['schemas']['Watch'] }>()
  for (const w of recent.items) {
    const at = new Date(w.watched_at)
    if (w.precision === 'none' || at.getMonth() !== now.getMonth() || at.getFullYear() !== now.getFullYear()) continue
    const id = w.item.title.ids.tethr ?? 0
    const c = counts.get(id)
    counts.set(id, { n: (c?.n ?? 0) + 1, watch: c?.watch ?? w })
  }
  const oldest = recent.items[recent.items.length - 1]
  const whole =
    recent.total <= recent.items.length || (!!oldest && new Date(oldest.watched_at).getMonth() !== now.getMonth())
  const leader = [...counts.values()].sort((a, b) => b.n - a.n)[0]
  const top = whole && leader && leader.n > 1 && leader.n * 3 >= plays ? leader.watch : undefined
  const kind = top ? kindOf(top.item) : 'show'
  const meta = useDetails(kind, top?.item.title.ids.tmdb)
  if (plays === 0) return null
  const month = now.toLocaleDateString('en-GB', { month: 'long' })
  return (
    <section className={styles.section}>
      <h2 className={styles.sectionTitle}>{month} so far</h2>
      <p className={styles.month}>
        <strong>{plural(plays, 'play')}</strong> this month
        {top && (
          <>
            , mostly{' '}
            <TitleLink type={kind} slug={top.item.title.slug} className={styles.inline}>
              {meta?.name ?? top.item.title.name ?? 'Untitled'}
            </TitleLink>
          </>
        )}
        .
      </p>
      <Link to="/year" className={styles.more}>
        {y.year} so far
      </Link>
    </section>
  )
}

function GetStarted() {
  return (
    <section className={styles.section}>
      <h2 className={styles.sectionTitle}>Getting started</h2>
      <ol className={styles.steps}>
        <li>
          <strong>Bring your history.</strong> Tethr reads the export file from your old tracker.{' '}
          <Link to="/import" className={styles.inline}>
            Import a file
          </Link>
        </li>
        <li>
          <strong>Connect a player.</strong> Sign in on your TV and enter the code it shows, or log plays from your own
          media server.{' '}
          <Link to="/activate" className={styles.inline}>
            Enter a TV code
          </Link>{' '}
          or{' '}
          <Link to="/media-servers" className={styles.inline}>
            set up a media server
          </Link>
        </li>
      </ol>
    </section>
  )
}

const daySeed = () => {
  const d = new Date()
  return d.getFullYear() * 400 + d.getMonth() * 32 + d.getDate()
}

const shuffleKey = (id: number, seed: number) => ((id ^ seed) * 2654435761) % 4294967296

function OnYourWatchlist({ profileID, skip }: { profileID: number; skip: Set<number> }) {
  const listed = useQuery({
    queryKey: ['watchlist', profileID],
    queryFn: () => load(api.GET('/v1/watchlist', { params: { query: { profile_id: profileID } } })),
  })
  const seed = daySeed()
  const titles = new Set<number>(skip)
  const picks = [...(listed.data ?? [])]
    .sort((a, b) => shuffleKey(a.id, seed) - shuffleKey(b.id, seed))
    .filter((l) => {
      const id = l.item.title.ids.tethr ?? 0
      if (titles.has(id)) return false
      titles.add(id)
      return true
    })
    .slice(0, 3)
  if (!picks.length) return null
  return (
    <Block title="From your watchlist" to="/watchlist" more="Your watchlist">
      {picks.map((l) => (
        <Row key={l.id} kind={kindOf(l.item)} title={l.item.title} detail={`Added ${ago(l.listed_at)}`} />
      ))}
    </Block>
  )
}

function PopularOnTethr({ profileID }: { profileID: number }) {
  const popular = useQuery({
    queryKey: ['popular', profileID],
    queryFn: () => load(api.GET('/v1/popular', { params: { query: { profile_id: profileID } } })),
  })
  if (!popular.data?.length) return null
  return (
    <Block title="Popular on Tethr this week">
      {popular.data.slice(0, 5).map((p) => (
        <Row
          key={p.item.title.ids.tethr}
          kind={kindOf(p.item)}
          title={p.item.title}
          detail={`${plural(p.households, 'household')} watched`}
        />
      ))}
    </Block>
  )
}

const listFloor = 3

function ListsPeopleFollow({ profileID }: { profileID: number }) {
  const lists = useQuery({
    queryKey: ['public-lists', profileID, 'popular'],
    queryFn: () =>
      load(api.GET('/v1/public-lists', { params: { query: { profile_id: profileID, order: 'popular', limit: 6 } } })),
  })
  const followed = (lists.data?.lists ?? []).filter((l) => l.follower_count > 1 && l.access !== 'mine')
  if (followed.length < listFloor) return null
  return (
    <Block title="Lists people follow" to="/lists/browse" more="Browse lists">
      {followed.slice(0, 3).map((l) => (
        <li key={l.id} className={`${styles.row} ${styles.listRow}`}>
          <ListCover items={l.cover ?? []} />
          <Link to="/@{$username}/lists/$slug" params={listParams(l)} className={styles.name}>
            {l.name}
          </Link>
          <span className={styles.detail}>
            From {ownerName(l)}, {plural(l.follower_count, 'follower')}
          </span>
        </li>
      ))}
    </Block>
  )
}
