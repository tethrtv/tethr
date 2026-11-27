import { useId, useState } from 'react'
import styles from './Stars.module.css'

const scores = Array.from({ length: 10 }, (_, i) => i + 1)
const star = 'M10 1.6l2.6 5.5 6 .8-4.4 4.2 1.1 6-5.3-2.9-5.3 2.9 1.1-6L1.4 7.9l6-.8z'

export function starWords(rating: number) {
  if (rating === 1) return 'half a star'
  const whole = Math.floor(rating / 2)
  return `${whole}${rating % 2 ? '½' : ''} star${rating === 2 ? '' : 's'}`
}

function Row({ value, small }: { value: number; small?: boolean }) {
  return (
    <span className={small ? `${styles.row} ${styles.small}` : styles.row} aria-hidden="true">
      {[1, 2, 3, 4, 5].map((n) => (
        <span key={n} className={styles.star}>
          <svg viewBox="0 0 20 20" className={styles.empty} aria-hidden="true">
            <path d={star} />
          </svg>
          {value >= n * 2 - 1 && (
            <svg
              viewBox="0 0 20 20"
              className={value >= n * 2 ? styles.full : `${styles.full} ${styles.half}`}
              aria-hidden="true"
            >
              <path d={star} />
            </svg>
          )}
        </span>
      ))}
    </span>
  )
}

export function Rated({ rating, small }: { rating: number; small?: boolean }) {
  return (
    <span className={styles.rated} role="img" aria-label={`Rated ${starWords(rating)}`}>
      <Row value={rating} small={small} />
    </span>
  )
}

export function Stars({
  value,
  label,
  onChange,
  small,
  disabled,
}: {
  value?: number
  label: string
  onChange: (rating: number | null) => void
  small?: boolean
  disabled?: boolean
}) {
  const name = useId()
  const [hover, setHover] = useState<number>()
  return (
    <fieldset
      className={hover ? `${styles.stars} ${styles.hovering}` : styles.stars}
      disabled={disabled}
      onMouseLeave={() => setHover(undefined)}
    >
      <legend className={styles.hidden}>{label}</legend>
      <Row value={hover ?? value ?? 0} small={small} />
      <span className={styles.hits}>
        {scores.map((n) => (
          <label key={n} className={styles.hit} onMouseEnter={() => setHover(n)}>
            <input
              type="radio"
              name={name}
              checked={n === value}
              onChange={() => onChange(n)}
              onClick={() => n === value && onChange(null)}
            />
            <span className={styles.hidden}>{starWords(n)}</span>
          </label>
        ))}
      </span>
    </fieldset>
  )
}
