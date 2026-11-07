import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useCanGoBack, useNavigate, useRouter } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { api, problemMessage } from '../api/client'
import type { components } from '../api/schema'
import { Button } from '../components/Button'
import { Cover } from '../components/Cover'
import { Field } from '../components/Field'
import { useWatching } from '../components/Shell'
import { Slip, SlipBody, SlipFoot } from '../components/Slip'
import { useToast } from '../components/Toast'
import { ago, date, duration, ep, plural } from '../format'
import { type Details, type Group, type Season, useDetails, useGroup, useOrders, useSeason } from '../tmdb'
import lists from './Lists.module.css'
import { ownerName, privacyWords } from './MyLists'
import styles from './Title.module.css'

type Detail = components['schemas']['TitleDetail']
type EpisodeDetail = components['schemas']['EpisodeDetail']
type Play = components['schemas']['Play']
type ItemRef = components['schemas']['ItemRef']

type Kind = 'movie' | 'show'

const today = () => new Date().toISOString().slice(0, 10)
const titleRef = (d: Detail): ItemRef => ({
  type: d.type,
  ids: d.title.ids.tethr ? { tethr: d.title.ids.tethr } : { tmdb: d.title.ids.tmdb },
})
const episodeRef = (show: Detail, e: EpisodeDetail): ItemRef =>
  e.episode.ids.tethr
    ? { type: 'episode', ids: { tethr: e.episode.ids.tethr } }
    : { type: 'episode', show: { tmdb: show.title.ids.tmdb }, season: e.episode.season, number: e.episode.number }
const shown = (e: EpisodeDetail) => e.episode.order ?? e.episode
const coverID = (d: Detail) => d.title.ids.tethr ?? d.title.ids.tmdb ?? 0
const aired = (e: EpisodeDetail) =>
  e.watches.length > 0 || (!!e.episode.first_aired && e.episode.first_aired <= today())
const how = (p: Play) =>
  p.action === 'scrobble' ? 'Played in an app' : p.action === 'checkin' ? 'Checked in' : 'Added by hand'
const names = (list?: string[]) => (list?.length ? new Intl.ListFormat('en-GB').format(list) : undefined)
const day = (iso: string) =>
  new Date(iso).toLocaleDateString('en-GB', { weekday: 'short', day: 'numeric', month: 'short' })
const statuses: Record<string, string> = {
  'Returning Series': 'Returning',
  Ended: 'Ended',
  Canceled: 'Cancelled',
  'In Production': 'In production',
  Planned: 'Planned',
  Pilot: 'Pilot',
}

function rated(meta?: Details | null) {
  const c = meta?.certificate
  return c && (c.country === 'GB' ? c.rating : `${c.rating} (${c.country})`)
}

function status(meta?: Details | null) {
  const word = meta?.status && statuses[meta.status]
  const next = meta?.nextEpisode
  if (!word || !next?.airDate) return word
  return `${word}, ${ep(next.season, next.number)} on ${day(next.airDate)}`
}

export function TitlePage({ slug, kind }: { slug: string; kind: Kind }) {
  const { profile } = useWatching()
  const navigate = useNavigate()
  const tmdb = Number(/^tmdb-(\d+)$/.exec(slug)?.[1]) || undefined
  const title = useQuery({
    queryKey: ['title', profile.id, slug],
    queryFn: async () => {
      const { data, response } = await api.GET('/v1/titles/{id}', {
        params: { path: { id: slug }, query: { profile_id: profile.id, type: tmdb ? kind : undefined } },
      })
      if (response.status === 404) return null
      if (!data) throw new Error('could not load the title')
      return data
    },
  })
  const unseen = useDetails(kind, title.data === null ? tmdb : undefined)
  const d: Detail | undefined =
    title.data ?? (unseen ? { type: kind, title: { ids: { tmdb } }, watches: [] } : undefined)
  const wanted = d?.title.slug

  useEffect(() => {
    if (!d || !wanted || wanted === slug || (tmdb && wanted.startsWith('t-'))) return
    navigate({ to: d.type === 'movie' ? '/films/$slug' : '/shows/$slug', params: { slug: wanted }, replace: true })
  }, [d, wanted, slug, tmdb, navigate])

  if (title.data === null && (!tmdb || unseen === null)) {
    return (
      <div className={lists.page}>
        <Back />
        <header className={lists.head}>
          <h1 className={lists.title}>Not found</h1>
          <p className={lists.lead}>There’s no film or show here. It may have been merged into another one.</p>
        </header>
      </div>
    )
  }
  if (!d) return null
  return d.type === 'show' ? <Show detail={d} profileID={profile.id} /> : <Film detail={d} profileID={profile.id} />
}

