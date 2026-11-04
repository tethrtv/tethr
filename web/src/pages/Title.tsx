import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useCanGoBack, useNavigate, useRouter } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { api, problemMessage } from '../api/client'
import type { components } from '../api/schema'
import { Button } from '../components/Button'
import { Cover } from '../components/Cover'
import { useWatching } from '../components/Shell'
import { ago, date, duration, ep, plural } from '../format'
import { useDetails, useSeason } from '../tmdb'
import lists from './Lists.module.css'
import styles from './Title.module.css'

type Detail = components['schemas']['TitleDetail']
type EpisodeDetail = components['schemas']['EpisodeDetail']
type Play = components['schemas']['Play']
type ItemRef = components['schemas']['ItemRef']

const today = () => new Date().toISOString().slice(0, 10)
const aired = (e: EpisodeDetail) =>
  e.watches.length > 0 || (!!e.episode.first_aired && e.episode.first_aired <= today())
const how = (p: Play) =>
  p.action === 'scrobble' ? 'Played in an app' : p.action === 'checkin' ? 'Checked in' : 'Added by hand'

export function TitlePage({ slug }: { slug: string }) {
  const { profile } = useWatching()
  const navigate = useNavigate()
  const title = useQuery({
    queryKey: ['title', profile.id, slug],
    queryFn: async () => {
      const { data, response } = await api.GET('/v1/titles/{id}', {
        params: { path: { id: slug }, query: { profile_id: profile.id } },
      })
      if (response.status === 404) return null
      if (!data) throw new Error('could not load the title')
      return data
    },
  })
  const d = title.data
  const wanted = d?.title.slug

  useEffect(() => {
    if (!d || !wanted || wanted === slug) return
    navigate({ to: d.type === 'movie' ? '/films/$slug' : '/shows/$slug', params: { slug: wanted }, replace: true })
  }, [d, wanted, slug, navigate])

  if (title.data === null) {
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
    for (const key of ['title', 'history', 'up-next', 'playback', 'watchlist']) {
      queries.invalidateQueries({ queryKey: [key, profileID] })
    }
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
        await api.DELETE('/v1/history/{id}', { params: { path: { id: p.id }, query: { profile_id: profileID } } })
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

function Show({ detail: d, profileID }: { detail: Detail; profileID: number }) {
  const meta = useDetails('show', d.title.ids.tmdb)
  const name = meta?.name ?? d.title.name ?? 'Untitled show'
  const seasons = d.seasons ?? []
  const numbered = seasons.filter((s) => s.number > 0)
  const episodes = numbered.flatMap((s) => s.episodes)
  const out = episodes.filter(aired)
  const seen = out.filter((e) => e.watches.length).length
  const left = out.length - seen
  const paused = episodes.find((e) => e.resume && !e.watches.length)
  const firstGap = numbered.find((s) => s.episodes.some((e) => aired(e) && !e.watches.length))
  const [chosen, setChosen] = useState<number>()
  const season = seasons.find((s) => s.number === chosen) ?? firstGap ?? numbered[numbered.length - 1] ?? seasons[0]
  const log = useLog(profileID)
  const rest = season?.episodes.filter((e) => aired(e) && !e.watches.length) ?? []

  return (
    <div className={`${lists.page} ${styles.page}`}>
      <Back />
      <header className={styles.masthead}>
        <Cover id={d.title.ids.tethr ?? 0} name={name} poster={meta?.poster} size="l" />
        <div className={styles.body}>
          <h1 className={lists.title}>{name}</h1>
          <Facts
            facts={[
              ['Original title', d.title.original_name !== name ? d.title.original_name : undefined],
              ['First aired', d.title.year?.toString()],
              ['Episodes', episodes.length ? `${episodes.length} in ${plural(numbered.length, 'season')}` : undefined],
              ['Runtime', d.title.runtime_minutes ? `About ${duration(d.title.runtime_minutes)} each` : undefined],
            ]}
          />
          <div className={styles.progress}>
            <p className={lists.lead}>
              {!out.length ? (
                'Tethr doesn’t have the episode list yet. It usually turns up within a minute of the first watch.'
              ) : !left ? (
                'You’ve seen every episode that’s aired.'
              ) : (
                <>
                  You’ve seen <strong>{seen}</strong> of {out.length}. {left} to go
                  {d.title.runtime_minutes ? `, about ${duration(left * d.title.runtime_minutes)}` : ''}.
                </>
              )}
              {paused &&
                ` Paused in ${ep(paused.episode.season, paused.episode.number)}, ${Math.round(paused.resume?.progress ?? 0)}% in.`}
            </p>
            {!!numbered.length && (
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

      {season && (
        <section className={styles.episodes}>
          <div className={lists.filters}>
            <span className={lists.faint}>Season</span>
            <fieldset className={lists.options}>
              <legend className={lists.hidden}>Season</legend>
              {seasons.map((s) => (
                <label
                  key={s.number}
                  className={s.number === season.number ? `${lists.option} ${lists.chosen}` : lists.option}
                >
                  <input
                    type="radio"
                    name="season"
                    checked={s.number === season.number}
                    onChange={() => setChosen(s.number)}
                  />
                  {s.number ? s.number : 'Specials'}
                </label>
              ))}
            </fieldset>
          </div>
          <Episodes show={d} episodes={season.episodes} season={season.number} profileID={profileID} />
          {rest.length > 1 && (
            <p>
              <Button
                onClick={() => log.mutate(rest.map((e) => ({ type: 'episode', ids: { tethr: e.episode.ids.tethr } })))}
                disabled={log.isPending}
              >
                Tick off the rest of {season.number ? `season ${season.number}` : 'the specials'}
              </Button>
            </p>
          )}
          {log.isError && <p className={lists.error}>{problemMessage(log.error)}</p>}
        </section>
      )}
    </div>
  )
}

function Episodes({
  show,
  episodes,
  season,
  profileID,
}: {
  show: Detail
  episodes: EpisodeDetail[]
  season: number
  profileID: number
}) {
  const names = useSeason(show.title.ids.tmdb, season)?.episodes
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
            key={e.episode.ids.tethr}
            e={e}
            name={names?.find((n) => n.number === e.episode.number)?.name}
            profileID={profileID}
          />
        ))}
      </tbody>
    </table>
  )
}

