import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import type { components } from '../api/schema'
import { Cover } from '../components/Cover'
import { useWatching } from '../components/Shell'
import { TitleLink } from '../components/TitleLink'
import { date, ep, plural } from '../format'
import { useDetails } from '../tmdb'
import styles from './Lists.module.css'

type Listing = components['schemas']['Listing']

export function Watchlist() {
  const { profile } = useWatching()
  const list = useQuery({
    queryKey: ['watchlist', profile.id],
    queryFn: async () => (await api.GET('/v1/watchlist', { params: { query: { profile_id: profile.id } } })).data ?? [],
  })

  return (
    <div className={styles.page}>
      <header className={styles.head}>
        <h1 className={styles.title}>Watchlist</h1>
        <p className={styles.lead}>
          {list.data && <strong>{plural(list.data.length, 'thing')}</strong>} to get round to. Watching one takes it off
          by itself.
        </p>
      </header>
      <section>
        <h2 className={styles.sectionTitle}>In order</h2>
        {list.data?.length ? (
          <ol className={styles.rows}>
            {list.data.map((l) => (
              <Row key={l.id} listing={l} profileID={profile.id} />
            ))}
          </ol>
        ) : (
          list.data && <p className={styles.quiet}>Nothing on the watchlist.</p>
        )}
      </section>
    </div>
  )
}

function Row({ listing: l, profileID }: { listing: Listing; profileID: number }) {
  const queries = useQueryClient()
  const kind = l.item.type === 'movie' ? 'movie' : 'show'
  const title = useDetails(kind, l.item.title.ids.tmdb)
  const name = title?.name ?? l.item.title.name ?? 'Untitled'
  const what =
    l.item.type === 'movie'
      ? 'Film'
      : l.item.type === 'season'
        ? `Season ${l.item.season}`
        : l.item.episode
          ? ep(l.item.episode.season, l.item.episode.number)
          : 'Show'

  const remove = useMutation({
    mutationFn: async () => {
      await api.DELETE('/v1/watchlist/{id}', { params: { path: { id: l.id }, query: { profile_id: profileID } } })
    },
    onSuccess: () => queries.invalidateQueries({ queryKey: ['watchlist', profileID] }),
  })

  return (
    <li className={styles.listing}>
      <span className={`${styles.code} ${styles.rank}`}>{l.rank}</span>
      <Cover id={l.item.title.ids.tethr ?? 0} name={name} poster={title?.poster} />
      <div className={styles.main}>
        <TitleLink type={l.item.type} slug={l.item.title.slug} className={`${styles.rowTitle} ${styles.titleLink}`}>
          {name}
        </TitleLink>
        <p className={styles.quiet}>
          {what}
          {title?.year ? `, ${title.year}` : ''}
        </p>
        {l.notes && <p className={styles.small}>{l.notes}</p>}
      </div>
      <span className={styles.faint}>Added {date(l.listed_at)}</span>
      <button type="button" className={`${styles.textButton} ${styles.remove}`} onClick={() => remove.mutate()}>
        Remove
      </button>
    </li>
  )
}