function Back() {
  const router = useRouter()
  if (!useCanGoBack()) {
    return (
      <Link to="/" className={`${lists.textButton} ${styles.back}`}>
        Up next
      </Link>
    )
  }
  return (
    <button type="button" className={`${lists.textButton} ${styles.back}`} onClick={() => router.history.back()}>
      Back
    </button>
  )
}

function useRefresh(profileID: number) {
  const queries = useQueryClient()
  return () => {
    for (const key of ['title', 'history', 'up-next', 'playback', 'watchlist', 'lists']) {
      queries.invalidateQueries({ queryKey: [key, profileID] })
    }
    queries.invalidateQueries({ queryKey: ['list'] })
  }
}

function useLog(profileID: number) {
  const refresh = useRefresh(profileID)
  return useMutation({
    mutationFn: async (items: ItemRef[]) => {
      const { error } = await api.POST('/v1/history', {
        params: { query: { profile_id: profileID } },
        body: { items: items.map((item) => ({ item })) },
      })
      if (error) throw error
    },
    onSuccess: refresh,
  })
}

function useUnlog(profileID: number) {
  const refresh = useRefresh(profileID)
  return useMutation({
    mutationFn: async (plays: Play[]) => {
      for (const p of plays) {
        const { error } = await api.DELETE('/v1/history/{id}', {
          params: { path: { id: p.id }, query: { profile_id: profileID } },
        })
        if (error) throw error
      }
    },
    onSuccess: refresh,
  })
}

function Facts({ facts }: { facts: [string, string | undefined][] }) {
  const shown = facts.filter((f): f is [string, string] => !!f[1])
  if (!shown.length) return null
  return (
    <dl className={styles.facts}>
      {shown.map(([label, value]) => (
        <div key={label}>
          <dt>{label}</dt>
          <dd>{value}</dd>
        </div>
      ))}
    </dl>
  )
}

function About({ meta, made }: { meta?: Details | null; made: string }) {
  const by = names(meta?.creators)
  const cast = names(meta?.cast)
  if (!meta || (!meta.overview && !by && !cast)) return null
  return (
    <div className={styles.about}>
      {meta.tagline && <p className={styles.tagline}>{meta.tagline}</p>}
      {meta.overview && <p className={styles.overview}>{meta.overview}</p>}
      {(by || cast) && (
        <p className={styles.credits}>
          {by && `${made} ${by}. `}
          {cast && `With ${cast}.`}
        </p>
      )}
    </div>
  )
}

