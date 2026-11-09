import { Link } from '@tanstack/react-router'
import type { CSSProperties } from 'react'
import type { components } from '../api/schema'
import { plural } from '../format'
import { ListCover } from './ListCover'
import styles from './ListTiles.module.css'
import { Made } from './Mark'

type List = components['schemas']['List']

export function ListTiles({ lists }: { lists: List[] }) {
  return (
    <ul className={styles.tiles}>
      {lists.map((l, i) => (
        <li key={l.id} className={styles.tile} style={{ '--i': i } as CSSProperties}>
          <Link
            to="/@{$username}/lists/$slug"
            params={{ username: l.owner.username, slug: l.slug }}
            className={styles.link}
          >
            <ListCover items={l.cover} size="l" />
            <span className={styles.name}>{l.name}</span>
            <span className={styles.note}>
              {l.owner.official ? <Made /> : `@${l.owner.username}`}
              {l.follower_count > 0 && `, ${plural(l.follower_count, 'follower')}`}
            </span>
          </Link>
        </li>
      ))}
    </ul>
  )
}
