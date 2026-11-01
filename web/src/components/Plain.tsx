import { Link } from '@tanstack/react-router'
import type { ReactNode } from 'react'
import { Mark } from './Mark'
import styles from './Plain.module.css'

export function Plain({ children, end, wide }: { children: ReactNode; end?: ReactNode; wide?: boolean }) {
  return (
    <div className={styles.plain}>
      <header className={styles.top}>
        <Link to="/" className={styles.wordmark} aria-label="Tethr">
          <Mark />
          tethr
        </Link>
        {end && <div className={styles.end}>{end}</div>}
      </header>
      <main className={wide ? `${styles.main} ${styles.wide}` : styles.main}>{children}</main>
    </div>
  )
}
