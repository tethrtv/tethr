import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { api, problemMessage } from '../api/client'
import { Button } from '../components/Button'
import { CodeInput } from '../components/CodeInput'
import { Plain } from '../components/Plain'
import { pendingEmail } from '../pendingEmail'
import styles from './Auth.module.css'

export function CheckEmail() {
  const navigate = useNavigate()
  const queries = useQueryClient()
  const email = pendingEmail.get()
  const [code, setCode] = useState('')
  const [wait, setWait] = useState(30)

  const verify = useMutation({
    mutationFn: async (code: string) => {
      const { data, error } = await api.POST('/v1/auth/verify', { body: { email, code } })
      if (error) throw error
      return data
    },
    onSuccess: (me) => {
      queries.setQueryData(['me'], me)
      navigate({ to: '/' })
    },
    onError: () => setCode(''),
  })

  const resend = useMutation({
    mutationFn: async () => {
      const { error } = await api.POST('/v1/auth/email-link', { body: { email } })
      if (error) throw error
    },
    onSuccess: () => setWait(30),
  })

  useEffect(() => {
    if (!wait) return
    const t = setTimeout(() => setWait(wait - 1), 1000)
    return () => clearTimeout(t)
  }, [wait])

  useEffect(() => {
    if (!email) navigate({ to: '/sign-in', replace: true })
  }, [email, navigate])

  const enter = (next: string) => {
    setCode(next)
    if (next.length === 6) verify.mutate(next)
  }

  return (
    <Plain
      end={
        <Link to="/sign-in" className={styles.link}>
          Use a different email
        </Link>
      }
    >
      <div className={styles.slip}>
        <div className={styles.section}>
          <h1 className={styles.title}>Check your email</h1>
          <p className={styles.quiet}>
            {pendingEmail.joining() ? 'We’ve sent a link to ' : 'If '}
            <strong className={styles.strong}>{email}</strong>
            {pendingEmail.joining()
              ? ' to finish creating your account.'
              : ' has an account, there’s a link on its way.'}{' '}
            It works once and runs out after 15 minutes.
          </p>
          <p>Reading it on another device? Type the six numbers from the email.</p>
          <CodeInput label="Code from the email" value={code} onChange={enter} disabled={verify.isPending} />
          <p className={verify.isError ? `${styles.status} ${styles.error}` : styles.status} aria-live="polite">
            {verify.isPending ? 'Checking…' : verify.isError ? problemMessage(verify.error) : ' '}
          </p>
          <p className={`${styles.small} ${styles.rule}`}>
            Nothing yet? Look in spam, or{' '}
            {wait ? (
              <span>send another in 0:{String(wait).padStart(2, '0')}</span>
            ) : (
              <Button variant="text" onClick={() => resend.mutate()}>
                send another
              </Button>
            )}
            .{resend.isError && <span className={styles.error}> {problemMessage(resend.error)}</span>}
          </p>
        </div>
      </div>
    </Plain>
  )
}
