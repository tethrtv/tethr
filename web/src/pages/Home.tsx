import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { type FormEvent, useEffect, useState } from 'react'
import { api, type Me, problemMessage } from '../api/client'
import type { components } from '../api/schema'
import { Button } from '../components/Button'
import { Field } from '../components/Field'
import { Made } from '../components/Mark'
import { Plain } from '../components/Plain'
import { Slip, SlipBody, SlipFoot } from '../components/Slip'
import { describeDevice } from '../device'
import { addPasskey, passkeysSupported } from '../passkeys'
import { returnTo } from '../returnTo'
import styles from './Home.module.css'

type Profile = components['schemas']['Profile']
type Connection = components['schemas']['Connection']
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
            Signed in as <strong>{account.email}</strong>. Moving from another tracker?{' '}
            <Link to="/import" className={styles.inline}>
              Import your history
            </Link>
            .
          </p>
        </header>
        <Household me={me.data} />
        <Invites />
        <ConnectedApps />
        <SigningIn />
        <YourData />
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
  const [slip, setSlip] = useState<'edit' | 'remove' | null>(null)
  const [name, setName] = useState(profile.name)
  const [username, setUsername] = useState(profile.username)
  const open = (step: 'edit' | 'remove') => {
    setName(profile.name)
    setUsername(profile.username)
    setSlip(step)
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
        <Button variant="text" onClick={() => open('edit')}>
          Edit
        </Button>
        {profile.kind !== 'owner' && (
          <Button variant="text" className={styles.danger} onClick={() => open('remove')}>
            Remove
          </Button>
        )}
      </span>
      <Slip
        open={slip !== null}
        onClose={() => setSlip(null)}
        step={slip ?? 'edit'}
        title={slip === 'remove' ? `Remove ${profile.name}?` : `Change ${profile.name}`}
      >
        {slip === 'remove' ? (
          <RemoveProfile profile={profile} onKeep={() => setSlip('edit')} onChange={onChange} />
        ) : (
          <EditProfile
            profile={profile}
            name={name}
            username={username}
            setName={setName}
            setUsername={setUsername}
            onRemove={() => setSlip('remove')}
            onClose={() => setSlip(null)}
            onChange={onChange}
          />
        )}
      </Slip>
    </li>
  )
}

function EditProfile({
  profile,
  name,
  username,
  setName,
  setUsername,
  onRemove,
  onClose,
  onChange,
}: {
  profile: Profile
  name: string
  username: string
  setName: (v: string) => void
  setUsername: (v: string) => void
  onRemove: () => void
  onClose: () => void
  onChange: () => void
}) {
  const [counted, setCounted] = useState(profile.in_popular)
  const counts = profile.kind === 'owner' || profile.kind === 'person'
  const save = useMutation({
    mutationFn: async () => {
      const { error } = await api.PATCH('/v1/profiles/{id}', {
        params: { path: { id: profile.id } },
        body: { name, username, in_popular: counts ? counted : undefined },
      })
      if (error) throw error
    },
    onSuccess: () => {
      onChange()
      onClose()
    },
  })
  const form = `profile-${profile.id}`
  return (
    <>
      <SlipBody>
        <form
          id={form}
          className={styles.slipForm}
          onSubmit={(e) => {
            e.preventDefault()
            save.mutate()
          }}
        >
          <Field label="Name" value={name} onChange={setName} />
          <Field
            label="Username"
            value={username}
            onChange={(v) => setUsername(v.toLowerCase())}
            spellCheck={false}
            hint="Apps show this, and so does anything this profile shares."
          />
          {counts && (
            <label className={styles.check}>
              <input type="checkbox" checked={counted} onChange={(e) => setCounted(e.target.checked)} />
              <span>
                Count {profile.kind === 'owner' ? 'your' : 'their'} plays in Popular on Tethr
                <span className={styles.checkNote}>
                  A title only shows once 10 households have played it in the last week, and it never says who.
                </span>
              </span>
            </label>
          )}
          {save.isError && <p className={styles.error}>{problemMessage(save.error)}</p>}
          {profile.kind !== 'owner' && (
            <p>
              <Button variant="text" className={styles.danger} onClick={onRemove}>
                Remove this profile
              </Button>
            </p>
          )}
        </form>
      </SlipBody>
      <SlipFoot>
        <Button variant="primary" type="submit" form={form} aria-busy={save.isPending}>
          Save
        </Button>
        <Button variant="text" onClick={onClose}>
          Cancel
        </Button>
      </SlipFoot>
    </>
  )
}