function Show({ detail: d, profileID }: { detail: Detail; profileID: number }) {
  const meta = useDetails('show', d.title.ids.tmdb)
  const group = useGroup(d.title.ids.tmdb, d.episode_order)
  const name = meta?.name ?? d.title.name ?? 'Untitled show'
  const seasons = d.seasons ?? []
  const numbered = seasons.filter((s) => s.number > 0)
  const episodes = numbered.flatMap((s) => s.episodes)
  const out = episodes.filter(aired)
  const seen = out.filter((e) => e.watches.length).length
  const left = out.length - seen
  const paused = episodes.find((e) => e.resume && !e.watches.length)
  const sizes = d.episode_order
    ? group?.seasons.map((s) => ({ number: s.number, episodes: s.episodes.length }))
    : meta?.seasons
  const counts = (sizes ?? []).filter((s) => s.episodes > 0)
  const listed = counts.filter((s) => s.number > 0)
  const total = listed.reduce((sum, s) => sum + s.episodes, 0)
  const outlined = !sizes || episodes.length >= total
  const numbers = [...new Set([...seasons.map((s) => s.number), ...counts.map((s) => s.number)])].sort(
    (a, b) => Number(a === 0) - Number(b === 0) || a - b,
  )
  const firstGap = numbered.find((s) => s.episodes.some((e) => aired(e) && !e.watches.length))
  const [chosen, setChosen] = useState<number>()
  const current =
    chosen ??
    firstGap?.number ??
    (outlined ? numbered[numbered.length - 1]?.number : undefined) ??
    numbers.find((n) => n > 0) ??
    numbers[0]
  const runtime = d.title.runtime_minutes ?? meta?.runtime
  const behind = numbered.some(
    (s) => current !== undefined && s.number < current && s.episodes.some((e) => aired(e) && !e.watches.length),
  )
  useOutline(profileID, !outlined && seen > 0)

  return (
    <div className={`${lists.page} ${styles.page}`}>
      <Back />
      <header className={styles.masthead}>
        <Cover id={coverID(d)} name={name} poster={meta?.poster} size="l" />
        <div className={styles.body}>
          <h1 className={lists.title}>{name}</h1>
          <About meta={meta} made="Created by" />
          <Facts
            facts={[
              ['Original title', d.title.original_name !== name ? d.title.original_name : undefined],
              ['First aired', (d.title.year ?? meta?.year)?.toString()],
              [
                'Episodes',
                outlined && episodes.length
                  ? `${episodes.length} in ${plural(numbered.length, 'season')}`
                  : total
                    ? `${total} in ${plural(listed.length, 'season')}`
                    : undefined,
              ],
              ['Runtime', runtime ? `About ${duration(runtime)} each` : undefined],
              ['Network', names(meta?.networks)],
              ['Genre', meta?.genres?.join(', ')],
              ['Rated', rated(meta)],
              ['Status', status(meta)],
            ]}
          />
          <div className={styles.progress}>
            <p className={lists.lead}>
              {!outlined && seen ? (
                <>
                  You’ve seen <strong>{seen}</strong> so far. Tethr is fetching the rest of the episode list.
                </>
              ) : !out.length ? (
                'You haven’t started this yet.'
              ) : !left ? (
                'You’ve seen every episode that’s aired.'
              ) : (
                <>
                  You’ve seen <strong>{seen}</strong> of {out.length}. {left} to go
                  {runtime ? `, about ${duration(left * runtime)}` : ''}.
                </>
              )}
              {paused &&
                ` Paused in ${ep(shown(paused).season, shown(paused).number)}, ${Math.round(paused.resume?.progress ?? 0)}% in.`}
            </p>
            {outlined && !!numbered.length && (
              <div className={styles.tally} aria-hidden="true">
                {numbered.map((s) => (
                  <span key={s.number} className={styles.tallyGroup}>
                    {s.episodes.map((e) => (
                      <span
                        key={e.episode.ids.tethr}
                        className={`${styles.tick} ${e.watches.length ? styles.done : ''} ${aired(e) ? '' : styles.later}`}
                      />
                    ))}
                    <span className={`${lists.code} ${styles.tallyLabel}`}>S{s.number}</span>
                  </span>
                ))}
              </div>
            )}
          </div>
          <Yours detail={d} profileID={profileID} />
        </div>
      </header>

      {current !== undefined && (
        <section className={styles.episodes}>
          <OrderPicker show={d} profileID={profileID} onPick={() => setChosen(undefined)} />
          <div className={lists.filters}>
            <span className={lists.faint}>Season</span>
            <fieldset className={lists.options}>
              <legend className={lists.hidden}>Season</legend>
              {numbers.map((n) => (
                <label key={n} className={n === current ? `${lists.option} ${lists.chosen}` : lists.option}>
                  <input type="radio" name="season" checked={n === current} onChange={() => setChosen(n)} />
                  {n ? n : 'Specials'}
                </label>
              ))}
            </fieldset>
          </div>
          <Episodes
            show={d}
            episodes={seasons.find((s) => s.number === current)?.episodes ?? []}
            season={current}
            group={d.episode_order ? (group ?? undefined) : undefined}
            behind={behind}
            profileID={profileID}
          />
        </section>
      )}
    </div>
  )
}

