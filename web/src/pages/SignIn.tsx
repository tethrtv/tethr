import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { type FormEvent, useState } from 'react'
import { api, problemMessage } from '../api/client'
import { Button } from '../components/Button'
import { Field } from '../components/Field'
import { Plain } from '../components/Plain'
import { passkeysSupported, signInWithPasskey } from '../passkeys'
import { pendingEmail } from '../pendingEmail'
import { returnTo } from '../returnTo'
import styles from './Auth.module.css'

export function SignIn() {
  const navigate = useNavigate()
  const queries = useQueryClient()
  const [email, setEmail] = useState(pendingEmail.get())
  const passkey = useMutation({
    mutationFn: signInWithPasskey,
    onSuccess: (me) => {
      queries.setQueryData(['me'], me)
      navigate({ href: returnTo.take() })
    },
  })
  const send = useMutation({
    mutationFn: async () => {
      const { error } = await api.POST('/v1/auth/email-link', { body: { email } })
      if (error) throw error
    },
    onSuccess: () => {
      pendingEmail.set(email.trim())
      navigate({ to: '/check-email' })
    },
  })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    send.mutate()
  }

  return (
    <Plain
      end={
        <Link to="/join" className={styles.link}>
          Create an account
        </Link>
      }
    >
      <form className={styles.slip} onSubmit={submit}>
        <div className={styles.section}>
          <h1 className={styles.title}>Sign in</h1>
          {passkeysSupported() && (
            <>
              <Button
                type="button"
                variant="primary"
                className={styles.wide}
                aria-busy={passkey.isPending}
                onClick={() => passkey.mutate()}
              >
                {passkey.isPending ? 'Waiting for your passkey…' : 'Sign in with a passkey'}
              </Button>
              {passkey.isError && (
                <p className={`${styles.status} ${styles.error}`}>
                  {passkey.error instanceof Error && passkey.error.name === 'NotAllowedError'
                    ? 'Passkey sign-in was cancelled.'
                    : problemMessage(passkey.error)}
                </p>
              )}
              <p className={styles.or}>or get a link by email</p>
            </>
          )}
          <p className={styles.quiet}>We’ll email you a link and a code. There’s no password.</p>
          <Field
            label="Email address"
            type="email"
            value={email}
            onChange={setEmail}
            autoComplete="email"
            inputMode="email"
            spellCheck={false}
            required
            error={send.isError ? problemMessage(send.error) : undefined}
          />
          <div className={styles.actions}>
            <Button
              variant={passkeysSupported() ? 'plain' : 'primary'}
              aria-busy={send.isPending}
              disabled={!email.includes('@')}
            >
              {send.isPending ? 'Sending…' : 'Email me a link'}
            </Button>
          </div>
        </div>
      </form>
    </Plain>
  )
}
