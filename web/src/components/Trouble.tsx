import { Link, useRouterState } from '@tanstack/react-router'
import { useMe } from '../pages/connect'
import { checkSignal } from '../signal'
import { Button } from './Button'
import buttons from './Button.module.css'
import { Plain } from './Plain'
import { Shell } from './Shell'
import { Telly } from './Telly'
import styles from './Trouble.module.css'

export function NotFound() {
  const me = useMe()
  const path = useRouterState({ select: (s) => s.location.href })
  const time = new Date().toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' })
  if (me.isPending) return null
  const page = (
    <div className={styles.trouble}>
      <title>No signal · Tethr</title>
      <Telly screen="no-signal" channel={404} />
      <h1 className={styles.title}>Nothing on this channel</h1>
      <p className={styles.lead}>There’s no page at this address. The link might be old, or have a typo in it.</p>
      <div className={styles.listing}>
        <span className={styles.time}>{time}</span>
        <div>
          <p className={styles.programme}>{path}</p>
          <p className={styles.note}>Not found. No repeats scheduled.</p>
        </div>
      </div>
      <p className={styles.actions}>
        <Link to="/" className={`${buttons.primary} ${styles.button}`}>
          Go to Up next
        </Link>
        <Link to="/lists" className={styles.textLink}>
          Your lists
        </Link>
      </p>
    </div>
  )
  return me.data ? <Shell>{page}</Shell> : <Plain>{page}</Plain>
}

export function Broken({ error }: { error: unknown }) {
  const detail = error instanceof Error ? error.message : String(error)
  return (
    <Plain>
      <div className={styles.trouble}>
        <title>Stand by · Tethr</title>
        <Telly screen="stand-by" channel={500} />
        <h1 className={styles.title}>Technical difficulties</h1>
        <p className={styles.lead}>
          Something broke on our side, not yours. Loading the page again usually sorts it out.
        </p>
        <p className={styles.actions}>
          <Button variant="primary" onClick={() => window.location.reload()}>
            Try again
          </Button>
          <a href="/" className={styles.textLink}>
            Go to Up next
          </a>
        </p>
        <details className={styles.details}>
          <summary>What went wrong</summary>
          <p className={styles.small}>
            If it keeps happening, please{' '}
            <a href="https://github.com/tethrtv/tethr/issues" target="_blank" rel="noreferrer">
              tell us on GitHub
            </a>{' '}
            with this message and the address of the page.
          </p>
          <code className={styles.code}>{detail}</code>
        </details>
      </div>
    </Plain>
  )
}

export function LostSignal() {
  return (
    <Plain>
      <div className={styles.trouble}>
        <title>Lost signal · Tethr</title>
        <Telly screen="lost" channel={503} />
        <h1 className={styles.title}>Lost signal</h1>
        <p className={styles.lead}>Tethr can’t reach its server. Check your connection, and we’ll keep trying.</p>
        <p className={styles.small}>This page reloads by itself once the signal’s back.</p>
        <p className={styles.actions}>
          <Button variant="primary" onClick={() => checkSignal()}>
            Try now
          </Button>
        </p>
      </div>
    </Plain>
  )
}
