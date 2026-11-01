import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import { useEffect, useRef } from 'react'
import { api, problemMessage } from '../api/client'
import { Plain } from '../components/Plain'
import styles from './Auth.module.css'

export function Verify() {
  const navigate = useNavigate()
  const queries = useQueryClient()
  const { token } = useSearch({ from: '/verify' })
  const started = useRef(false)

  const verify = useMutation({
    mutationFn: async () => {
      const { data, error } = await api.POST('/v1/auth/verify', { body: { token } })
      if (error) throw error
      return data
    },
    onSuccess: (me) => {
      queries.setQueryData(['me'], me)
      navigate({ to: '/', replace: true })
    },
  })

  useEffect(() => {
    if (started.current || !token) return
    started.current = true
    verify.mutate()
  }, [token, verify])

  return (
    <Plain>
      <div className={styles.slip}>
        <div className={styles.section}>
          {verify.isError || !token ? (
            <>
              <h1 className={styles.title}>That link didn’t work</h1>
              <p className={styles.quiet}>{token ? problemMessage(verify.error) : 'The link is missing its code.'}</p>
              <p>
                <Link to="/sign-in" className={styles.link}>
                  Get a new one
                </Link>
              </p>
            </>
          ) : (
            <h1 className={styles.title}>Signing you in…</h1>
          )}
        </div>
      </div>
    </Plain>
  )
}
