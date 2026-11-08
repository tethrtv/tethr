import { useEffect, useRef, useState } from 'react'
import styles from './Trouble.module.css'

type Screen = 'no-signal' | 'stand-by' | 'lost' | 'starting' | 'off-air'
const extras = ['test-card', 'weather', 'cat', 'normal-service'] as const

const staticLines = [
  'M74 108q5-4 10 0t10 0t10 0t10 0t10 0t10 0',
  'M92 124q5-4 10 0t10 0t10 0t10 0t10 0t10 0t10 0',
  'M70 176q5-4 10 0t10 0t10 0t10 0',
  'M128 184q5-4 10 0t10 0t10 0t10 0t10 0t10 0',
]

const noise = [100, 112, 124, 136, 148, 160, 172, 184, 196].map(
  (y, i) => `M${70 + (i % 3) * 4} ${y}q5-4 10 0t10 0t10 0t10 0t10 0t10 0t10 0t10 0t10 0t10 0t10 0t10 0t10 0t10 0`,
)

function Band({ lines, width, first }: { lines: string[]; width: number; first?: boolean }) {
  const height = 14 + lines.length * 16
  return (
    <g transform="rotate(-4 143 146)" className={first ? styles.label : undefined}>
      <rect x={143 - width / 2} y={146 - height / 2} width={width} height={height} rx="3" className={styles.band} />
      {lines.map((l, i) => (
        <text key={l} x="143" y={146 - height / 2 + 20 + i * 16} textAnchor="middle" className={styles.bandText}>
          {l}
        </text>
      ))}
    </g>
  )
}

function Picture({ shown, first }: { shown: Screen | (typeof extras)[number]; first: boolean }) {
  switch (shown) {
    case 'no-signal':
      return (
        <>
          <g className={styles.lines}>
            {staticLines.map((d) => (
              <path key={d} d={d} className={styles.static} />
            ))}
          </g>
          <Band lines={['NO SIGNAL']} width={118} first={first} />
        </>
      )
    case 'off-air':
      return (
        <>
          <g className={styles.lines}>
            {staticLines.slice(2).map((d) => (
              <path key={d} d={d} className={styles.static} />
            ))}
          </g>
          <Band lines={['OFF AIR']} width={104} first={first} />
        </>
      )
    case 'stand-by':
      return (
        <>
          <g className={styles.lines}>
            <path d="M190 90l-14 30l12 8l-20 32l10 6l-14 30" />
            <path className={styles.smoke} d="M268 60q-14-12 0-22t0-22" />
            <path className={styles.smoke} d="M286 64q-10-10 2-18t0-18" />
          </g>
          <Band lines={['PLEASE STAND BY']} width={154} first={first} />
        </>
      )
    case 'lost':
      return (
        <>
          <g className={styles.lines}>
            {staticLines.slice(0, 2).map((d) => (
              <path key={d} d={d} className={styles.static} />
            ))}
          </g>
          <Band lines={['SEARCHING']} width={118} first={first} />
        </>
      )
    case 'starting':
      return (
        <>
          <Picture shown="test-card" first={false} />
          <Band lines={['STARTING SOON']} width={146} first={first} />
        </>
      )
    case 'test-card':
      return (
        <g className={styles.lines}>
          {[0, 1, 2, 3, 4, 5].map((i) => (
            <rect
              key={i}
              x={72 + i * 24}
              y="98"
              width="24"
              height="100"
              className={styles.bar}
              style={{ fillOpacity: 0.08 + i * 0.15 }}
            />
          ))}
          <circle cx="143" cy="148" r="30" className={styles.body} />
          <text x="143" y="153" textAnchor="middle" className={styles.cardText}>
            TETHR
          </text>
        </g>
      )
    case 'weather':
      return (
        <g className={styles.lines}>
          <path className={styles.static} d="M70 120q30-16 60 0t60 0t30-6" />
          <path className={styles.static} d="M70 170q30-14 60 0t60 0t30-4" />
          <circle cx="100" cy="145" r="12" />
          <path d="M100 124v-6M100 172v-6M79 145h-6M127 145h-6M85 130l-4-4M115 160l4 4M85 160l-4 4M115 130l4-4" />
          <path
            d="M150 160q-14 0-12-13q2-10 14-8q4-14 20-12q14 2 15 15q12 0 12 10q0 9-11 8z"
            className={styles.cloud}
          />
          <text x="196" y="196" textAnchor="middle" className={styles.cardText}>
            H
          </text>
        </g>
      )
    case 'cat':
      return (
        <g className={styles.lines}>
          {staticLines.map((d) => (
            <path key={d} d={d} className={styles.static} />
          ))}
          <path d="M222 67c-2-22 6-36 24-36s26 14 24 35" className={styles.body} />
          <path d="M249 22l1-14l8 8l9-9l2 15" className={styles.body} />
          <circle cx="258" cy="28" r="13" className={styles.body} />
          <path d="M250 29q3 3 6 0M260 29q3 3 6 0" />
          <path d="M224 64c-12 4-14 20-6 32" />
        </g>
      )
    case 'normal-service':
      return <Band lines={['NORMAL SERVICE', 'WILL RESUME', 'SHORTLY']} width={140} />
  }
}

