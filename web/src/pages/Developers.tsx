import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { type FormEvent, useState } from 'react'
import { api, problemMessage } from '../api/client'
import type { components } from '../api/schema'
import { Button } from '../components/Button'
import { Field } from '../components/Field'
import { Plain } from '../components/Plain'
import { useSignedIn } from './connect'
import styles from './Developers.module.css'
import home from './Home.module.css'

type DeveloperApp = components['schemas']['DeveloperApp']

const apiURL = import.meta.env.VITE_API_URL ?? 'http://localhost:8080'
const registered = (iso: string) =>
  new Date(iso).toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' })
const splitURIs = (raw: string) => raw.split(/[\s,]+/).filter(Boolean)

export function Developers() {
  const me = useSignedIn()
  const [secret, setSecret] = useState<{ name: string; value: string } | null>(null)
  const apps = useQuery({
    queryKey: ['apps'],
    enabled: !!me,
    queryFn: async () => (await api.GET('/v1/apps')).data ?? [],
  })

  if (!me) return <Plain>{null}</Plain>

  return (
    <Plain
      wide
      end={
        <Link to="/account" className={home.inline}>
          Your account
        </Link>
      }
    >
      <div className={home.page}>
        <header className={home.head}>
          <h1 className={home.title}>Developers</h1>
          <p className={home.lead}>
            Register a player or plugin to get a client ID. Apps built for the Trakt API only need their server address
            changed to <code className={styles.code}>{apiURL.replace(/^https?:\/\//, '')}</code>.
          </p>
        </header>

        {secret && (
          <div className={styles.secret} role="alert">
            <p>
              Here’s the secret for <strong>{secret.name}</strong>. Copy it now. It won’t be shown again.
            </p>
            <div className={styles.secretRow}>
              <code className={styles.secretValue}>{secret.value}</code>
              <Button variant="primary" onClick={() => navigator.clipboard?.writeText(secret.value).catch(() => {})}>
                Copy
              </Button>
              <Button variant="text" onClick={() => setSecret(null)}>
                I’ve saved it
              </Button>
            </div>
          </div>
        )}

        <section>
          <h2 className={home.sectionTitle}>Your apps</h2>
          {apps.data?.length ? (
            <ul className={home.rows}>
              {apps.data.map((a) => (
                <AppRow key={a.id} app={a} onSecret={(value) => setSecret({ name: a.name, value })} />
              ))}
            </ul>
          ) : (
            <p className={home.note}>Nothing registered yet.</p>
          )}
        </section>

        <div className={styles.columns}>
          <Register onRegistered={(name, value) => setSecret({ name, value })} />
          <section>
            <h2 className={home.sectionTitle}>Reference</h2>
            <ul className={styles.links}>
              <li>
                <a href={`${apiURL}/docs`} className={home.inline}>
                  API reference
                </a>{' '}
                <span className={home.note}>every endpoint, including sign-in for TVs and phones</span>
              </li>
              <li>
                <a href={`${apiURL}/openapi.yaml`} className={home.inline}>
                  OpenAPI spec
                </a>{' '}
                <span className={home.note}>MIT licensed, so you can generate a client from it</span>
              </li>
            </ul>
          </section>
        </div>
      </div>
    </Plain>
  )
}

function Register({ onRegistered }: { onRegistered: (name: string, secret: string) => void }) {
  const queries = useQueryClient()
  const [name, setName] = useState('')
  const [tvOnly, setTVOnly] = useState(false)
  const [redirects, setRedirects] = useState('')

  const register = useMutation({
    mutationFn: async () => {
      const { data, error } = await api.POST('/v1/apps', {
        body: { name, redirect_uris: tvOnly ? [] : splitURIs(redirects) },
      })
      if (error) throw error
      return data
    },
    onSuccess: (data) => {
      onRegistered(data.app.name, data.client_secret)
      setName('')
      setRedirects('')
      queries.invalidateQueries({ queryKey: ['apps'] })
    },
  })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    register.mutate()
  }

  return (
    <section>
      <h2 className={home.sectionTitle}>Register an app</h2>
      <form className={styles.form} onSubmit={submit}>
        <Field label="Name people will see" value={name} onChange={setName} placeholder="My Player" />
        <label className={styles.check}>
          <input type="checkbox" checked={tvOnly} onChange={() => setTVOnly(!tvOnly)} />
          <span>
            Sign in with TV codes only
            <span className={home.note}> For apps with no browser, like a TV or a server plugin.</span>
          </span>
        </label>
        {!tvOnly && (
          <Field
            label="Sign-in returns to"
            value={redirects}
            onChange={setRedirects}
            placeholder="myplayer://tethr/callback"
            spellCheck={false}
            autoCapitalize="none"
            hint="Separate several with spaces. Custom schemes and localhost are fine."
          />
        )}
        <div className={home.rowActions}>
          <Button
            variant="primary"
            aria-busy={register.isPending}
            disabled={!name.trim() || (!tvOnly && !redirects.trim())}
          >
            Register
          </Button>
        </div>
        {register.isError && <p className={home.error}>{problemMessage(register.error)}</p>}
      </form>
    </section>
  )
}

function AppRow({ app: a, onSecret }: { app: DeveloperApp; onSecret: (secret: string) => void }) {
  const queries = useQueryClient()
  const [editing, setEditing] = useState(false)
  const [revoking, setRevoking] = useState(false)
  const [name, setName] = useState(a.name)
  const [redirects, setRedirects] = useState(a.redirect_uris.join(' '))
  const refresh = () => queries.invalidateQueries({ queryKey: ['apps'] })

  const save = useMutation({
    mutationFn: async () => {
      const { error } = await api.PATCH('/v1/apps/{id}', {
        params: { path: { id: a.id } },
        body: { name, redirect_uris: splitURIs(redirects) },
      })
      if (error) throw error
    },
    onSuccess: () => {
      setEditing(false)
      refresh()
    },
  })
  const secret = useMutation({
    mutationFn: async () => {
      const { data, error } = await api.POST('/v1/apps/{id}/secret', { params: { path: { id: a.id } } })
      if (error) throw error
      return data
    },
    onSuccess: (data) => onSecret(data.client_secret),
  })
  const revoke = useMutation({
    mutationFn: async () => {
      const { error } = await api.DELETE('/v1/apps/{id}', { params: { path: { id: a.id } } })
      if (error) throw error
    },
    onSuccess: refresh,
  })

  if (editing) {
    return (
      <li className={home.editing}>
        <Field label="Name" value={name} onChange={setName} />
        <Field
          label="Sign-in returns to"
          value={redirects}
          onChange={setRedirects}
          spellCheck={false}
          autoCapitalize="none"
          hint="Leave empty for TV codes only."
        />
        <div className={home.rowActions}>
          <Button variant="primary" onClick={() => save.mutate()} aria-busy={save.isPending}>
            Save
          </Button>
          <Button variant="text" onClick={() => setEditing(false)}>
            Cancel
          </Button>
        </div>
        {save.isError && <p className={home.error}>{problemMessage(save.error)}</p>}
      </li>
    )
  }

  return (
    <li className={styles.app}>
      <div className={styles.appMain}>
        <span className={home.name}>{a.name}</span>
        <span className={home.kind}>
          Registered {registered(a.created_at)}, {a.connections === 1 ? '1 person' : `${a.connections} people`}{' '}
          connected
        </span>
      </div>
      <button
        type="button"
        className={styles.clientID}
        title="Copy the client ID"
        onClick={() => navigator.clipboard?.writeText(a.client_id).catch(() => {})}
      >
        {a.client_id}
      </button>
      <span className={styles.redirects}>
        {a.redirect_uris.length ? (
          a.redirect_uris.map((r) => (
            <code key={r} className={styles.code}>
              {r}
            </code>
          ))
        ) : (
          <span className={home.kind}>TV codes only</span>
        )}
      </span>
      <span className={home.rowActions}>
        {revoking ? (
          <>
            <span className={home.warn}>
              {a.connections ? `Signs out ${a.connections} connected.` : 'Nobody is using it yet.'}
            </span>
            <Button variant="text" onClick={() => setRevoking(false)}>
              Keep it
            </Button>
            <Button variant="text" className={home.danger} onClick={() => revoke.mutate()}>
              Revoke
            </Button>
          </>
        ) : (
          <>
            <Button variant="text" onClick={() => setEditing(true)}>
              Edit
            </Button>
            <Button variant="text" aria-busy={secret.isPending} onClick={() => secret.mutate()}>
              New secret
            </Button>
            <Button variant="text" className={home.danger} onClick={() => setRevoking(true)}>
              Revoke
            </Button>
          </>
        )}
      </span>
    </li>
  )
}
