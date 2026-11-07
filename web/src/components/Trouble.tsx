import { Link } from '@tanstack/react-router'
import { Button } from './Button'
import buttons from './Button.module.css'
import { Plain } from './Plain'
import { Telly } from './Telly'
import styles from './Trouble.module.css'

export function NotFound() {
  return (
    <Plain>
      <div className={styles.trouble}>
        <Telly />
        <h1 className={styles.title}>Nothing on this channel</h1>
        <p className={styles.lead}>There’s no page at this address. The link might be old, or have a typo in it.</p>
        <p className={styles.actions}>
          <Link to="/" className={`${buttons.primary} ${styles.button}`}>
            Go to Up next
          </Link>
          <Link to="/lists" className={styles.textLink}>
            Your lists
          </Link>
        </p>
      </div>
    </Plain>
  )
}

export function Broken({ error }: { error: unknown }) {
  const detail = error instanceof Error ? error.message : String(error)
  return (
    <Plain>
      <div className={styles.trouble}>
        <Telly broken />
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
