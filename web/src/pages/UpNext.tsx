import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { api, problemMessage } from '../api/client'
import type { components } from '../api/schema'
import { Cover } from '../components/Cover'
import { ListCover } from '../components/ListCover'
import { useWatching } from '../components/Shell'
import { TitleLink } from '../components/TitleLink'
import { ago, date, ep, plural } from '../format'
import { useDetails, useEpisode } from '../tmdb'
import styles from './Lists.module.css'
import { listPath } from './MyLists'

type Entry = components['schemas']['UpNextEntry']
type ListEntry = components['schemas']['UpNextListEntry']
type ResumePoint = components['schemas']['ResumePoint']

export function UpNext() {
  const { profile } = useWatching()
  const params = { query: { profile_id: profile.id } }
  const next = useQuery({
    queryKey: ['up-next', profile.id],
    queryFn: async () => (await api.GET('/v1/up-next', { params })).data ?? [],
  })
  const lists = useQuery({
    queryKey: ['up-next-lists', profile.id],
    queryFn: async () => (await api.GET('/v1/up-next/lists', { params })).data ?? [],
  })
  const paused = useQuery({
    queryKey: ['playback', profile.id],
    queryFn: async () => (await api.GET('/v1/playback', { params })).data ?? [],
  })
  const today = new Date().toLocaleDateString('en-GB', { weekday: 'long', day: 'numeric', month: 'long' })
  const fromLists = new Set(lists.data?.map((l) => l.next.episode?.ids.tethr).filter(Boolean))
  const shows = next.data?.filter((e) => !fromLists.has(e.next.ids.tethr))
  const rows =
    shows &&
    lists.data &&
    [
      ...shows.map((e) => ({ at: e.last_watched_at, show: e })),
      ...lists.data.map((l) => ({ at: l.last_watched_at ?? '', list: l })),
    ].sort((a, b) => b.at.localeCompare(a.at))

  return (
    <div className={styles.page}>
      <header className={styles.head}>
        <h1 className={styles.title}>Up next</h1>
        <p className={styles.lead}>
          {today}.{' '}
          {shows && lists.data && (
            <>
              <strong>{plural(shows.length, 'show')}</strong>
              {lists.data.length > 0 && (
                <>
                  {' and '}
                  <strong>{plural(lists.data.length, 'list')}</strong>
                </>
              )}{' '}
              on the go.
            </>
          )}
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
        <h2 className={styles.sectionTitle}>Next to watch</h2>
        {rows?.length ? (
          <ol className={styles.rows}>
            {rows.map((r) =>
              'show' in r ? (
                <Next key={`show-${r.show.show.ids.tethr}`} entry={r.show} profileID={profile.id} />
              ) : (
                <ListNext key={`list-${r.list.list.id}`} entry={r.list} profileID={profile.id} />
              ),
            )}
          </ol>
        ) : (
          rows && (
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
  const numbered = e.next.order ?? e.next
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
        aria-label={`Mark ${name} ${ep(numbered.season, numbered.number)} as watched`}
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
        <TitleLink type="show" slug={e.show.slug} className={`${styles.rowTitle} ${styles.titleLink}`}>
          {name}
        </TitleLink>
        <p className={styles.line}>
          <span className={styles.code}>{ep(numbered.season, numbered.number)}</span> {episode?.name}
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

function ListNext({ entry: e, profileID }: { entry: ListEntry; profileID: number }) {
  const queries = useQueryClient()
  const tmdb = e.next.title.ids.tmdb
  const film = e.next.type === 'movie'
  const title = useDetails(film ? 'movie' : 'show', tmdb)
  const episode = useEpisode(film ? undefined : tmdb, e.next.episode?.season, e.next.episode?.number)
  const numbered = e.next.episode && (e.next.episode.order ?? e.next.episode)
  const name = title?.name ?? e.next.title.name ?? 'Untitled'
  const watchedCount = e.list.watched_count ?? 0

  const watched = useMutation({
    mutationFn: async () => {
      const item = e.next.episode
        ? { type: 'episode' as const, ids: e.next.episode.ids }
        : { type: 'movie' as const, ids: e.next.title.ids }
      const { error } = await api.POST('/v1/history', {
        params: { query: { profile_id: profileID } },
        body: { items: [{ item }] },
      })
      if (error) throw error
    },
    onSuccess: () => {
      queries.invalidateQueries({ queryKey: ['up-next-lists', profileID] })
      queries.invalidateQueries({ queryKey: ['up-next', profileID] })
      queries.invalidateQueries({ queryKey: ['history', profileID] })
    },
  })

  return (
    <li className={styles.next}>
      <button
        type="button"
        aria-label={`Mark ${name}${numbered ? ` ${ep(numbered.season, numbered.number)}` : ''} as watched`}
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
      <ListCover items={e.list.cover} size="s" />
      <div className={styles.main}>
        <Link
          to="/lists/$list"
          params={{ list: listPath(e.list) }}
          className={`${styles.rowTitle} ${styles.titleLink}`}
        >
          {e.list.name}
        </Link>
        <p className={styles.line}>
          <TitleLink type={film ? 'movie' : 'show'} slug={e.next.title.slug} className={styles.titleLink}>
            {name}
          </TitleLink>{' '}
          {numbered ? (
            <>
              <span className={styles.code}>{ep(numbered.season, numbered.number)}</span> {episode?.name}
            </>
          ) : (
            <span className={styles.quiet}>Film{title?.year ? `, ${title.year}` : ''}</span>
          )}
        </p>
        {e.resume && <p className={styles.small}>Started, {Math.round(e.resume.progress)}% in</p>}
        {watched.isError && <p className={styles.error}>{problemMessage(watched.error)}</p>}
      </div>
      <div className={styles.progress}>
        <span className={styles.meter}>
          <span style={{ transform: `scaleX(${e.list.item_count ? watchedCount / e.list.item_count : 0})` }} />
        </span>
        <span className={styles.faint}>
          {watchedCount} of {e.list.item_count} watched, number {e.position} next
        </span>
      </div>
      <div className={styles.meta}>
        <span className={styles.faint}>
          {e.last_watched_at ? `Last watched ${ago(e.last_watched_at)}` : 'Not started'}
        </span>
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
        <TitleLink type={p.item.type} slug={p.item.title.slug} className={`${styles.rowTitle} ${styles.titleLink}`}>
          {name}
        </TitleLink>
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
