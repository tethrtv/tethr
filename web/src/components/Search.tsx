import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { api } from '../api/client'
import { useFind } from '../tmdb'
import styles from './Search.module.css'
import { TitleLink } from './TitleLink'

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
  const elsewhere = useFind(settled)
  const library: Hit[] = (q ? (found.data ?? []) : []).map((r) => ({
    key: `t${r.title.ids.tethr}`,
    type: r.type,
    slug: r.title.slug,
    name: r.title.name ?? 'Untitled',
    year: r.title.year,
  }))
  const held = new Set((found.data ?? []).map((r) => `${r.type}-${r.title.ids.tmdb}`))
  const others: Hit[] = (settled && q ? (elsewhere.data ?? []) : [])
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
        <TitleLink type={r.type} slug={r.slug} className={styles.result} onClick={close}>
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
          const first = [...library, ...others][0]
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
              {elsewhere.isError ? (
                <p className={styles.none}>{elsewhere.error.message}</p>
              ) : settled !== q || elsewhere.isPending ? (
                <p className={styles.none}>Looking…</p>
              ) : others.length ? (
                <ul className={styles.list}>{list(others)}</ul>
              ) : (
                <p className={styles.none}>Nothing else matches.</p>
              )}
            </>
          )}
          {q.length < 2 && !library.length && <p className={styles.none}>Keep typing…</p>}
        </div>
      )}
    </search>
  )
}