function RemoveProfile({ profile, onKeep, onChange }: { profile: Profile; onKeep: () => void; onChange: () => void }) {
  const remove = useMutation({
    mutationFn: async () => {
      const { error } = await api.DELETE('/v1/profiles/{id}', { params: { path: { id: profile.id } } })
      if (error) throw error
    },
    onSuccess: onChange,
  })
  return (
    <>
      <SlipBody>
        <p>
          Their history, ratings and lists go with them, and it can’t be undone. Apps connected as them stop logging.
        </p>
        {remove.isError && <p className={styles.error}>{problemMessage(remove.error)}</p>}
      </SlipBody>
      <SlipFoot>
        <Button variant="danger" onClick={() => remove.mutate()} aria-busy={remove.isPending}>
          Remove {profile.name}
        </Button>
        <Button variant="text" onClick={onKeep}>
          Keep them
        </Button>
      </SlipFoot>
    </>
  )
}

function ConnectedApps() {
  const connections = useQuery({
    queryKey: ['connections'],
    queryFn: async () => (await api.GET('/v1/connections')).data ?? [],
  })
  return (
    <section>
      <h2 className={styles.sectionTitle}>Connected apps</h2>
      <p className={styles.note}>
        Open the app, choose Tethr, and it will show a short code.{' '}
        <Link to="/activate" className={styles.inline}>
          Enter the code here
        </Link>
        . Disconnecting an app stops it logging, and never touches your history. Run Jellyfin, Plex or Emby?{' '}
        <Link to="/media-servers" className={styles.inline}>
          Log plays from the server
        </Link>
        . Building a player?{' '}
        <Link to="/developers" className={styles.inline}>
          Register it here
        </Link>
        .
      </p>
      {connections.data?.length ? (
        <ul className={styles.rows}>
          {connections.data.map((c) => (
            <ConnectionRow key={c.id} connection={c} />
          ))}
        </ul>
      ) : null}
    </section>
  )
}

function ConnectionRow({ connection: c }: { connection: Connection }) {
  const queries = useQueryClient()
  const [slip, setSlip] = useState<'rename' | 'disconnect' | null>(null)
  const [label, setLabel] = useState(c.label ?? '')
  const refresh = () => queries.invalidateQueries({ queryKey: ['connections'] })
  const open = (step: 'rename' | 'disconnect') => {
    setLabel(c.label ?? '')
    setSlip(step)
  }

  return (
    <li className={styles.row}>
      <span className={styles.name}>
        {c.app_name}
        {c.official && (
          <span className={styles.handle}>
            {' '}
            <Made />
          </span>
        )}
        {c.label && <span className={styles.handle}> {c.label}</span>}
      </span>
      <span className={styles.kind}>Logs as {c.profile_name}</span>
      <span className={styles.kind}>Last used {when(c.last_used_at ?? c.created_at)}</span>
      <span className={styles.rowActions}>
        <Button variant="text" onClick={() => open('rename')}>
          Rename
        </Button>
        <Button variant="text" className={styles.danger} onClick={() => open('disconnect')}>
          Disconnect
        </Button>
      </span>
      <Slip
        open={slip !== null}
        onClose={() => setSlip(null)}
        step={slip ?? 'rename'}
        title={slip === 'disconnect' ? `Disconnect ${c.app_name}?` : `Rename ${c.app_name}`}
      >
        {slip === 'disconnect' ? (
          <Disconnect connection={c} onKeep={() => setSlip(null)} onDone={refresh} />
        ) : (
          <Rename connection={c} label={label} setLabel={setLabel} onClose={() => setSlip(null)} onDone={refresh} />
        )}
      </Slip>
    </li>
  )
}