function Episodes({
  show,
  episodes,
  season,
  group,
  behind,
  profileID,
}: {
  show: Detail
  episodes: EpisodeDetail[]
  season: number
  group?: Group
  behind: boolean
  profileID: number
}) {
  const broadcast = useSeason(show.title.ids.tmdb, show.episode_order ? undefined : season)
  const part = group?.seasons.find((s) => s.number === season)
  const info = show.episode_order ? part && { overview: '', episodes: part.episodes } : broadcast
  const log = useLog(profileID)
  const rows = merge(episodes, info ?? undefined, season)
  const rest = rows.filter((e) => aired(e) && !e.watches.length)
  return (
    <>
      {info?.overview && (
        <Synopsis
          text={info.overview}
          hidden={behind}
          reveal={`Read about ${season ? `season ${season}` : 'the specials'}`}
          className={styles.seasonBlurb}
        />
      )}
      <EpisodeTable show={show} episodes={rows} info={info ?? undefined} profileID={profileID} />
      {rest.length > 1 && (
        <p>
          <Button onClick={() => log.mutate(rest.map((e) => episodeRef(show, e)))} disabled={log.isPending}>
            Tick off the rest of {season ? `season ${season}` : 'the specials'}
          </Button>
        </p>
      )}
      {log.isError && <p className={lists.error}>{problemMessage(log.error)}</p>}
    </>
  )
}

function merge(server: EpisodeDetail[], info: Season | undefined, season: number): EpisodeDetail[] {
  if (!info) return server
  const known = new Map(server.map((e) => [shown(e).number, e]))
  const rows = info.episodes.map((m): EpisodeDetail => {
    const first_aired = m.air_date ?? undefined
    const e = known.get(m.number)
    if (!e) {
      const order = m.aired && { season, number: m.number }
      return { episode: { ids: {}, ...(m.aired ?? { season, number: m.number }), order, first_aired }, watches: [] }
    }
    return e.episode.first_aired ? e : { ...e, episode: { ...e.episode, first_aired } }
  })
  const listed = new Set(info.episodes.map((m) => m.number))
  return [...rows, ...server.filter((e) => !listed.has(shown(e).number))].sort(
    (a, b) => shown(a).number - shown(b).number,
  )
}

function OrderPicker({ show, profileID, onPick }: { show: Detail; profileID: number; onPick: () => void }) {
  const orders = useOrders(show.title.ids.tmdb)
  const refresh = useRefresh(profileID)
  const pick = useMutation({
    mutationFn: async (group: string) => {
      const path = { id: String(show.title.ids.tethr) }
      const query = { profile_id: profileID }
      const { error } = group
        ? await api.PUT('/v1/titles/{id}/episode-order', { params: { path, query }, body: { tmdb_group: group } })
        : await api.DELETE('/v1/titles/{id}/episode-order', { params: { path, query } })
      if (error) throw error
    },
    onSuccess: () => {
      onPick()
      refresh()
    },
  })
  if (!orders?.length || !show.title.ids.tethr) return null
  const current = pick.isPending ? pick.variables : (show.episode_order ?? '')
  return (
    <div className={lists.filters}>
      <span className={lists.faint}>Order</span>
      <fieldset className={lists.options} disabled={pick.isPending}>
        <legend className={lists.hidden}>Episode order</legend>
        {[{ id: '', name: 'Aired' }, ...orders].map((o) => (
          <label key={o.id} className={o.id === current ? `${lists.option} ${lists.chosen}` : lists.option}>
            <input type="radio" name="order" checked={o.id === current} onChange={() => pick.mutate(o.id)} />
            {o.name}
          </label>
        ))}
      </fieldset>
      {pick.isError && <p className={lists.error}>{problemMessage(pick.error)}</p>}
    </div>
  )
}

