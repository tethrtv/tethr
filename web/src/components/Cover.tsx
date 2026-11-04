import { useState } from 'react'
import { image } from '../tmdb'
import styles from './Cover.module.css'

const inks = ['#2f4a3a', '#5a2f2a', '#283b5a', '#5a4a24', '#3d2f55', '#24504f', '#55303f', '#3a3a36']

export function Cover({
  id,
  name,
  poster,
  size = 's',
}: {
  id: number
  name?: string
  poster?: string
  size?: 'xs' | 's' | 'l'
}) {
  const [failed, setFailed] = useState(false)
  const src = image(poster, size === 'xs' ? 'w92' : size === 'l' ? 'w342' : 'w185')
  if (src && !failed) {
    return (
      <img
        className={`${styles.cover} ${styles[size]}`}
        src={src}
        alt=""
        loading="lazy"
        onError={() => setFailed(true)}
      />
    )
  }
  return (
    <span
      className={`${styles.cover} ${styles[size]} ${styles.type}`}
      style={{ background: inks[id % inks.length] }}
      aria-hidden="true"
    >
      <span>{name}</span>
    </span>
  )
}
