import { useMutation } from '@tanstack/react-query'
import { Link, useSearch } from '@tanstack/react-router'
import { useEffect, useRef, useState } from 'react'
import { api, problemMessage } from '../api/client'
import { Button } from '../components/Button'
import { CodeInput } from '../components/CodeInput'
import { Plain } from '../components/Plain'
import auth from './Auth.module.css'
import styles from './Connect.module.css'
import { ProfilePicker, useSignedIn } from './connect'

const LENGTH = 8

type Step = { kind: 'code' } | { kind: 'approve'; app: string } | { kind: 'done'; app: string } | { kind: 'declined' }

export function Activate() {
  const me = useSignedIn()
  const { code: prefill } = useSearch({ from: '/activate' })
  const [code, setCode] = useState(prefill ?? '')
  const [step, setStep] = useState<Step>({ kind: 'code' })

  const look = useMutation({
    mutationFn: async (code: string) => {
      const { data, error } = await api.GET('/v1/devices/{code}', { params: { path: { code } } })
      if (error) throw error
      return data
    },
    onSuccess: (device) => setStep({ kind: 'approve', app: device.app_name }),
  })

  const started = useRef(false)
  useEffect(() => {
    if (started.current || !me || prefill?.length !== LENGTH) return
    started.current = true
    look.mutate(prefill)
  }, [me, prefill, look])

  const enter = (next: string) => {
    setCode(next)
    look.reset()
    if (next.length === LENGTH) look.mutate(next)
  }

  const restart = () => {
    setCode('')
    look.reset()
    setStep({ kind: 'code' })
  }

  if (!me) return <Plain>{null}</Plain>

  return (
    <Plain
      end={
        <Link to="/" className={auth.link}>
          Your account
        </Link>
      }
    >
      <div className={styles.page}>
        <div className={auth.slip}>
          {step.kind === 'code' && (
            <div className={auth.section}>
              <h1 className={auth.title}>Connect a TV or app</h1>
              <p className={auth.quiet}>
                Type the eight letters and numbers it’s showing. Capitals, dashes and spaces don’t matter.
              </p>
              <p className={styles.label}>Code from your screen</p>
              <CodeInput
                label="Code from your screen"
                value={code}
                onChange={enter}
                length={LENGTH}
                letters
                disabled={look.isPending}
              />
              <p className={look.isError ? `${auth.status} ${auth.error}` : auth.status} aria-live="polite">
                {look.isPending
                  ? 'Looking for your TV…'
                  : look.isError
                    ? problemMessage(look.error)
                    : `${LENGTH - code.length} to go`}
              </p>
            </div>
          )}
          {step.kind === 'approve' && (
            <Approve
              code={code}
              app={step.app}
              me={me}
              onDone={() => setStep({ kind: 'done', app: step.app })}
              onDeclined={() => setStep({ kind: 'declined' })}
            />
          )}
          {step.kind === 'done' && (
            <div className={`${auth.section} ${styles.done}`}>
              <span className={styles.stamp}>Connected</span>
              <h1 className={auth.title}>Look back at your TV</h1>
              <p className={auth.quiet}>
                {step.app} will finish signing in by itself in a few seconds. Anything you watch from now on lands in
                your history.
              </p>
              <div className={auth.actions}>
                <Link to="/" className={auth.link}>
                  Back to your account
                </Link>
                <Button variant="plain" onClick={restart}>
                  Connect another
                </Button>
              </div>
            </div>
          )}
          {step.kind === 'declined' && (
            <div className={auth.section}>
              <h1 className={auth.title}>Not connected</h1>
              <p className={auth.quiet}>Nothing was shared. The code on your TV runs out by itself in a few minutes.</p>
              <div className={auth.actions}>
                <Button variant="plain" onClick={restart}>
                  Enter a different code
                </Button>
              </div>
            </div>
          )}
        </div>
        <p className={styles.note}>
          The code comes from the sign-in screen of the app you’re connecting. Codes last 10 minutes.
        </p>
      </div>
    </Plain>
  )
}

function Approve({
  code,
  app,
  me,
  onDone,
  onDeclined,
}: {
  code: string
  app: string
  me: NonNullable<ReturnType<typeof useSignedIn>>
  onDone: () => void
  onDeclined: () => void
}) {
  const [profile, setProfile] = useState(me.profiles[0]?.id ?? 0)
  const approve = useMutation({
    mutationFn: async () => {
      const { error } = await api.POST('/v1/devices/{code}/approve', {
        params: { path: { code } },
        body: { profile_id: profile },
      })
      if (error) throw error
    },
    onSuccess: onDone,
  })
  const deny = useMutation({
    mutationFn: async () => {
      await api.POST('/v1/devices/{code}/deny', { params: { path: { code } } })
    },
    onSettled: onDeclined,
  })

  return (
    <div className={auth.section}>
      <p className={styles.codeLine}>
        Code {code.slice(0, 4)}-{code.slice(4)}
      </p>
      <h1 className={auth.title}>{app} wants to connect</h1>
      <Permissions />
      <ProfilePicker legend="Log plays from it to" profiles={me.profiles} value={profile} onChange={setProfile} />
      <div className={auth.actions}>
        <Button variant="primary" aria-busy={approve.isPending} onClick={() => approve.mutate()}>
          Connect {app}
        </Button>
        <Button variant="plain" aria-busy={deny.isPending} onClick={() => deny.mutate()}>
          Not now
        </Button>
      </div>
      {approve.isError && <p className={`${auth.status} ${auth.error}`}>{problemMessage(approve.error)}</p>}
    </div>
  )
}

export function Permissions() {
  return (
    <div className={styles.rows}>
      <p className={styles.rowLabel}>It will be able to</p>
      <ul className={styles.list}>
        <li>log what you play, while you play it</li>
        <li>read and change your history, resume points and watchlist</li>
      </ul>
      <p className={styles.rowLabel}>It won’t see</p>
      <p className={auth.quiet}>your email address, your other apps, or anyone else’s history.</p>
    </div>
  )
}
