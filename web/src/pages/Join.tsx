import { useMutation, useQuery } from '@tanstack/react-query'
import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import { type FormEvent, useState } from 'react'
import { api, type Problem, problemMessage } from '../api/client'
import { Button } from '../components/Button'
import { Field } from '../components/Field'
import { Plain } from '../components/Plain'
import { pendingEmail } from '../pendingEmail'
import styles from './Auth.module.css'

export const toUsername = (name: string) =>
  name
    .toLowerCase()
    .normalize('NFKD')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 24)

const handled = [
  'name_required',
  'username_taken',
  'invalid_username',
  'invalid_email',
  'too_many_emails',
  'invite_required',
  'invalid_invite',
]

export function Join() {
  const navigate = useNavigate()
  const [name, setName] = useState('')
  const [username, setUsername] = useState('')
  const [edited, setEdited] = useState(false)
  const [email, setEmail] = useState('')
  const { invite: invited } = useSearch({ from: '/join' })
  const [invite, setInvite] = useState(invited ?? '')
  const handle = edited ? username : toUsername(name)
  const code = invite.trim()
  const check = useQuery({
    queryKey: ['invite', code],
    enabled: code.length === 9,
    queryFn: async () => (await api.GET('/v1/invites/{code}', { params: { path: { code } } })).data ?? null,
  })
  const inviter = check.data?.valid ? check.data.invited_by : undefined

  const create = useMutation({
    mutationFn: async () => {
      const { error } = await api.POST('/v1/accounts', {
        body: { email, name, username: handle, invite: invite || undefined },
      })
      if (error) throw error
    },
    onSuccess: () => {
      pendingEmail.set(email.trim(), true)
      navigate({ to: '/check-email' })
    },
  })

  const problem = create.error as Problem | null
  const fieldError = (codes: string[]) =>
    problem && codes.includes(problem.code) ? problemMessage(problem) : undefined

  const submit = (e: FormEvent) => {
    e.preventDefault()
    create.mutate()
  }

  return (
    <Plain
      end={
        <Link to="/sign-in" className={styles.link}>
          Sign in
        </Link>
      }
    >
      <form className={styles.slip} onSubmit={submit} noValidate>
        <div className={styles.section}>
          <h1 className={styles.title}>Create an account</h1>
          {inviter && (
            <p className={styles.invited}>
              <mark>@{inviter}</mark> invited you.
            </p>
          )}
          <p className={styles.quiet}>
            Free, with nothing to upgrade to. Your history stays private unless you share it.
          </p>
          <Field
            label="Your name"
            value={name}
            onChange={setName}
            autoComplete="given-name"
            error={fieldError(['name_required'])}
            hint="Used for your profile. Change it any time."
          />
          <Field
            label="Username"
            value={handle}
            onChange={(v) => {
              setEdited(true)
              setUsername(v.toLowerCase())
            }}
            autoComplete="username"
            autoCapitalize="none"
            spellCheck={false}
            error={fieldError(['username_taken', 'invalid_username'])}
            hint="Apps show this once you’ve connected them."
          />
          <Field
            label="Email address"
            type="email"
            value={email}
            onChange={setEmail}
            autoComplete="email"
            inputMode="email"
            spellCheck={false}
            error={fieldError(['invalid_email', 'too_many_emails'])}
            hint="For sign-in links and anything about your account. Nothing else."
          />
          {(invited !== undefined || fieldError(['invite_required', 'invalid_invite'])) && (
            <Field
              label="Invite code"
              value={invite}
              onChange={(v) => setInvite(v.toUpperCase())}
              autoCapitalize="characters"
              spellCheck={false}
              error={
                fieldError(['invite_required', 'invalid_invite']) ??
                (check.data?.valid === false ? 'That code has been used, or isn’t quite right.' : undefined)
              }
              hint="Tethr is invite-only for now."
            />
          )}
          {create.isError && !fieldError(handled) && (
            <p className={`${styles.status} ${styles.error}`}>{problemMessage(create.error)}</p>
          )}
          <div className={styles.actions}>
            <Button variant="primary" aria-busy={create.isPending}>
              {create.isPending ? 'Creating…' : 'Continue'}
            </Button>
          </div>
          <p className={`${styles.quiet} ${styles.small}`}>
            By continuing, you agree to the <Link to="/terms">terms</Link> and have read the{' '}
            <Link to="/privacy">privacy notice</Link>.
          </p>
        </div>
      </form>
    </Plain>
  )
}
