import { Link } from '@tanstack/react-router'
import { type CSSProperties, useEffect, useRef, useState } from 'react'
import type { components } from '../api/schema'
import { ListCover } from '../components/ListCover'
import { TellySet } from '../components/Telly'
import trouble from '../components/Trouble.module.css'
import { plural } from '../format'
import { listParams } from './MyLists'
import styles from './Station.module.css'

type User = components['schemas']['User']

const channels = ['ident', 'test-card', 'weather', 'closedown'] as const
type Channel = (typeof channels)[number] | 'backstage'

const bars = ['#f2f2ec', '#d4fa3c', '#85857d', '#53534c', '#2d2c27', '#d4fa3c', '#141412']

function Band({ lines }: { lines: string[] }) {
  const height = 14 + lines.length * 16
  return (
    <g transform="rotate(-4 143 146)">
      <rect x={143 - 66} y={146 - height / 2} width={132} height={height} rx="3" className={trouble.band} />
      {lines.map((l, i) => (
        <text key={l} x="143" y={146 - height / 2 + 20 + i * 16} textAnchor="middle" className={trouble.bandText}>
          {l}
        </text>
      ))}
    </g>
  )
}

function Picture({ channel }: { channel: Channel }) {
  switch (channel) {
    case 'ident':
      return (
        <g className={styles.ident}>
          <path d="M62 96q0-8 8-8l146-2q8 0 8 8l1 104q0 8-8 8l-146 3q-8 0-8-8z" className={styles.glass} />
          <path d="M108 164c10-30 36-4 46-34" className={styles.markLine} />
          <circle cx="108" cy="164" r="9" className={styles.markDot} />
          <circle cx="154" cy="130" r="9" className={styles.markDot} />
          <text x="182" y="190" textAnchor="middle" className={styles.identText}>
            ONE
          </text>
        </g>
      )
    case 'test-card':
      return (
        <g>
          {bars.map((fill, i) => (
            <rect key={fill + String(i)} x={66 + i * 22.6} y={92} width={22.6} height={112} fill={fill} />
          ))}
          <Band lines={['TEST CARD T']} />
        </g>
      )
    case 'weather':
      return <Band lines={['WEATHER', 'CLOUDY, WITH A', 'CHANCE OF', 'REWATCHES']} />
    case 'closedown':
      return <Band lines={['CLOSEDOWN', 'BACK AFTER', 'THE NEXT EPISODE']} />
    case 'backstage':
      return <Band lines={['BACKSTAGE', 'MADE WITH FAR', 'TOO MUCH TEA']} />
  }
}

function useSecret(word: string, found: () => void) {
  const typed = useRef('')
  const done = useRef(found)
  done.current = found
  useEffect(() => {
    const listen = (e: KeyboardEvent) => {
      if (e.key.length !== 1 || (e.target instanceof Element && e.target.closest('input, textarea'))) return
      typed.current = (typed.current + e.key.toLowerCase()).slice(-word.length)
      if (typed.current === word) done.current()
    }
    window.addEventListener('keydown', listen)
    return () => window.removeEventListener('keydown', listen)
  }, [word])
}

function StationTelly() {
  const [channel, setChannel] = useState<Channel>('ident')
  const [flicker, setFlicker] = useState(false)
  const timer = useRef(0)
  useEffect(() => () => clearTimeout(timer.current), [])
  const tune = (next: Channel) => {
    setFlicker(true)
    clearTimeout(timer.current)
    timer.current = window.setTimeout(() => {
      setFlicker(false)
      setChannel(next)
    }, 220)
  }
  useSecret('tethr', () => tune('backstage'))
  const index = channels.indexOf(channel as (typeof channels)[number])
  const number = channel === 'backstage' ? 0 : index + 1

  return (
    <div className={`${trouble.set} ${styles.set}`}>
      <svg className={trouble.telly} viewBox="0 0 320 250" role="img" aria-label="An old television tuned to Tethr One">
        <TellySet />
        <g className={trouble.lines}>
          <path
            d="M250 99v10"
            className={trouble.dial}
            style={{ transform: `rotate(${Math.max(index, 0) * 60}deg)` }}
          />
        </g>
        <text x="251" y="212" textAnchor="middle" className={trouble.channel}>
          CH {number}
        </text>
        <defs>
          <clipPath id="station-screen">
            <path d="M62 96q0-8 8-8l146-2q8 0 8 8l1 104q0 8-8 8l-146 3q-8 0-8-8z" />
          </clipPath>
        </defs>
        <g clipPath="url(#station-screen)">
          <g className={flicker ? styles.off : styles.on}>
            <Picture channel={channel} />
          </g>
        </g>
      </svg>
      <button
        type="button"
        className={trouble.knob}
        onClick={() => tune(channels[(Math.max(index, -1) + 1) % channels.length])}
        aria-label="Change the channel"
      />
    </div>
  )
}

const slot = (i: number) => `${String(18 + i).padStart(2, '0')}:00`

export function Station({ user }: { user: User }) {
  return (
    <div className={styles.page}>
      <title>Tethr One · Tethr</title>
      <header className={styles.head}>
        <div className={styles.words}>
          <h1 className={styles.title}>Tethr One</h1>
          <p className={styles.lead}>
            Tethr’s own channel. Lists Tethr makes and keeps up to date, free to follow, and fine to watch in any order
            you like.
          </p>
          <p className={styles.hint}>Have a go at the knob.</p>
        </div>
        <StationTelly />
      </header>

      <section>
        <h2 className={styles.sectionTitle}>Tonight on Tethr</h2>
        {user.lists.length === 0 ? (
          <p className={styles.quiet}>Nothing on yet. The first lists are coming soon.</p>
        ) : (
          <ol className={styles.schedule}>
            {user.lists.map((l, i) => (
              <li key={l.id} className={styles.slot} style={{ '--i': i } as CSSProperties}>
                <span className={styles.time}>{slot(i)}</span>
                <ListCover items={l.cover} />
                <span className={styles.programme}>
                  <Link to="/@{$username}/lists/$slug" params={listParams(l)} className={styles.name}>
                    {l.name}
                  </Link>
                  {l.description && <span className={styles.blurb}>{l.description}</span>}
                </span>
                <span className={styles.meta}>
                  {plural(l.item_count, 'title')}
                  {l.follower_count > 0 && `, ${plural(l.follower_count, 'follower')}`}
                </span>
              </li>
            ))}
            <li className={`${styles.slot} ${styles.closedown}`}>
              <span className={styles.time}>{slot(user.lists.length)}</span>
              <span />
              <span className={styles.programme}>Closedown</span>
              <span className={styles.meta}>Back tomorrow</span>
            </li>
          </ol>
        )}
      </section>
    </div>
  )
}
