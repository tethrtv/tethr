import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, problemMessage } from '../api/client'
import type { components } from '../api/schema'
import { Cover } from '../components/Cover'
import { useWatching } from '../components/Shell'
import { ago, date, ep, plural } from '../format'
import { useDetails, useEpisode } from '../tmdb'
import styles from './Lists.module.css'

type Entry = components['schemas']['UpNextEntry']
type ResumePoint = components['schemas']['ResumePoint']

export function UpNext() {
  const { profile } = useWatching()
  const params = { query: { profile_id: profile.id } }
  const next = useQuery({
    queryKey: ['up-next', profile.id],
    queryFn: async () => (await api.GET('/v1/up-next', { params })).data ?? [],
  })
  const paused = useQuery({
    queryKey: ['playback', profile.id],
    queryFn: async () => (await api.GET('/v1/playback', { params })).data ?? [],
  })
  const today = new Date().toLocaleDateString('en-GB', { weekday: 'long', day: 'numeric', month: 'long' })

  return (
    <div className={styles.page}>
      <header className={styles.head}>
        <h1 className={styles.title}>Up next</h1>
        <p className={styles.lead}>
          {today}. {next.data && <strong>{plural(next.data.length, 'show')}</strong>}
          {next.data && ' on the go.'}
        </p>
      </header>

      {!!paused.data?.length && (
        <section>
          <h2 className={styles.sectionTitle}>Paused</h2>
          <ol className={styles.rows}>
            {paused.data.map((p) => (
              <Paused key={p.id} point={p} profileID={profile.id} />
            ))}
          </ol>
        </section>
      )}

      <section>
        <h2 className={styles.sectionTitle}>Next episodes</h2>
        {next.data?.length ? (
          <ol className={styles.rows}>
            {next.data.map((e) => (
              <Next key={e.show.ids.tethr} entry={e} profileID={profile.id} />
            ))}
          </ol>
        ) : (
          next.data && (
            <p className={styles.quiet}>
              Nothing on the go. Start something in any connected app and it’ll be listed here.
            </p>
          )
        )}
      </section>
    </div>
  )
}

function Next({ entry: e, profileID }: { entry: Entry; profileID: number }) {
  const queries = useQueryClient()
  const tmdb = e.show.ids.tmdb
  const show = useDetails('show', tmdb)
  const episode = useEpisode(tmdb, e.next.season, e.next.number)
  const name = show?.name ?? e.show.name ?? 'Untitled show'
  const left = e.aired - e.watched

  const watched = useMutation({
    mutationFn: async () => {
      const { error } = await api.POST('/v1/history', {
        params: { query: { profile_id: profileID } },
        body: { items: [{ item: { type: 'episode', ids: e.next.ids } }] },
      })
      if (error) throw error
    },
    onSuccess: () => {
      queries.invalidateQueries({ queryKey: ['up-next', profileID] })
      queries.invalidateQueries({ queryKey: ['history', profileID] })
    },
  })

  return (
    <li className={styles.next}>
      <button
        type="button"
        aria-label={`Mark ${name} ${ep(e.next.season, e.next.number)} as watched`}
        className={watched.isPending ? `${styles.tickbox} ${styles.on}` : styles.tickbox}
        onClick={() => watched.mutate()}
        disabled={watched.isPending}
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
      <Cover id={e.show.ids.tethr ?? 0} name={name} poster={show?.poster} />
      <div className={styles.main}>
        <span className={styles.rowTitle}>{name}</span>
        <p className={styles.line}>
          <span className={styles.code}>{ep(e.next.season, e.next.number)}</span> {episode?.name}
        </p>
        {e.resume && <p className={styles.small}>Started, {Math.round(e.resume.progress)}% in</p>}
        {watched.isError && <p className={styles.error}>{problemMessage(watched.error)}</p>}
      </div>
      <div className={styles.progress}>
        <span className={styles.meter}>
          <span style={{ transform: `scaleX(${e.aired ? e.watched / e.aired : 0})` }} />
        </span>
        <span className={styles.faint}>
          {e.watched} of {e.aired} watched, {left === 1 ? 'the last one' : `${left} to go`}
        </span>
      </div>
      <div className={styles.meta}>
        {e.next.first_aired && <span className={styles.code}>Aired {date(e.next.first_aired)}</span>}
        <span className={styles.faint}>Last watched {ago(e.last_watched_at)}</span>
      </div>
    </li>
  )
}

function Paused({ point: p, profileID }: { point: ResumePoint; profileID: number }) {
  const queries = useQueryClient()
  const tmdb = p.item.title.ids.tmdb
  const kind = p.item.type === 'movie' ? 'movie' : 'show'
  const title = useDetails(kind, tmdb)
  const episode = useEpisode(kind === 'show' ? tmdb : undefined, p.item.episode?.season, p.item.episode?.number)
  const name = title?.name ?? p.item.title.name ?? 'Untitled'

  const clear = useMutation({
    mutationFn: async () => {
      await api.DELETE('/v1/playback/{id}', { params: { path: { id: p.id }, query: { profile_id: profileID } } })
    },
    onSuccess: () => {
      queries.invalidateQueries({ queryKey: ['playback', profileID] })
      queries.invalidateQueries({ queryKey: ['up-next', profileID] })
    },
  })

  return (
    <li className={styles.paused}>
      <Cover id={p.item.title.ids.tethr ?? 0} name={name} poster={title?.poster} />
      <div className={styles.main}>
        <span className={styles.rowTitle}>{name}</span>
        <p className={styles.line}>
          {p.item.episode ? (
            <>
              <span className={styles.code}>{ep(p.item.episode.season, p.item.episode.number)}</span> {episode?.name}
            </>
          ) : (
            <span className={styles.quiet}>Film{title?.year ? `, ${title.year}` : ''}</span>
          )}
        </p>
      </div>
      <div className={styles.progress}>
        <span className={styles.meter}>
          <span style={{ transform: `scaleX(${p.progress / 100})` }} />
        </span>
        <span className={styles.faint}>
          {Math.round(p.progress)}% in, paused {ago(p.paused_at)}
        </span>
      </div>
      <button type="button" className={styles.textButton} onClick={() => clear.mutate()}>
        Clear
      </button>
    </li>
  )
}