function Rename({
  connection: c,
  label,
  setLabel,
  onClose,
  onDone,
}: {
  connection: Connection
  label: string
  setLabel: (v: string) => void
  onClose: () => void
  onDone: () => void
}) {
  const rename = useMutation({
    mutationFn: async () => {
      const { error } = await api.PATCH('/v1/connections/{id}', { params: { path: { id: c.id } }, body: { label } })
      if (error) throw error
    },
    onSuccess: () => {
      onDone()
      onClose()
    },
  })
  const form = `rename-${c.id}`
  return (
    <>
      <SlipBody>
        <form
          id={form}
          className={styles.slipForm}
          onSubmit={(e) => {
            e.preventDefault()
            rename.mutate()
          }}
        >
          <Field
            label="Name"
            value={label}
            onChange={setLabel}
            placeholder="Living room"
            hint="To tell two of the same app apart."
          />
          {rename.isError && <p className={styles.error}>{problemMessage(rename.error)}</p>}
        </form>
      </SlipBody>
      <SlipFoot>
        <Button variant="primary" type="submit" form={form} aria-busy={rename.isPending}>
          Save
        </Button>
        <Button variant="text" onClick={onClose}>
          Cancel
        </Button>
      </SlipFoot>
    </>
  )
}

function Disconnect({
  connection: c,
  onKeep,
  onDone,
}: {
  connection: Connection
  onKeep: () => void
  onDone: () => void
}) {
  const disconnect = useMutation({
    mutationFn: async () => {
      const { error } = await api.DELETE('/v1/connections/{id}', { params: { path: { id: c.id } } })
      if (error) throw error
    },
    onSuccess: onDone,
  })
  return (
    <>
      <SlipBody>
        <p>
          It stops logging as {c.profile_name} straight away, and never touches your history. To use it again, it’ll
          need a new code.
        </p>
        {disconnect.isError && <p className={styles.error}>{problemMessage(disconnect.error)}</p>}
      </SlipBody>
      <SlipFoot>
        <Button variant="danger" onClick={() => disconnect.mutate()} aria-busy={disconnect.isPending}>
          Disconnect
        </Button>
        <Button variant="text" onClick={onKeep}>
          Keep it
        </Button>
      </SlipFoot>
    </>
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
      </div>
    </section>
  )
}

