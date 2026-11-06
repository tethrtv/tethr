import type { components } from '../api/schema'
import { useDetails } from '../tmdb'
import { Cover } from './Cover'
import styles from './ListCover.module.css'

type Item = components['schemas']['Item']

const key = (i: Item) =>
  [i.type, i.title.ids.tethr ?? i.title.ids.tmdb, i.season, i.episode?.season, i.episode?.number].join('-')

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