function Episode({ e, name, profileID }: { e: EpisodeDetail; name?: string; profileID: number }) {
  const log = useLog(profileID)
  const unlog = useUnlog(profileID)
  const [confirming, setConfirming] = useState(false)
  const seen = e.watches.length > 0
  const due = !aired(e)
  const label = name ?? `Episode ${e.episode.number}`
  const busy = log.isPending || unlog.isPending

  const toggle = () => {
    if (!seen) log.mutate([{ type: 'episode', ids: { tethr: e.episode.ids.tethr } }])
    else if (e.watches.length > 1) setConfirming(true)
    else unlog.mutate(e.watches)
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
      <td className={`${lists.code} ${styles.num}`}>{e.episode.number}</td>
      <td className={styles.colTitle}>
        <span className={styles.epName}>{label}</span>
        <span className={lists.small}>
          {e.watches.length > 1 && `Seen ${e.watches.length} times. `}
          {e.resume && !seen && `Paused at ${Math.round(e.resume.progress)}%.`}
          {(log.error || unlog.error) && problemMessage(log.error ?? unlog.error)}
        </span>
      </td>
      <td className={`${lists.code} ${styles.colAired}`}>
        {e.episode.first_aired &&
          (due
            ? `Due ${new Date(e.episode.first_aired).toLocaleDateString('en-GB', { weekday: 'short', day: 'numeric', month: 'short' })}`
            : date(e.episode.first_aired))}
      </td>
      <td className={`${lists.small} ${styles.colLog}`}>
        {confirming ? (
          <span className={styles.confirm}>
            Remove all {e.watches.length} plays?{' '}
            <button type="button" className={lists.textButton} onClick={() => setConfirming(false)}>
              Keep them
            </button>{' '}
            <button
              type="button"
              className={`${lists.textButton} ${styles.danger}`}
              onClick={() => {
                setConfirming(false)
                unlog.mutate(e.watches)
              }}
            >
              Remove
            </button>
          </span>
        ) : (
          seen && `${how(e.watches[0])}, ${ago(e.watches[0].watched_at)}`
        )}
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
  const self: ItemRef = { type: 'movie', ids: { tethr: d.title.ids.tethr } }

  return (
    <div className={`${lists.page} ${styles.page}`}>
      <Back />
      <header className={styles.masthead}>
        <Cover id={d.title.ids.tethr ?? 0} name={name} poster={meta?.poster} size="l" />
        <div className={styles.body}>
          <h1 className={lists.title}>{name}</h1>
          <Facts
            facts={[
              ['Original title', d.title.original_name !== name ? d.title.original_name : undefined],
              ['Released', d.title.year?.toString()],
              ['Runtime', d.title.runtime_minutes ? duration(d.title.runtime_minutes) : undefined],
            ]}
          />
          <div className={styles.progress}>
            <p className={lists.lead}>
              {d.resume
                ? `Paused ${Math.round(d.resume.progress)}% in${d.title.runtime_minutes ? `, with about ${duration(Math.round(d.title.runtime_minutes * (1 - d.resume.progress / 100)))} left` : ''}. Any connected app can pick it up.`
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
  const self: ItemRef = { type: d.type, ids: { tethr: d.title.ids.tethr } }
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
      {(rate.isError || list.isError) && <p className={lists.error}>{problemMessage(rate.error ?? list.error)}</p>}
    </div>
  )
}