function Invites() {
  const queries = useQueryClient()
  const [copied, setCopied] = useState('')
  const invites = useQuery({
    queryKey: ['invites'],
    queryFn: async () => (await api.GET('/v1/invites')).data ?? null,
  })
  const make = useMutation({
    mutationFn: async () => {
      const { error } = await api.POST('/v1/invites')
      if (error) throw error
    },
    onSuccess: () => queries.invalidateQueries({ queryKey: ['invites'] }),
  })
  const copy = (url: string) =>
    navigator.clipboard
      ?.writeText(url)
      .then(() => setCopied(url))
      .catch(() => {})

  const data = invites.data
  if (!data || data.open) return null
  return (
    <section>
      <h2 className={styles.sectionTitle}>Invites</h2>
      <div className={styles.group}>
        <div className={styles.groupHead}>
          <h3>
            {data.left === 0 ? 'No invites left' : `${data.left} ${data.left === 1 ? 'invite' : 'invites'} to share`}
          </h3>
          {data.left > 0 && !data.paused && (
            <Button variant="plain" onClick={() => make.mutate()} aria-busy={make.isPending}>
              Make an invite
            </Button>
          )}
        </div>
        <p className={styles.note}>
          Tethr is invite-only for now. Send a link to someone you think would like it. Each one works once, and they’ll
          see your username when they join.
          {data.paused && ' New invites are paused for a little while, but the ones you’ve made still work.'}
        </p>
        {make.isError && <p className={styles.error}>{problemMessage(make.error)}</p>}
        {data.invites.length > 0 && (
          <ul className={styles.rows}>
            {data.invites.map((i) => (
              <li key={i.code} className={styles.row}>
                <span className={`${styles.name} ${styles.code}`}>{i.code}</span>
                <span className={styles.kind}>
                  {i.used_by ? `Used by @${i.used_by}` : i.used_at ? 'Used' : `Made ${when(i.created_at)}`}
                </span>
                <span className={styles.rowActions}>
                  {!i.used_at && (
                    <Button variant="text" onClick={() => copy(i.url)}>
                      {copied === i.url ? 'Link copied' : 'Copy the link'}
                    </Button>
                  )}
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  )
}

function YourData() {
  const navigate = useNavigate()
  const download = useMutation({
    mutationFn: async () => {
      const { data, error } = await api.GET('/v1/export', { parseAs: 'blob' })
      if (error || !data) throw error
      const link = document.createElement('a')
      link.href = URL.createObjectURL(data)
      link.download = `tethr-${new Date().toISOString().slice(0, 10)}.zip`
      link.click()
      URL.revokeObjectURL(link.href)
    },
  })
  const code =
    download.error && typeof download.error === 'object' && 'code' in download.error ? download.error.code : ''

  return (
    <section>
      <h2 className={styles.sectionTitle}>Your data</h2>
      <p className={styles.note}>
        A zip of everything in the household: every profile’s history, ratings, watchlist and resume points, plus the
        account and connected apps. It holds a JSON file and a spreadsheet of your history. You can download it twice a
        day, and we email you each time.
      </p>
      {code === 'sign_in_again' ? (
        <div className={styles.rowActions}>
          <p className={styles.note}>{problemMessage(download.error)}</p>
          <Button
            onClick={() => {
              returnTo.set('/account')
              navigate({ to: '/sign-in' })
            }}
          >
            Sign in again
          </Button>
        </div>
      ) : (
        <>
          <p>
            <Button onClick={() => download.mutate()} aria-busy={download.isPending} disabled={download.isPending}>
              {download.isPending ? 'Putting it together' : 'Download everything'}
            </Button>
          </p>
          {download.isError && <p className={styles.error}>{problemMessage(download.error)}</p>}
        </>
      )}
    </section>
  )
}

function DeleteAccount({ me }: { me: Me }) {
  const [open, setOpen] = useState(false)
  return (
    <section>
      <h2 className={styles.sectionTitle}>Delete the account</h2>
      <p className={styles.note}>
        This removes you, everyone in the household and all of your history. Backups roll off within 30 days.
      </p>
      <p>
        <Button variant="text" className={styles.danger} onClick={() => setOpen(true)}>
          Delete account
        </Button>
      </p>
      <Slip open={open} onClose={() => setOpen(false)} title="Delete the whole account?">
        <ConfirmDelete me={me} onKeep={() => setOpen(false)} />
      </Slip>
    </section>
  )
}

function ConfirmDelete({ me, onKeep }: { me: Me; onKeep: () => void }) {
  const navigate = useNavigate()
  const queries = useQueryClient()
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
    <>
      <SlipBody>
        <form
          id="delete-account"
          className={styles.slipForm}
          onSubmit={(e) => {
            e.preventDefault()
            remove.mutate()
          }}
        >
          <p>
            Every profile in the household goes, with all its history, ratings and lists. It can’t be undone. If you
            might want your history later, download it first.
          </p>
          <Field
            label={`Type ${owner?.username ?? 'your username'} to confirm`}
            value={typed}
            onChange={setTyped}
            autoCapitalize="none"
            autoComplete="off"
            spellCheck={false}
            error={remove.isError ? problemMessage(remove.error) : undefined}
          />
        </form>
      </SlipBody>
      <SlipFoot>
        <Button
          variant="danger"
          type="submit"
          form="delete-account"
          disabled={typed !== owner?.username}
          aria-busy={remove.isPending}
        >
          Delete everything
        </Button>
        <Button variant="text" onClick={onKeep}>
          Keep my account
        </Button>
      </SlipFoot>
    </>
  )
}
