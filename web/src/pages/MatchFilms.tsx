import { useMutation } from '@tanstack/react-query'
import { type FormEvent, useEffect, useState } from 'react'
import { api, problemMessage } from '../api/client'
import type { components } from '../api/schema'
import { Button } from '../components/Button'
import { Cover } from '../components/Cover'
import { plural } from '../format'
import { type Film, label } from '../letterboxd'
import { type Found, type Matched, matchFilms, useFind } from '../tmdb'
import home from './Home.module.css'
import styles from './Import.module.css'

type Item = components['schemas']['ImportedItem']

const batch = 100

export function MatchFilms({
  films,
  filename,
  profileID,
  onDone,
  onAnother,
}: {
  films: Film[]
  filename: string
  profileID: number
  onDone: () => void
  onAnother: () => void
}) {
  const [matched, setMatched] = useState<Matched[]>([])
  const [failed, setFailed] = useState<string>()
  const [tries, setTries] = useState(0)
  const [picks, setPicks] = useState<Record<number, Found | null>>({})

  // biome-ignore lint/correctness/useExhaustiveDependencies: tries restarts matching after a failure
  useEffect(() => {
    let stopped = false
    setFailed(undefined)
    ;(async () => {
      const done: Matched[] = []
      for (let i = 0; i < films.length && !stopped; i += batch) {
        done.push(...(await matchFilms(films.slice(i, i + batch))))
        if (!stopped) setMatched([...done])
      }
    })().catch((e: Error) => !stopped && setFailed(e.message))
    return () => {
      stopped = true
    }
  }, [films, tries])

  const send = useMutation({
    mutationFn: async () => {
      const history: Item[] = []
      const watchlist: Item[] = []
      const ratings: Item[] = []
      films.forEach((f, i) => {
        const tmdb = (i in picks ? picks[i] : matched[i]?.match)?.tmdb
        const base = { type: 'movie' as const, tmdb, label: label(f) }
        for (const p of f.plays) history.push({ ...base, at: p.at, precision: p.at ? 'day' : undefined })
        if (f.rating) ratings.push({ ...base, rating: f.rating, at: f.ratedAt })
        if (f.listedAt) watchlist.push({ ...base, at: f.listedAt })
      })
      const { error } = await api.POST('/v1/imports', {
        params: { query: { profile_id: profileID, filename } },
        body: { source: 'letterboxd', history, watchlist, ratings },
      })
      if (error) throw error
    },
    onSuccess: onDone,
  })

  if (failed) {
    return (
      <div className={styles.review}>
        <p className={home.error}>{failed}</p>
        <div className={styles.actions}>
          <Button variant="primary" onClick={() => setTries(tries + 1)}>
            Try again
          </Button>
          <Button variant="text" onClick={onAnother}>
            Choose another file
          </Button>
        </div>
      </div>
    )
  }

  if (matched.length < films.length) {
    return (
      <div className={styles.review}>
        <p>
          Matching {matched.length.toLocaleString('en-GB')} of {plural(films.length, 'film')} from Letterboxd…
        </p>
        <span className={styles.meter}>
          <span style={{ transform: `scaleX(${matched.length / films.length})` }} />
        </span>
      </div>
    )
  }

  const unsure = films.map((f, i) => ({ film: f, i })).filter(({ i }) => !matched[i].match)

  return (
    <div className={styles.review}>
      <p>
        {plural(films.length, 'film')} in the file.{' '}
        {unsure.length
          ? `${(films.length - unsure.length).toLocaleString('en-GB')} matched by name and year. ${unsure.length === 1 ? 'Pick the right film for the last one, or leave it out.' : `Pick the right film for each of the other ${unsure.length.toLocaleString('en-GB')}, or leave them out.`}`
          : 'Every one matched by name and year.'}
      </p>
      {unsure.length > 0 && (
        <ol className={styles.films}>
          {unsure.map(({ film, i }) => (
            <Unsure
              key={i}
              film={film}
              candidates={matched[i].candidates}
              picked={picks[i] ?? null}
              onPick={(f) => setPicks({ ...picks, [i]: f })}
            />
          ))}
        </ol>
      )}
      <p className={home.note}>Nothing is saved yet. Next you’ll see what will come across before anything is added.</p>
      <div className={styles.actions}>
        <Button variant="primary" onClick={() => send.mutate()} aria-busy={send.isPending} disabled={send.isPending}>
          Carry on
        </Button>
        <Button variant="text" onClick={onAnother}>
          Choose another file
        </Button>
      </div>
      {send.isError && <p className={home.error}>{problemMessage(send.error)}</p>}
    </div>
  )
}

function Unsure({
  film,
  candidates,
  picked,
  onPick,
}: {
  film: Film
  candidates: Found[]
  picked: Found | null
  onPick: (f: Found | null) => void
}) {
  const [draft, setDraft] = useState('')
  const [query, setQuery] = useState('')
  const found = useFind(query)
  const films = [...candidates, ...(found.data ?? []).filter((f) => f.type === 'movie')]
  const options = films.filter((f, i) => films.findIndex((o) => o.tmdb === f.tmdb) === i)
  if (picked && !options.some((o) => o.tmdb === picked.tmdb)) options.unshift(picked)
  const name = `film-${label(film)}`

  const search = (e: FormEvent) => {
    e.preventDefault()
    setQuery(draft.trim())
  }

  return (
    <li className={styles.film}>
      <p className={home.name}>{label(film)}</p>
      <fieldset className={styles.choices}>
        <legend className={home.visuallyHidden}>Which film is {label(film)}?</legend>
        {options.map((o) => (
          <label key={o.tmdb} className={picked?.tmdb === o.tmdb ? `${styles.choice} ${styles.chosen}` : styles.choice}>
            <input type="radio" name={name} checked={picked?.tmdb === o.tmdb} onChange={() => onPick(o)} />
            <Cover id={o.tmdb} name={o.name} poster={o.poster ?? undefined} size="xs" />
            <span>
              {o.name}
              {o.year && <span className={home.kind}> {o.year}</span>}
            </span>
          </label>
        ))}
        <label className={picked ? styles.choice : `${styles.choice} ${styles.chosen}`}>
          <input type="radio" name={name} checked={!picked} onChange={() => onPick(null)} />
          <span>Leave it out</span>
        </label>
      </fieldset>
      <form className={styles.find} onSubmit={search}>
        <input
          className={styles.findInput}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          placeholder="Search for it"
          aria-label={`Search for ${label(film)}`}
        />
        <Button variant="plain" type="submit" disabled={draft.trim().length < 2}>
          Search
        </Button>
      </form>
      {found.isError && <p className={home.error}>{(found.error as Error).message}</p>}
      {found.data && !found.data.some((f) => f.type === 'movie') && <p className={home.note}>No films called that.</p>}
    </li>
  )
}
