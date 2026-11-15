import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { api } from '../api/client'
import type { components } from '../api/schema'
import { useFind } from '../tmdb'
import styles from './Search.module.css'
import { TitleLink } from './TitleLink'

type Found = components['schemas']['Item']
type Hit = { key: string; type: string; slug?: string; name: string; year?: number }

const kind = (type: string, year?: number | null) => `${type === 'movie' ? 'Film' : 'Show'}${year ? `, ${year}` : ''}`

function useSettled(value: string, ms = 300) {
  const [settled, setSettled] = useState(value)
  useEffect(() => {
    const timer = setTimeout(() => setSettled(value), ms)
    return () => clearTimeout(timer)
  }, [value, ms])
  return settled
}

export function Search({ profileID }: { profileID: number }) {
  const navigate = useNavigate()
  const [words, setWords] = useState('')
  const [open, setOpen] = useState(false)
  const q = words.trim()
  const settled = useSettled(q)
  const found = useQuery({
    queryKey: ['search', profileID, q],
    enabled: q.length > 0,
    placeholderData: keepPreviousData,
    queryFn: async () => (await api.GET('/v1/search', { params: { query: { profile_id: profileID, q } } })).data ?? [],
  })
  const catalogue = useQuery({
    queryKey: ['search', 'catalogue', profileID, settled],
    enabled: settled.length >= 2,
    queryFn: async () => {
      const { data } = await api.GET('/v1/search', {
        params: { query: { profile_id: profileID, q: settled, scope: 'catalogue' } },
      })
      if (!data) throw new Error('Search isn’t working right now. Try again in a moment.')
      return data
    },
  })
  const [further, setFurther] = useState('')
  const asking = settled === q && (further === settled || catalogue.data?.length === 0)
  const elsewhere = useFind(asking ? settled : '')
  const hit = (r: Found): Hit => ({
    key: `t${r.title.ids.tethr}`,
    type: r.type,
    slug: r.title.slug,
    name: r.title.name ?? 'Untitled',
    year: r.title.year,
  })
  const library: Hit[] = (q ? (found.data ?? []) : []).map(hit)
  const mine = new Set(library.map((r) => r.key))
  const everything: Hit[] = (settled === q ? (catalogue.data ?? []) : []).map(hit).filter((r) => !mine.has(r.key))
  const held = new Set([...(found.data ?? []), ...(catalogue.data ?? [])].map((r) => `${r.type}-${r.title.ids.tmdb}`))
  const others: Hit[] = (asking ? (elsewhere.data ?? []) : [])
    .filter((r) => !held.has(`${r.type}-${r.tmdb}`))
    .map((r) => ({
      key: `${r.type}-${r.tmdb}`,
      type: r.type,
      slug: `tmdb-${r.tmdb}`,
      name: r.name,
      year: r.year ?? undefined,
    }))
  const close = () => {
    setOpen(false)
    setWords('')
  }

  const list = (hits: Hit[]) =>
    hits.map((r) => (
      <li key={r.key}>
        <TitleLink
          type={r.type}
          slug={r.slug}
          className={styles.result}
          onClick={close}
          // Safari doesn't focus a link on click, so the search box would blur and close the list first.
          onMouseDown={(e) => e.preventDefault()}
        >
          <span className={styles.name}>{r.name}</span>
          <span className={styles.faint}>{kind(r.type, r.year)}</span>
        </TitleLink>
      </li>
    ))

  return (
    <search
      className={styles.search}
      onBlur={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget)) setOpen(false)
      }}
    >
      <input
        type="search"
        className={styles.input}
        placeholder="Find a film or show"
        aria-label="Find a film or show"
        value={words}
        onChange={(e) => {
          setWords(e.target.value)
          setOpen(true)
        }}
        onFocus={() => setOpen(true)}
        onKeyDown={(e) => {
          if (e.key === 'Escape') close()
          const first = [...library, ...everything, ...others][0]
          if (e.key === 'Enter' && first?.slug) {
            navigate({ to: first.type === 'movie' ? '/films/$slug' : '/shows/$slug', params: { slug: first.slug } })
            close()
          }
        }}
      />
      {open && q && found.data && (
        <div className={styles.results}>
          {!!library.length && (
            <>
              <p className={styles.group}>In your library</p>
              <ul className={styles.list}>{list(library)}</ul>
            </>
          )}
          {q.length >= 2 && (
            <>
              <p className={styles.group}>{library.length ? 'Everything else' : 'Films and shows'}</p>
              {catalogue.isError ? (
                <p className={styles.none}>{catalogue.error.message}</p>
              ) : settled !== q || catalogue.isPending ? (
                <p className={styles.none}>Looking…</p>
              ) : everything.length ? (
                <ul className={styles.list}>{list(everything)}</ul>
              ) : (
                <p className={styles.none}>
                  {catalogue.data?.length ? 'Nothing else matches.' : 'Nothing in Tethr matches.'}
                </p>
              )}
              {catalogue.data && settled === q && !asking && (
                <button
                  type="button"
                  className={styles.further}
                  onMouseDown={(e) => e.preventDefault()}
                  onClick={() => setFurther(settled)}
                >
                  Not finding it? Look further
                </button>
              )}
              {asking && (
                <>
                  <p className={styles.group}>From TMDB</p>
                  {elsewhere.isError ? (
                    <p className={styles.none}>{elsewhere.error.message}</p>
                  ) : elsewhere.isPending ? (
                    <p className={styles.none}>Looking…</p>
                  ) : others.length ? (
                    <ul className={styles.list}>{list(others)}</ul>
                  ) : (
                    <p className={styles.none}>Nothing else matches.</p>
                  )}
                </>
              )}
            </>
          )}
          {q.length < 2 && !library.length && <p className={styles.none}>Keep typing…</p>}
        </div>
      )}
    </search>
  )
}
