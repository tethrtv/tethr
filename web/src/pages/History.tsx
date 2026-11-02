import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { api } from '../api/client'
import type { components } from '../api/schema'
import { Cover } from '../components/Cover'
import { useWatching } from '../components/Shell'
import { clock, dayLabel, ep, plural } from '../format'
import { useDetails, useEpisode } from '../tmdb'
import styles from './Lists.module.css'

type Watch = components['schemas']['Watch']
type Kind = 'all' | 'episode' | 'movie'

const kinds: { value: Kind; label: string }[] = [
  { value: 'all', label: 'Everything' },
  { value: 'episode', label: 'Episodes' },
  { value: 'movie', label: 'Films' },
]

export function History() {
  const { profile } = useWatching()
  const [kind, setKind] = useState<Kind>('all')
  const [limit, setLimit] = useState(100)
  const history = useQuery({
    queryKey: ['history', profile.id, kind, limit],
    placeholderData: (previous) => previous,
    queryFn: async () =>
      (
        await api.GET('/v1/history', {
          params: { query: { profile_id: profile.id, limit, type: kind === 'all' ? undefined : kind } },
        })
      ).data,
  })

  const groups: { label: string; items: Watch[] }[] = []
  for (const w of history.data?.items ?? []) {
    const label = dayLabel(w.watched_at)
    const last = groups[groups.length - 1]
    if (last?.label === label) last.items.push(w)
    else groups.push({ label, items: [w] })
  }

  return (
    <div className={styles.page}>
      <header className={styles.head}>
        <h1 className={styles.title}>History</h1>
        <p className={styles.lead}>{history.data && <strong>{plural(history.data.total, 'play')}</strong>} so far.</p>
      </header>

      <div className={styles.filters}>
        <span className={styles.faint}>Show</span>
        <fieldset className={styles.options}>
          <legend className={styles.hidden}>Type</legend>
          {kinds.map((k) => (
            <label key={k.value} className={k.value === kind ? `${styles.option} ${styles.chosen}` : styles.option}>
              <input type="radio" name="kind" checked={k.value === kind} onChange={() => setKind(k.value)} />
              {k.label}
            </label>
          ))}
        </fieldset>
      </div>

      <div className={styles.diary}>
        {groups.map((g) => (
          <section key={g.label}>
            <h2 className={styles.dayHead}>
              <span>{g.label}</span>
              <span className={styles.faint}>{plural(g.items.length, 'play')}</span>
            </h2>
            <ol className={styles.plainList}>
              {g.items.map((w) => (
                <Entry key={w.id} watch={w} profileID={profile.id} />
              ))}
            </ol>
          </section>
        ))}
        {history.data && !groups.length && <p className={styles.quiet}>Nothing here yet.</p>}
        {history.data && history.data.total > history.data.items.length && (
          <p>
            <button type="button" className={styles.textButton} onClick={() => setLimit(limit + 100)}>
              Show older plays
            </button>
          </p>
        )}
      </div>
    </div>
  )
}

function Entry({ watch: w, profileID }: { watch: Watch; profileID: number }) {
  const queries = useQueryClient()
  const tmdb = w.item.title.ids.tmdb
  const kind = w.item.type === 'movie' ? 'movie' : 'show'
  const title = useDetails(kind, tmdb)
  const episode = useEpisode(kind === 'show' ? tmdb : undefined, w.item.episode?.season, w.item.episode?.number)
  const name = title?.name ?? w.item.title.name ?? 'Untitled'

  const remove = useMutation({
    mutationFn: async () => {
      await api.DELETE('/v1/history/{id}', { params: { path: { id: w.id }, query: { profile_id: profileID } } })
    },
    onSuccess: () => {
      queries.invalidateQueries({ queryKey: ['history', profileID] })
      queries.invalidateQueries({ queryKey: ['up-next', profileID] })
    },
  })

  return (
    <li className={styles.entry}>
      <span className={`${styles.code} ${styles.time}`}>{clock(w.watched_at)}</span>
      <Cover id={w.item.title.ids.tethr ?? 0} name={name} poster={title?.poster} size="xs" />
      <span className={styles.main}>
        <span className={styles.entryTitle}>{name}</span>
        <span className={styles.quiet}>
          {w.item.episode ? (
            <>
              <span className={styles.code}>{ep(w.item.episode.season, w.item.episode.number)}</span> {episode?.name}
            </>
          ) : (
            `Film${title?.year ? `, ${title.year}` : ''}`
          )}
        </span>
      </span>
      <span className={styles.faint}>
        {w.action === 'scrobble' ? 'Played in an app' : w.action === 'checkin' ? 'Checked in' : 'Added by hand'}
      </span>
      <button type="button" className={`${styles.textButton} ${styles.remove}`} onClick={() => remove.mutate()}>
        Remove
      </button>
    </li>
  )
}