function useOutline(profileID: number, waiting: boolean) {
  const queries = useQueryClient()
  useEffect(() => {
    if (!waiting) return
    let tries = 0
    const timer = setInterval(() => {
      if (++tries > 8) clearInterval(timer)
      queries.invalidateQueries({ queryKey: ['title', profileID] })
    }, 15000)
    return () => clearInterval(timer)
  }, [queries, profileID, waiting])
}

function Synopsis({
  text,
  hidden,
  reveal,
  className,
}: {
  text: string
  hidden: boolean
  reveal: string
  className?: string
}) {
  const [open, setOpen] = useState(false)
  const [shown, setShown] = useState(!hidden)
  if (!shown) {
    return (
      <button
        type="button"
        className={`${lists.textButton} ${styles.reveal} ${className ?? ''}`}
        onClick={() => {
          setShown(true)
          setOpen(true)
        }}
      >
        {reveal}
      </button>
    )
  }
  return (
    <button
      type="button"
      className={`${styles.synopsis} ${open ? styles.open : ''} ${className ?? ''}`}
      aria-expanded={open}
      onClick={() => setOpen(!open)}
    >
      {text}
    </button>
  )
}

function EpisodeTable({
  show,
  episodes,
  info,
  profileID,
}: {
  show: Detail
  episodes: EpisodeDetail[]
  info?: Season
  profileID: number
}) {
  return (
    <table className={styles.eps}>
      <thead>
        <tr className={lists.faint}>
          <th scope="col">
            <span className={lists.hidden}>Watched</span>
          </th>
          <th scope="col">No.</th>
          <th scope="col">Title</th>
          <th scope="col" className={styles.colAired}>
            Aired
          </th>
          <th scope="col" className={styles.colLog}>
            Logged
          </th>
        </tr>
      </thead>
      <tbody>
        {episodes.map((e) => (
          <Episode
            key={e.episode.ids.tethr ?? `n${shown(e).number}`}
            show={show}
            e={e}
            info={info?.episodes.find((n) => n.number === shown(e).number)}
            profileID={profileID}
          />
        ))}
      </tbody>
    </table>
  )
}

