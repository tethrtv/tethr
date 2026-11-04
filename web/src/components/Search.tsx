import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { api } from '../api/client'
import styles from './Search.module.css'
import { TitleLink } from './TitleLink'

export function Search({ profileID }: { profileID: number }) {
  const navigate = useNavigate()
  const [words, setWords] = useState('')
  const [open, setOpen] = useState(false)
  const q = words.trim()
  const found = useQuery({
    queryKey: ['search', profileID, q],
    enabled: q.length > 0,
    placeholderData: keepPreviousData,
    queryFn: async () => (await api.GET('/v1/search', { params: { query: { profile_id: profileID, q } } })).data ?? [],
  })
  const results = q ? (found.data ?? []) : []
  const close = () => {
    setOpen(false)
    setWords('')
  }

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
        placeholder="Search your library"
        aria-label="Search your library"
        value={words}
        onChange={(e) => {
          setWords(e.target.value)
          setOpen(true)
        }}
        onFocus={() => setOpen(true)}
        onKeyDown={(e) => {
          if (e.key === 'Escape') close()
          const first = results[0]
          if (e.key === 'Enter' && first?.title.slug) {
            navigate({
              to: first.type === 'movie' ? '/films/$slug' : '/shows/$slug',
              params: { slug: first.title.slug },
            })
            close()
          }
        }}
      />
      {open && q && found.data && (
        <ul className={styles.results}>
          {results.map((r) => (
            <li key={r.title.ids.tethr}>
              <TitleLink type={r.type} slug={r.title.slug} className={styles.result} onClick={close}>
                <span className={styles.name}>{r.title.name ?? 'Untitled'}</span>
                <span className={styles.faint}>
                  {r.type === 'movie' ? 'Film' : 'Show'}
                  {r.title.year ? `, ${r.title.year}` : ''}
                </span>
              </TitleLink>
            </li>
          ))}
          {!results.length && <li className={styles.none}>Nothing you’ve watched, rated or listed matches.</li>}
        </ul>
      )}
    </search>
  )
}
