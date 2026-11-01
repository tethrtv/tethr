import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { type FormEvent, useEffect, useState } from 'react'
import { api, type Me, problemMessage } from '../api/client'
import type { components } from '../api/schema'
import { Button } from '../components/Button'
import { Field } from '../components/Field'
import { Plain } from '../components/Plain'
import { describeDevice } from '../device'
import { addPasskey, passkeysSupported } from '../passkeys'
import styles from './Home.module.css'

type Profile = components['schemas']['Profile']
const kinds = { owner: 'You', person: 'Person', kids: 'Kids', guest: 'Guest' } as const
const when = (iso?: string) =>
  iso ? new Date(iso).toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' }) : 'never'

export function Home() {
  const navigate = useNavigate()
  const queries = useQueryClient()
  const me = useQuery({
    queryKey: ['me'],
    queryFn: async () => {
      const { data, response } = await api.GET('/v1/me')
      if (response.status === 401) return null
      if (!data) throw new Error('could not load your account')
      return data
    },
  })

  const signOut = useMutation({
    mutationFn: () => api.POST('/v1/auth/sign-out'),
    onSuccess: () => {
      queries.setQueryData(['me'], null)
      navigate({ to: '/sign-in' })
    },
  })

  useEffect(() => {
    if (me.data === null) navigate({ to: '/sign-in', replace: true })
  }, [me.data, navigate])

  if (!me.data) return <Plain>{me.isError ? <p>Couldn’t reach Tethr. Try again in a moment.</p> : null}</Plain>

  const { account } = me.data
  return (
    <Plain
      wide
      end={
        <Button variant="text" onClick={() => signOut.mutate()}>
          Sign out
        </Button>
      }
    >
      <div className={styles.page}>
        <header className={styles.head}>
          <h1 className={styles.title}>{account.name}</h1>
          <p className={styles.lead}>
            Signed in as <strong>{account.email}</strong>. Up next, history and connected apps arrive here soon.
          </p>
        </header>
        <Household me={me.data} />
        <SigningIn />
        <DeleteAccount me={me.data} />
      </div>
    </Plain>
  )
}

function Household({ me }: { me: Me }) {
  const queries = useQueryClient()
  const [name, setName] = useState('')
  const [kind, setKind] = useState<'person' | 'kids' | 'guest'>('person')
  const refresh = () => queries.invalidateQueries({ queryKey: ['me'] })

  const add = useMutation({
    mutationFn: async () => {
      const { error } = await api.POST('/v1/profiles', { body: { name, kind } })
      if (error) throw error
    },
    onSuccess: () => {
      setName('')
      setKind('person')
      refresh()
    },
  })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    add.mutate()
  }

  return (
    <section>
      <h2 className={styles.sectionTitle}>Household</h2>
      <p className={styles.note}>Everyone gets their own history. Approving a TV picks which profile it logs to.</p>
      <ul className={styles.rows}>
        {me.profiles.map((p) => (
          <ProfileRow key={p.id} profile={p} onChange={refresh} />
        ))}
      </ul>
      <form className={styles.addForm} onSubmit={submit}>
        <Field label="Add someone" value={name} onChange={setName} placeholder="Their name" />
        <fieldset className={styles.kinds}>
          <legend className={styles.visuallyHidden}>Kind of profile</legend>
          {(['person', 'kids', 'guest'] as const).map((k) => (
            <label key={k} className={kind === k ? `${styles.kind} ${styles.chosen}` : styles.kind}>
              <input type="radio" name="kind" checked={kind === k} onChange={() => setKind(k)} />
              {kinds[k]}
            </label>
          ))}
        </fieldset>
        <Button variant="plain" aria-busy={add.isPending} disabled={!name.trim()}>
          Add
        </Button>
        {add.isError && <p className={styles.error}>{problemMessage(add.error)}</p>}
      </form>
    </section>
  )
}

