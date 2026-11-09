import { Link, useRouterState } from '@tanstack/react-router'
import { createContext, type ReactNode, useContext, useState } from 'react'
import type { Me } from '../api/client'
import { useSignedIn } from '../pages/connect'
import { Mark } from './Mark'
import { Search } from './Search'
import styles from './Shell.module.css'
import { Slip, SlipBody } from './Slip'

const links = [
  { to: '/', label: 'Today' },
  { to: '/up-next', label: 'Up next' },
  { to: '/history', label: 'History' },
  { to: '/year', label: String(new Date().getFullYear()) },
  { to: '/lists', label: 'Lists' },
  { to: '/account', label: 'Account' },
] as const

type Watching = { me: Me; profile: Me['profiles'][number] }
const WatchingContext = createContext<Watching | null>(null)

export function useWatching() {
  const w = useContext(WatchingContext)
  if (!w) throw new Error('useWatching needs a Shell')
  return w
}

function stored() {
  try {
    return Number(localStorage.getItem('tethr.profile')) || 0
  } catch {
    return 0
  }
}

const section = (path: string) =>
  path === '/year' || path.startsWith('/year/')
    ? '/year'
    : path === '/watchlist' || path.startsWith('/lists/') || /^\/@[^/]+\/lists\//.test(path)
      ? '/lists'
      : path

export function Shell({ children }: { children: ReactNode }) {
  const me = useSignedIn()
  const path = section(useRouterState({ select: (s) => s.location.pathname }))
  const [chosen, setChosen] = useState(stored)
  const [more, setMore] = useState(false)

  if (!me) return null
  const profile = me.profiles.find((p) => p.id === chosen) ?? me.profiles[0]
  const choose = (id: number) => {
    setChosen(id)
    try {
      localStorage.setItem('tethr.profile', String(id))
    } catch {}
  }

  return (
    <WatchingContext.Provider value={{ me, profile }}>
      <div className={styles.shell}>
        <header className={styles.topbar}>
          <div className={styles.row}>
            <Link to="/" className={styles.wordmark} aria-label="Tethr, today">
              <Mark />
              tethr
            </Link>
            <nav className={styles.nav} aria-label="Main">
              {links.map((l) => (
                <Link
                  key={l.to}
                  to={l.to}
                  className={path === l.to ? `${styles.link} ${styles.active}` : styles.link}
                  aria-current={path === l.to ? 'page' : undefined}
                >
                  {l.label}
                </Link>
              ))}
            </nav>
            <Search key={profile.id} profileID={profile.id} />
            {me.profiles.length > 1 && (
              <label className={styles.switcher}>
                <span className={styles.faint}>Watching as</span>
                <select value={profile.id} onChange={(e) => choose(Number(e.target.value))}>
                  {me.profiles.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name}
                    </option>
                  ))}
                </select>
              </label>
            )}
          </div>
        </header>

        <main className={styles.main}>{children}</main>

        <footer className={styles.footer}>
          <div className={styles.footerRow}>
            <span className={styles.credit}>
              <img
                src="https://www.themoviedb.org/assets/v4/logos/v2/blue_short-8e7b30f73a4020692ccca9c88bafe5dcb6f8a62a4c6bc55cd9ba82bb2cd95f6c.svg"
                alt="TMDB"
                height="12"
              />
              <span className={styles.faint}>
                Details and images from TMDB. Tethr uses the TMDB API but isn’t endorsed or certified by TMDB.
              </span>
            </span>
            <span className={styles.faint}>
              Free and open source. <Link to="/terms">Terms</Link> · <Link to="/privacy">Privacy</Link>
            </span>
          </div>
        </footer>

        <nav className={styles.tabbar} aria-label="Main">
          {links.slice(0, 3).map((l) => (
            <Link key={l.to} to={l.to} className={path === l.to ? `${styles.tab} ${styles.active}` : styles.tab}>
              {l.label}
            </Link>
          ))}
          <button
            type="button"
            className={links.slice(3).some((l) => l.to === path) ? `${styles.tab} ${styles.active}` : styles.tab}
            onClick={() => setMore(true)}
          >
            More
          </button>
        </nav>
        <Slip open={more} onClose={() => setMore(false)} title="More">
          <SlipBody>
            <ul className={styles.moreLinks}>
              {links.slice(3).map((l) => (
                <li key={l.to}>
                  <Link to={l.to} onClick={() => setMore(false)}>
                    {l.label}
                  </Link>
                </li>
              ))}
            </ul>
          </SlipBody>
        </Slip>
      </div>
    </WatchingContext.Provider>
  )
}
