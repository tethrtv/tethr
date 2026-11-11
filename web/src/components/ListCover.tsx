import type { components } from '../api/schema'
import { useDetails } from '../tmdb'
import { Cover } from './Cover'
import styles from './ListCover.module.css'

type Item = components['schemas']['Item']

const key = (i: Item) =>
  [i.type, i.title.ids.tethr ?? i.title.ids.tmdb, i.season?.number, i.episode?.season, i.episode?.number].join('-')

export function ListCover({ items, size = 's' }: { items: Item[]; size?: 's' | 'l' }) {
  return (
    <span className={`${styles.cover} ${styles[size]} ${items.length > 1 ? styles.grid : ''}`} aria-hidden="true">
      {items.map((i) => (
        <Poster key={key(i)} item={i} size={size} />
      ))}
    </span>
  )
}

function Poster({ item, size }: { item: Item; size: 's' | 'l' }) {
  const title = useDetails(item.type === 'movie' ? 'movie' : 'show', item.title.ids.tmdb)
  return (
    <Cover
      id={item.title.ids.tethr ?? 0}
      name={title?.name ?? item.title.name}
      poster={title?.poster}
      size={size === 'l' ? 's' : 'xs'}
    />
  )
}

export function TethrCover({ size = 's' }: { size?: 's' | 'l' }) {
  return (
    <span className={`${styles.cover} ${styles[size]} ${styles.tethr}`} aria-hidden="true">
      <svg viewBox="0 0 32 32" className={styles.glyph} aria-hidden="true">
        <path d="M9 21c3-9 11-1 14-10" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round" />
        <circle cx="9" cy="21" r="3.2" fill="currentColor" />
        <circle cx="23" cy="11" r="3.2" fill="currentColor" />
      </svg>
      {size === 'l' && <span className={styles.made}>Made by Tethr</span>}
    </span>
  )
}
