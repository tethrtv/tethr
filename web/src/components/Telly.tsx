import styles from './Trouble.module.css'

const staticLines = [
  'M74 108q5-4 10 0t10 0t10 0t10 0t10 0t10 0',
  'M92 124q5-4 10 0t10 0t10 0t10 0t10 0t10 0t10 0',
  'M70 176q5-4 10 0t10 0t10 0t10 0',
  'M128 184q5-4 10 0t10 0t10 0t10 0t10 0t10 0',
]

export function Telly({ broken }: { broken?: boolean }) {
  return (
    <svg className={styles.telly} viewBox="0 0 320 250" role="img" aria-hidden="true">
      <g className={styles.lines}>
        <path d="M152 70L112 18" />
        <path d="M164 70L214 24" />
        <circle cx="110" cy="15" r="5" />
        <circle cx="216" cy="21" r="5" />
        <path d="M44 78q-3-8 6-9l214-4q10 0 10 9l3 138q0 9-9 10l-212 6q-10 0-10-9z" className={styles.body} />
        <path d="M62 96q0-8 8-8l146-2q8 0 8 8l1 104q0 8-8 8l-146 3q-8 0-8-8z" className={styles.screen} />
        <circle cx="250" cy="104" r="9" />
        <circle cx="250" cy="138" r="9" />
        <path d="M250 99v10M246 136l8 4" />
        <path d="M240 168h22M240 180h22M240 192h22" />
        <path d="M84 225l-10 18M236 222l10 18" />
        {broken ? (
          <>
            <path d="M190 90l-14 30l12 8l-20 32l10 6l-14 30" />
            <path className={styles.smoke} d="M268 60q-14-12 0-22t0-22" />
            <path className={styles.smoke} d="M286 64q-10-10 2-18t0-18" />
          </>
        ) : (
          staticLines.map((d) => <path key={d} d={d} className={styles.static} />)
        )}
      </g>
      <g transform="rotate(-4 143 146)">
        <rect x={broken ? 66 : 84} y="132" width={broken ? 154 : 118} height="28" rx="3" className={styles.band} />
        <text x="143" y="152" textAnchor="middle" className={styles.bandText}>
          {broken ? 'PLEASE STAND BY' : 'NO SIGNAL'}
        </text>
      </g>
    </svg>
  )
}