function Episode({
  show,
  e,
  info,
  profileID,
}: {
  show: Detail
  e: EpisodeDetail
  info?: Season['episodes'][number]
  profileID: number
}) {
  const log = useLog(profileID)
  const unlog = useUnlog(profileID)
  const toast = useToast()
  const plays = e.watches.map((w) => w.id).join()
  const [cleared, setCleared] = useState('')
  const seen = e.watches.length > 0 && cleared !== plays
  const due = !aired(e)
  const label = info?.name ?? `Episode ${shown(e).number}`
  const busy = log.isPending || unlog.isPending

  const toggle = () => {
    if (!seen) {
      log.mutate([episodeRef(show, e)])
      return
    }
    const n = e.watches.length
    setCleared(plays)
    toast({
      text: n > 1 ? `Took all ${n} plays of ${label} off your history.` : `Unticked ${label}.`,
      commit: () => unlog.mutateAsync(e.watches),
      undo: () => setCleared(''),
    })
  }

  return (
    <tr className={due ? styles.due : undefined}>
      <td className={styles.colTick}>
        <button
          type="button"
          aria-label={seen ? `Untick ${label}` : `Tick off ${label}`}
          aria-pressed={seen}
          className={seen || log.isPending ? `${lists.tickbox} ${lists.on}` : lists.tickbox}
          onClick={toggle}
          disabled={due || busy}
        >
          <svg viewBox="0 0 20 20" width="20" height="20" aria-hidden="true">
            <path
              d="M4.5 10.5l3.6 3.6L15.5 6"
              fill="none"
              stroke="currentColor"
              strokeWidth="2.4"
              strokeLinecap="round"
            />
          </svg>
        </button>
      </td>
      <td className={`${lists.code} ${styles.num}`}>{shown(e).number}</td>
      <td className={styles.colTitle}>
        <span className={styles.epName}>{label}</span>
        {info?.overview && <Synopsis text={info.overview} hidden={!seen} reveal="Read what happens" />}
        <span className={lists.small}>
          {[
            info?.runtime && duration(info.runtime),
            seen && e.watches.length > 1 && `Seen ${e.watches.length} times`,
            e.resume && !seen && `Paused at ${Math.round(e.resume.progress)}%`,
          ]
            .filter(Boolean)
            .join('. ')}
          {(log.error || unlog.error) && problemMessage(log.error ?? unlog.error)}
        </span>
      </td>
      <td className={`${lists.code} ${styles.colAired}`}>
        {e.episode.first_aired && (due ? `Due ${day(e.episode.first_aired)}` : date(e.episode.first_aired))}
      </td>
      <td className={`${lists.small} ${styles.colLog}`}>
        {seen && `${how(e.watches[0])}, ${ago(e.watches[0].watched_at)}`}
      </td>
    </tr>
  )
}

function Film({ detail: d, profileID }: { detail: Detail; profileID: number }) {
  const meta = useDetails('movie', d.title.ids.tmdb)
  const name = meta?.name ?? d.title.name ?? 'Untitled film'
  const log = useLog(profileID)
  const unlog = useUnlog(profileID)
  const plays = d.watches
  const self = titleRef(d)
  const runtime = d.title.runtime_minutes ?? meta?.runtime

  return (
    <div className={`${lists.page} ${styles.page}`}>
      <Back />
      <header className={styles.masthead}>
        <Cover id={coverID(d)} name={name} poster={meta?.poster} size="l" />
        <div className={styles.body}>
          <h1 className={lists.title}>{name}</h1>
          <About meta={meta} made="Directed by" />
          <Facts
            facts={[
              ['Original title', d.title.original_name !== name ? d.title.original_name : undefined],
              ['Released', (d.title.year ?? meta?.year)?.toString()],
              ['Runtime', runtime ? duration(runtime) : undefined],
              ['Genre', meta?.genres?.join(', ')],
              ['Rated', rated(meta)],
            ]}
          />
          <div className={styles.progress}>
            <p className={lists.lead}>
              {d.resume
                ? `Paused ${Math.round(d.resume.progress)}% in${runtime ? `, with about ${duration(Math.round(runtime * (1 - d.resume.progress / 100)))} left` : ''}. Any connected app can pick it up.`
                : plays.length
                  ? `Watched ${plays.length === 1 ? 'once' : plural(plays.length, 'time')}, last ${ago(plays[0].watched_at)}.`
                  : 'Not watched yet.'}
            </p>
            {d.resume && (
              <span className={lists.meter}>
                <span style={{ transform: `scaleX(${d.resume.progress / 100})` }} />
              </span>
            )}
          </div>
          <p className={styles.actions}>
            <Button
              variant={plays.length ? 'plain' : 'primary'}
              onClick={() => log.mutate([self])}
              disabled={log.isPending}
            >
              {plays.length ? 'Log another watch' : 'Tick off as watched'}
            </Button>
          </p>
          {log.isError && <p className={lists.error}>{problemMessage(log.error)}</p>}
          <Yours detail={d} profileID={profileID} />
        </div>
      </header>

      {!!plays.length && (
        <section>
          <h2 className={lists.sectionTitle}>Plays</h2>
          <ol className={lists.plainList}>
            {plays.map((p) => (
              <li key={p.id} className={styles.play}>
                <span className={lists.code}>{date(p.watched_at)}</span>
                <span className={lists.quiet}>{how(p)}</span>
                <button
                  type="button"
                  className={`${lists.textButton} ${lists.remove}`}
                  onClick={() => unlog.mutate([p])}
                  disabled={unlog.isPending}
                >
                  Remove
                </button>
              </li>
            ))}
          </ol>
        </section>
      )}
    </div>
  )
}