function ProfileRow({ profile, onChange }: { profile: Profile; onChange: () => void }) {
  const [editing, setEditing] = useState(false)
  const [confirming, setConfirming] = useState(false)
  const [name, setName] = useState(profile.name)
  const [username, setUsername] = useState(profile.username)

  const save = useMutation({
    mutationFn: async () => {
      const { error } = await api.PATCH('/v1/profiles/{id}', {
        params: { path: { id: profile.id } },
        body: { name, username },
      })
      if (error) throw error
    },
    onSuccess: () => {
      setEditing(false)
      onChange()
    },
  })
  const remove = useMutation({
    mutationFn: async () => {
      const { error } = await api.DELETE('/v1/profiles/{id}', { params: { path: { id: profile.id } } })
      if (error) throw error
    },
    onSuccess: onChange,
  })

  if (editing) {
    return (
      <li className={styles.editing}>
        <Field label="Name" value={name} onChange={setName} />
        <Field label="Username" value={username} onChange={(v) => setUsername(v.toLowerCase())} spellCheck={false} />
        <div className={styles.rowActions}>
          <Button variant="primary" onClick={() => save.mutate()} aria-busy={save.isPending}>
            Save
          </Button>
          <Button variant="text" onClick={() => setEditing(false)}>
            Cancel
          </Button>
        </div>
        {save.isError && <p className={styles.error}>{problemMessage(save.error)}</p>}
      </li>
    )
  }

  return (
    <li className={styles.row}>
      <span className={styles.name}>{profile.name}</span>
      <span className={styles.handle}>{profile.username}</span>
      <span className={styles.kind}>
        {kinds[profile.kind]}
        {profile.private ? ', private' : ''}
      </span>
      <span className={styles.rowActions}>
        {confirming ? (
          <>
            <span className={styles.warn}>Their history goes too.</span>
            <Button variant="text" onClick={() => setConfirming(false)}>
              Keep
            </Button>
            <Button variant="text" className={styles.danger} onClick={() => remove.mutate()}>
              Remove
            </Button>
          </>
        ) : (
          <>
            <Button variant="text" onClick={() => setEditing(true)}>
              Edit
            </Button>
            {profile.kind !== 'owner' && (
              <Button variant="text" className={styles.danger} onClick={() => setConfirming(true)}>
                Remove
              </Button>
            )}
          </>
        )}
      </span>
    </li>
  )
}

