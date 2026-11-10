import { useId } from 'react'
import styles from './Television.module.css'

export function Television({
  still,
  poster,
  name,
  on,
}: {
  still?: string
  poster?: string
  name: string
  on: boolean
}) {
  const clip = `tv-${useId().replace(/[^a-zA-Z0-9_-]/g, '')}`
  return (
    <g className={on ? `${styles.set} ${styles.on}` : styles.set}>
      <defs>
        <clipPath id={clip}>
          <rect x="34" y="62" width="128" height="104" rx="14" />
        </clipPath>
      </defs>
      <g className={styles.aerials}>
        <path d="M120 44L92 8" />
        <path d="M120 44L156 14" />
        <circle cx="92" cy="8" r="3" />
        <circle cx="156" cy="14" r="3" />
      </g>
      <rect x="18" y="44" width="204" height="140" rx="12" className={styles.cabinet} />
      <rect x="34" y="62" width="128" height="104" rx="14" className={styles.glass} />
      <g clipPath={`url(#${clip})`}>
        <g className={styles.picture}>
          {still ? (
            <image href={still} x="34" y="62" width="128" height="104" preserveAspectRatio="xMidYMid slice" />
          ) : poster ? (
            <image href={poster} x="34" y="40" width="128" height="192" preserveAspectRatio="xMidYMid slice" />
          ) : (
            <>
              <rect x="34" y="62" width="128" height="104" className={styles.blank} />
              <rect x="34" y="100" width="128" height="28" className={styles.band} />
              <text x="98" y="119" textAnchor="middle" className={styles.bandText}>
                {name.length > 16 ? `${name.slice(0, 15)}…` : name}
              </text>
            </>
          )}
        </g>
      </g>
      <rect x="34" y="62" width="128" height="104" rx="14" className={styles.bezel} />
      <circle cx="194" cy="84" r="11" className={styles.knob} />
      <path d="M194 76v8" />
      <circle cx="194" cy="118" r="11" className={styles.knob} />
      <path d="M188 114l6 4" />
      <path d="M182 148h24M182 156h24M182 164h24" />
      <path d="M46 184l-10 28M194 184l10 28" />
    </g>
  )
}