function Yours({ detail: d, profileID }: { detail: Detail; profileID: number }) {
  const refresh = useRefresh(profileID)
  const self = titleRef(d)
  const params = { query: { profile_id: profileID } }

  const rate = useMutation({
    mutationFn: async (rating: number | null) => {
      const { error } =
        rating === null
          ? await api.DELETE('/v1/ratings/{id}', { params: { ...params, path: { id: d.rating?.id ?? 0 } } })
          : await api.POST('/v1/ratings', { params, body: { items: [{ item: self, rating }] } })
      if (error) throw error
    },
    onSuccess: refresh,
  })
  const list = useMutation({
    mutationFn: async () => {
      const { error } = d.listing
        ? await api.DELETE('/v1/watchlist/{id}', { params: { ...params, path: { id: d.listing.id } } })
        : await api.POST('/v1/watchlist', { params, body: { items: [{ item: self }] } })
      if (error) throw error
    },
    onSuccess: refresh,
  })

  return (
    <div className={styles.yours}>
      <div className={styles.rating}>
        <span className={lists.faint}>Your rating</span>
        <fieldset className={lists.options} disabled={rate.isPending}>
          <legend className={lists.hidden}>Your rating out of 10</legend>
          {Array.from({ length: 10 }, (_, i) => i + 1).map((n) => (
            <label
              key={n}
              className={
                n === d.rating?.rating
                  ? `${lists.option} ${lists.chosen} ${styles.score}`
                  : `${lists.option} ${styles.score}`
              }
            >
              <input type="radio" name="rating" checked={n === d.rating?.rating} onChange={() => rate.mutate(n)} />
              {n}
            </label>
          ))}
        </fieldset>
        {d.rating && (
          <button type="button" className={lists.textButton} onClick={() => rate.mutate(null)}>
            Clear
          </button>
        )}
      </div>
      <p className={lists.small}>
        {d.listing ? (
          <>
            On your watchlist since {date(d.listing.listed_at)}.{' '}
            <button type="button" className={lists.textButton} onClick={() => list.mutate()} disabled={list.isPending}>
              Take it off
            </button>
          </>
        ) : (
          <button type="button" className={lists.textButton} onClick={() => list.mutate()} disabled={list.isPending}>
            Add to watchlist
          </button>
        )}
      </p>
      <OnLists detail={d} profileID={profileID} />
      {(rate.isError || list.isError) && <p className={lists.error}>{problemMessage(rate.error ?? list.error)}</p>}
    </div>
  )
}