export function TellySet({ lost }: { lost?: boolean }) {
  return (
    <g className={`${styles.lines} ${styles.draw}`}>
      {lost ? (
        <>
          <path pathLength={1} d="M152 70L130 40L96 52" />
          <circle pathLength={1} cx="92" cy="54" r="5" />
        </>
      ) : (
        <>
          <path pathLength={1} d="M152 70L112 18" />
          <circle pathLength={1} cx="110" cy="15" r="5" />
        </>
      )}
      <path pathLength={1} d="M164 70L214 24" />
      <circle pathLength={1} cx="216" cy="21" r="5" />
      <path
        pathLength={1}
        d="M44 78q-3-8 6-9l214-4q10 0 10 9l3 138q0 9-9 10l-212 6q-10 0-10-9z"
        className={styles.body}
      />
      <path pathLength={1} d="M62 96q0-8 8-8l146-2q8 0 8 8l1 104q0 8-8 8l-146 3q-8 0-8-8z" className={styles.screen} />
      <circle pathLength={1} cx="250" cy="104" r="9" />
      <circle pathLength={1} cx="250" cy="138" r="9" />
      <path pathLength={1} d="M246 136l8 4" />
      <path pathLength={1} d="M240 168h22M240 180h22M240 192h22" />
      <path pathLength={1} d="M84 225l-10 18M236 222l10 18" />
    </g>
  )
}

export function Telly({ screen, channel }: { screen: Screen; channel: number }) {
  const [hops, setHops] = useState(0)
  const [flicker, setFlicker] = useState(false)
  const [touched, setTouched] = useState(false)
  const timer = useRef(0)
  useEffect(() => () => clearTimeout(timer.current), [])
  const shown = hops === 0 ? screen : extras[hops - 1]
  const hop = () => {
    setFlicker(true)
    setTouched(true)
    clearTimeout(timer.current)
    timer.current = window.setTimeout(() => {
      setFlicker(false)
      setHops((h) => (h + 1) % (extras.length + 1))
    }, 220)
  }

  return (
    <div className={styles.set}>
      <svg className={styles.telly} viewBox="0 0 320 250" role="img" aria-hidden="true">
        <TellySet lost={screen === 'lost'} />
        <g className={styles.lines}>
          <path d="M250 99v10" className={styles.dial} style={{ transform: `rotate(${hops * 60}deg)` }} />
        </g>
        <text x="251" y="212" textAnchor="middle" className={styles.channel}>
          CH {channel + hops}
        </text>
        {flicker ? (
          <g className={`${styles.lines} ${styles.noise}`}>
            {noise.map((d) => (
              <path key={d} d={d} className={styles.static} />
            ))}
          </g>
        ) : (
          <Picture shown={shown} first={hops === 0 && !touched} />
        )}
      </svg>
      <button type="button" className={styles.knob} onClick={hop} aria-label="Change the channel" />
    </div>
  )
}