function SigningIn() {
  const passkeys = useQuery({
    queryKey: ['passkeys'],
    queryFn: async () => (await api.GET('/v1/passkeys')).data ?? [],
  })
  const sessions = useQuery({
    queryKey: ['sessions'],
    queryFn: async () => (await api.GET('/v1/sessions')).data ?? [],
  })
  const queries = useQueryClient()

  const add = useMutation({
    mutationFn: () => addPasskey(describeDevice(navigator.userAgent)),
    onSuccess: () => queries.invalidateQueries({ queryKey: ['passkeys'] }),
  })
  const removePasskey = useMutation({
    mutationFn: (id: number) => api.DELETE('/v1/passkeys/{id}', { params: { path: { id } } }),
    onSuccess: () => queries.invalidateQueries({ queryKey: ['passkeys'] }),
  })
  const endSession = useMutation({
    mutationFn: (id: number) => api.DELETE('/v1/sessions/{id}', { params: { path: { id } } }),
    onSuccess: () => queries.invalidateQueries({ queryKey: ['sessions'] }),
  })
  const others = useMutation({
    mutationFn: () => api.POST('/v1/sessions/sign-out-others'),
    onSuccess: () => queries.invalidateQueries({ queryKey: ['sessions'] }),
  })

  return (
    <section>
      <h2 className={styles.sectionTitle}>Signing in</h2>

      <div className={styles.group}>
        <div className={styles.groupHead}>
          <h3>Passkeys</h3>
          {passkeysSupported() && (
            <Button variant="plain" onClick={() => add.mutate()} aria-busy={add.isPending}>
              {add.isPending ? 'Waiting…' : 'Add a passkey'}
            </Button>
          )}
        </div>
        {add.isError && (
          <p className={styles.error}>
            {add.error instanceof Error && add.error.name === 'NotAllowedError'
              ? 'Adding a passkey was cancelled.'
              : problemMessage(add.error)}
          </p>
        )}
        {passkeys.data?.length ? (
          <ul className={styles.rows}>
            {passkeys.data.map((p) => (
              <li key={p.id} className={styles.row}>
                <span className={styles.name}>{p.name}</span>
                <span className={styles.kind}>
                  Added {when(p.created_at)}, last used {when(p.last_used_at)}
                </span>
                <span className={styles.rowActions}>
                  <Button variant="text" className={styles.danger} onClick={() => removePasskey.mutate(p.id)}>
                    Remove
                  </Button>
                </span>
              </li>
            ))}
          </ul>
        ) : (
          <p className={styles.note}>
            No passkeys yet. Add one and you can sign in with Face ID, Touch ID or your phone instead of waiting for an
            email.
          </p>
        )}
      </div>

      <div className={styles.group}>
        <div className={styles.groupHead}>
          <h3>Where you’re signed in</h3>
          {(sessions.data?.length ?? 0) > 1 && (
            <Button variant="text" onClick={() => others.mutate()}>
              Sign out everywhere else
            </Button>
          )}
        </div>
        <ul className={styles.rows}>
          {sessions.data?.map((s) => (
            <li key={s.id} className={styles.row}>
              <span className={styles.name}>{describeDevice(s.user_agent)}</span>
              <span className={styles.kind}>
                {s.current ? <mark className={styles.here}>This browser</mark> : `Last used ${when(s.last_seen_at)}`}
              </span>
              <span className={styles.rowActions}>
                {!s.current && (
                  <Button variant="text" onClick={() => endSession.mutate(s.id)}>
                    Sign out
                  </Button>
                )}
              </span>
            </li>
          ))}
        </ul>
        <p className={styles.note}>TVs and apps will be listed under Connected apps.</p>
      </div>
    </section>
  )
}

function DeleteAccount({ me }: { me: Me }) {
  const navigate = useNavigate()
  const queries = useQueryClient()
  const [open, setOpen] = useState(false)
  const [typed, setTyped] = useState('')
  const owner = me.profiles.find((p) => p.kind === 'owner')

  const remove = useMutation({
    mutationFn: async () => {
      const { error } = await api.DELETE('/v1/me', { body: { username: typed } })
      if (error) throw error
    },
    onSuccess: () => {
      queries.clear()
      navigate({ to: '/sign-in' })
    },
  })

  return (
    <section>
      <h2 className={styles.sectionTitle}>Delete the account</h2>
      <p className={styles.note}>
        This removes you, everyone in the household and all of your history. Backups roll off within 30 days.
      </p>
      {open ? (
        <form
          className={styles.deleteBox}
          onSubmit={(e) => {
            e.preventDefault()
            remove.mutate()
          }}
        >
          <Field
            label={`Type ${owner?.username ?? 'your username'} to confirm`}
            value={typed}
            onChange={setTyped}
            autoCapitalize="none"
            autoComplete="off"
            spellCheck={false}
            error={remove.isError ? problemMessage(remove.error) : undefined}
          />
          <div className={styles.rowActions}>
            <Button variant="danger" disabled={typed !== owner?.username} aria-busy={remove.isPending}>
              Delete everything
            </Button>
            <Button type="button" variant="text" onClick={() => setOpen(false)}>
              Keep my account
            </Button>
          </div>
        </form>
      ) : (
        <p>
          <Button variant="text" className={styles.danger} onClick={() => setOpen(true)}>
            Delete account
          </Button>
        </p>
      )}
    </section>
  )
}