function OnLists({ detail: d, profileID }: { detail: Detail; profileID: number }) {
  const refresh = useRefresh(profileID)
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [said, setSaid] = useState('')
  const self = titleRef(d)
  const params = { query: { profile_id: profileID } }
  const on = new Map((d.lists ?? []).map((l) => [l.list_id, l.item_id]))
  const all = useQuery({
    queryKey: ['lists', profileID],
    enabled: open || on.size > 0,
    queryFn: async () => (await api.GET('/v1/lists', { params })).data ?? [],
  })
  const editable = all.data?.filter((l) => l.can_edit_items) ?? []
  const listed = all.data?.filter((l) => on.has(l.id)).map((l) => l.name) ?? []

  const toggle = useMutation({
    mutationFn: async (l: { id: number; name: string }) => {
      const item = on.get(l.id)
      const { error } = item
        ? await api.DELETE('/v1/lists/{id}/items/{item}', { params: { ...params, path: { id: l.id, item } } })
        : await api.POST('/v1/lists/{id}/items', {
            params: { ...params, path: { id: l.id } },
            body: { items: [{ item: self }] },
          })
      if (error) throw error
      return item ? `Taken off ${l.name}.` : `Added to ${l.name}.`
    },
    onSuccess: (words) => {
      setSaid(words)
      refresh()
    },
  })
  const create = useMutation({
    mutationFn: async () => {
      const made = await api.POST('/v1/lists', { params, body: { name } })
      if (made.error) throw made.error
      const { error } = await api.POST('/v1/lists/{id}/items', {
        params: { ...params, path: { id: made.data.id } },
        body: { items: [{ item: self }] },
      })
      if (error) throw error
      return made.data.name
    },
    onSuccess: (made) => {
      setName('')
      setSaid(`Made ${made} and added this to it.`)
      refresh()
    },
  })
  const start = () => {
    setSaid('')
    toggle.reset()
    create.reset()
    setOpen(true)
  }

  return (
    <div className={styles.onLists}>
      <p className={lists.small}>
        {listed.length > 0 && <>On {names(listed)}. </>}
        <button type="button" className={lists.textButton} onClick={start}>
          {listed.length ? 'Change lists' : 'Add to a list'}
        </button>
      </p>
      <Slip open={open} onClose={() => setOpen(false)} title={`Add ${d.title.name ?? 'this'} to a list`}>
        <SlipBody>
          {editable.length > 0 ? (
            <ul className={styles.pick}>
              {editable.map((l) => (
                <li key={l.id}>
                  <label className={styles.pickRow}>
                    <input
                      type="checkbox"
                      checked={on.has(l.id)}
                      disabled={toggle.isPending}
                      onChange={() => toggle.mutate(l)}
                    />
                    <span className={on.has(l.id) ? `${lists.tickbox} ${lists.on}` : lists.tickbox}>
                      <svg viewBox="0 0 20 20" width="20" height="20" aria-hidden="true">
                        <path
                          d="M4.5 10.5l3.6 3.6L15.5 6"
                          fill="none"
                          stroke="currentColor"
                          strokeWidth="2.4"
                          strokeLinecap="round"
                        />
                      </svg>
                    </span>
                    <span>
                      <span className={styles.pickName}>{l.name}</span>
                      <span className={styles.pickWho}>
                        {l.access === 'mine'
                          ? privacyWords[l.privacy]
                          : `${ownerName(l)}’s, ${privacyWords[l.privacy].toLowerCase()}`}
                      </span>
                    </span>
                    <span className={`${lists.code} ${styles.pickWho}`}>{l.item_count}</span>
                  </label>
                </li>
              ))}
            </ul>
          ) : (
            all.data && <p className={lists.quiet}>You haven’t made any lists yet.</p>
          )}
          <p className={styles.said} aria-live="polite">
            {toggle.isError || create.isError ? (
              <span className={lists.error}>{problemMessage(toggle.error ?? create.error)}</span>
            ) : (
              said
            )}
          </p>
          <form
            className={styles.newList}
            onSubmit={(e) => {
              e.preventDefault()
              if (name.trim()) create.mutate()
            }}
          >
            <Field label="Or start a new list" value={name} onChange={setName} maxLength={100} />
            <Button type="submit" disabled={!name.trim() || create.isPending}>
              Make it
            </Button>
          </form>
        </SlipBody>
        <SlipFoot>
          <Button variant="primary" onClick={() => setOpen(false)}>
            Done
          </Button>
          <span className={lists.quiet}>Ticking adds it straight away.</span>
        </SlipFoot>
      </Slip>
    </div>
  )
}
