import { useMutation, useQuery } from '@tanstack/react-query'
import { type FormEvent, useEffect, useState } from 'react'
import { api, problemMessage } from '../api/client'
import type { components } from '../api/schema'
import { Button } from '../components/Button'
import { Cover } from '../components/Cover'
import { plural } from '../format'
import { type Film, type FilmList, label } from '../letterboxd'
import { type Found, useFind } from '../tmdb'
import home from './Home.module.css'
import styles from './Import.module.css'

type Item = components['schemas']['ImportedItem']
type TitleItem = components['schemas']['Item']
type Pick = { tethr?: number; tmdb?: number; name: string; year?: number; poster?: string }
type Matched = { match: Pick | null; candidates: Pick[] }

const batch = 100

const fromItem = (i: TitleItem): Pick => ({
  tethr: i.title.ids.tethr,
  tmdb: i.title.ids.tmdb,
  name: i.title.name ?? 'Untitled',
  year: i.title.year,
})
const fromTMDB = (f: Found): Pick => ({
  tmdb: f.tmdb,
  name: f.name,
  year: f.year ?? undefined,
  poster: f.poster ?? undefined,
})
const key = (p: Pick) => (p.tethr ? `t${p.tethr}` : `m${p.tmdb}`)
const same = (a: Pick, b: Pick) => key(a) === key(b) || (!!a.tmdb && a.tmdb === b.tmdb)

async function matchFilms(films: Film[], signal: AbortSignal, onWait: (waiting: boolean) => void): Promise<Matched[]> {
  for (;;) {
    const { data, response } = await api.POST('/v1/match', {
      body: { films: films.map((f) => ({ name: f.name, year: f.year })) },
      signal,
    })
    if (response.status === 429) {
      onWait(true)
      await new Promise((r) => setTimeout(r, Number(response.headers.get('Retry-After') ?? 5) * 1000))
      signal.throwIfAborted()
      continue
    }
    onWait(false)
    if (response.status === 401) throw new Error('You’ve been signed out. Sign in again, then choose your file again.')
    if (!data) throw new Error('Matching isn’t working right now. Try again in a moment.')
    return data.results.map((r) => ({
      match: r.match ? fromItem(r.match) : null,
      candidates: r.candidates.map(fromItem),
    }))
  }
}

export function MatchFilms({
  films,
  lists,
  unreadable,
  filename,
  profileID,
  onDone,
  onAnother,
}: {
  films: Film[]
  lists: FilmList[]
  unreadable: string[]
  filename: string
  profileID: number
  onDone: () => void
  onAnother: () => void
}) {
  const [matched, setMatched] = useState<Matched[]>([])
  const [failed, setFailed] = useState<string>()
  const [tries, setTries] = useState(0)
  const [waiting, setWaiting] = useState(false)
  const [picks, setPicks] = useState<Record<number, Pick | null>>({})

  // biome-ignore lint/correctness/useExhaustiveDependencies: tries carries on matching after a failure
  useEffect(() => {
    const stop = new AbortController()
    setFailed(undefined)
    ;(async () => {
      const done = [...matched]
      for (let i = done.length; i < films.length; i += batch) {
        done.push(...(await matchFilms(films.slice(i, i + batch), stop.signal, setWaiting)))
        setMatched([...done])
      }
    })().catch((e: Error) => !stop.signal.aborted && setFailed(e.message))
    return () => stop.abort()
  }, [films, tries])

  const send = useMutation({
    mutationFn: async () => {
      const history: Item[] = []
      const watchlist: Item[] = []
      const ratings: Item[] = []
      const base = films.map((f, i) => {
        const pick = i in picks ? picks[i] : matched[i]?.match
        return { type: 'movie' as const, tethr: pick?.tethr, tmdb: pick?.tmdb, label: label(f) }
      })
      films.forEach((f, i) => {
        for (const p of f.plays) history.push({ ...base[i], at: p.at, precision: p.at ? 'day' : undefined })
        if (f.rating) ratings.push({ ...base[i], rating: f.rating, at: f.ratedAt })
        if (f.listedAt) watchlist.push({ ...base[i], at: f.listedAt })
      })
      const imported = lists.map((l) => ({
        name: l.name,
        description: l.description || undefined,
        items: l.entries.map((e) => ({ ...base[e.film], at: l.at, notes: e.notes || undefined })),
      }))
      const { error } = await api.POST('/v1/imports', {
        params: { query: { profile_id: profileID, filename } },
        body: { source: 'letterboxd', history, watchlist, ratings, lists: imported },
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
          {waiting && ' That’s a lot of films at once, so Tethr is waiting a moment before carrying on.'}
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
              profileID={profileID}
              film={film}
              candidates={matched[i].candidates}
              picked={picks[i] ?? null}
              onPick={(f) => setPicks({ ...picks, [i]: f })}
            />
          ))}
        </ol>
      )}
      {unreadable.length > 0 && (
        <p className={home.error}>
          {unreadable.length === 1
            ? 'One list in the file couldn’t be read'
            : `${unreadable.length} lists in the file couldn’t be read`}
          , so {unreadable.length === 1 ? 'it won’t' : 'they won’t'} come across: {unreadable.join(', ')}.
        </p>
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
  profileID,
  film,
  candidates,
  picked,
  onPick,
}: {
  profileID: number
  film: Film
  candidates: Pick[]
  picked: Pick | null
  onPick: (p: Pick | null) => void
}) {
  const [draft, setDraft] = useState('')
  const [query, setQuery] = useState('')
  const [further, setFurther] = useState('')
  const found = useQuery({
    queryKey: ['search', 'catalogue', profileID, 'movie', query],
    enabled: query.length >= 2,
    queryFn: async () => {
      const { data } = await api.GET('/v1/search', {
        params: { query: { profile_id: profileID, q: query, scope: 'catalogue', type: 'movie' } },
      })
      if (!data) throw new Error('Search isn’t working right now. Try again in a moment.')
      return data.map(fromItem)
    },
  })
  const asking = !!query && (further === query || found.data?.length === 0)
  const elsewhere = useFind(asking ? query : '')
  const films = [
    ...candidates,
    ...(found.data ?? []),
    ...(elsewhere.data ?? []).filter((f) => f.type === 'movie').map(fromTMDB),
  ]
  const options = films.filter((f, i) => films.findIndex((o) => same(o, f)) === i)
  if (picked && !options.some((o) => same(o, picked))) options.unshift(picked)
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
          <label
            key={key(o)}
            className={picked && same(picked, o) ? `${styles.choice} ${styles.chosen}` : styles.choice}
          >
            <input type="radio" name={name} checked={!!picked && same(picked, o)} onChange={() => onPick(o)} />
            <Cover id={o.tethr ?? o.tmdb ?? 0} name={o.name} poster={o.poster} size="xs" />
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
      {elsewhere.isError && <p className={home.error}>{(elsewhere.error as Error).message}</p>}
      {found.data && !asking && (
        <button type="button" className={styles.further} onClick={() => setFurther(query)}>
          Not finding it? Look further
        </button>
      )}
      {asking && elsewhere.data && !found.data?.length && !elsewhere.data.some((f) => f.type === 'movie') && (
        <p className={home.note}>No films called that.</p>
      )}
    </li>
  )
}
