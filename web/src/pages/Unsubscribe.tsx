import { useMutation } from '@tanstack/react-query'
import { Link, useSearch } from '@tanstack/react-router'
import { api, problemMessage } from '../api/client'
import { Button } from '../components/Button'
import { Plain } from '../components/Plain'
import styles from './Auth.module.css'

export function Unsubscribe() {
  const { token } = useSearch({ from: '/unsubscribe' })
  const stop = useMutation({
    mutationFn: async () => {
      const { error } = await api.POST('/v1/unsubscribe', { params: { query: { token } } })
      if (error) throw error
    },
  })

  return (
    <Plain>
      <div className={styles.slip}>
        <div className={styles.section}>
          {stop.isSuccess ? (
            <>
              <h1 className={styles.title}>You’re unsubscribed</h1>
              <p className={styles.quiet}>
                You won’t get the weekly email any more. Changed your mind? Turn it back on from{' '}
                <Link to="/account" className={styles.link}>
                  your account page
                </Link>
                .
              </p>
            </>
          ) : !token || (stop.error && 'code' in stop.error && stop.error.code === 'not_found') ? (
            <>
              <h1 className={styles.title}>That link didn’t work</h1>
              <p className={styles.quiet}>
                You can still turn the weekly email off on{' '}
                <Link to="/account" className={styles.link}>
                  your account page
                </Link>
                .
              </p>
            </>
          ) : (
            <>
              <h1 className={styles.title}>Stop the weekly email?</h1>
              <p className={styles.quiet}>
                It’s the Sunday email with the week ahead. Tethr will still send sign-in links and anything about your
                account.
              </p>
              <p>
                <Button
                  variant="primary"
                  onClick={() => stop.mutate()}
                  aria-busy={stop.isPending}
                  disabled={stop.isPending}
                >
                  Unsubscribe
                </Button>
              </p>
              {stop.isError && <p className={styles.error}>{problemMessage(stop.error)}</p>}
            </>
          )}
        </div>
      </div>
    </Plain>
  )
}
