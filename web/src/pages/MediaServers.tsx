import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useState } from 'react'
import { api, type Me, problemMessage } from '../api/client'
import type { components } from '../api/schema'
import { Button } from '../components/Button'
import { Plain } from '../components/Plain'
import { ago } from '../format'
import { useSignedIn } from './connect'
import styles from './Developers.module.css'
import home from './Home.module.css'

type MediaServer = components['schemas']['MediaServer']

const apiURL = import.meta.env.VITE_API_URL ?? 'http://localhost:8080'

export function MediaServers() {
  const me = useSignedIn()
  const queries = useQueryClient()
  const [address, setAddress] = useState<string | null>(null)
  const servers = useQuery({
    queryKey: ['media-servers'],
    enabled: !!me,
    queryFn: async () => (await api.GET('/v1/media-servers')).data ?? [],
  })
  const add = useMutation({
    mutationFn: async () => {
      const { data, error } = await api.POST('/v1/media-servers', { body: { kind: 'jellyfin' } })
      if (error) throw error
      return data
    },
    onSuccess: (data) => {
      setAddress(apiURL + data.path)
      queries.invalidateQueries({ queryKey: ['media-servers'] })
    },
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
          <h1 className={home.title}>Media servers</h1>
          <p className={home.lead}>
            Log what everyone watches on your Jellyfin server, whichever app they use. Match each person on the server
            to a profile below. Plays from people you haven’t matched aren’t logged.
          </p>
        </header>

        {address && <Setup address={address} onDone={() => setAddress(null)} />}

        <section>
          <h2 className={home.sectionTitle}>Your servers</h2>
          {servers.data?.length ? (
            <ul className={home.rows}>
              {servers.data.map((s) => (
                <ServerRow key={s.id} server={s} profiles={me.profiles} onAddress={setAddress} />
              ))}
            </ul>
          ) : (
            <p className={home.note}>None yet.</p>
          )}
          <div className={home.rowActions}>
            <Button variant="primary" aria-busy={add.isPending} onClick={() => add.mutate()}>
              Add Jellyfin
            </Button>
          </div>
          {add.isError && <p className={home.error}>{problemMessage(add.error)}</p>}
        </section>
      </div>
    </Plain>
  )
}

function Setup({ address, onDone }: { address: string; onDone: () => void }) {
  return (
    <div className={styles.secret} role="alert">
      <p>
        Here’s the address for your server. Copy it now. It won’t be shown again, and anyone with it can log plays to
        the people you match.
      </p>
      <div className={styles.secretRow}>
        <code className={styles.secretValue}>{address}</code>
        <Button variant="primary" onClick={() => navigator.clipboard?.writeText(address).catch(() => {})}>
          Copy
        </Button>
        <Button variant="text" onClick={onDone}>
          I’ve saved it
        </Button>
      </div>
      <ol className={home.note}>
        <li>In Jellyfin, open Dashboard, then Plugins, then Catalogue. Install Webhook and restart Jellyfin.</li>
        <li>Open the Webhook plugin’s settings and choose Add Generic Destination.</li>
        <li>Paste the address into the webhook URL and tick Send All Properties.</li>
        <li>
          Under Notification Type, tick Playback Start, Playback Stop and User Data Saved. Under Item Type, tick Movies
          and Episodes. Save.
        </li>
        <li>Play something. Each person shows up here after their first play, ready to match to a profile.</li>
      </ol>
    </div>
  )
}

function ServerRow({
  server: s,
  profiles,
  onAddress,
}: {
  server: MediaServer
  profiles: Me['profiles']
  onAddress: (address: string) => void
}) {
  const queries = useQueryClient()
  const [removing, setRemoving] = useState(false)
  const refresh = () => {
    queries.invalidateQueries({ queryKey: ['media-servers'] })
    queries.invalidateQueries({ queryKey: ['connections'] })
  }
  const match = useMutation({
    mutationFn: async ({ user, profile }: { user: number; profile: number | null }) => {
      const { error } = await api.PUT('/v1/media-servers/{id}/users/{user}', {
        params: { path: { id: s.id, user } },
        body: { profile_id: profile },
      })
      if (error) throw error
    },
    onSuccess: refresh,
  })
  const address = useMutation({
    mutationFn: async () => {
      const { data, error } = await api.POST('/v1/media-servers/{id}/address', { params: { path: { id: s.id } } })
      if (error) throw error
      return data
    },
    onSuccess: (data) => onAddress(apiURL + data.path),
  })
  const remove = useMutation({
    mutationFn: async () => {
      const { error } = await api.DELETE('/v1/media-servers/{id}', { params: { path: { id: s.id } } })
      if (error) throw error
    },
    onSuccess: refresh,
  })

  return (
    <li className={styles.app}>
      <div className={styles.appMain}>
        <span className={home.name}>{s.name ?? 'Jellyfin'}</span>
        <span className={home.kind}>
          {s.last_event_at ? `Jellyfin, last heard from ${ago(s.last_event_at)}` : 'Waiting for its first play'}
        </span>
      </div>
      <span>
        {s.users.map((u) => (
          <label key={u.id} className={home.kind}>
            {u.name}{' '}
            <select
              value={u.profile_id ?? ''}
              disabled={match.isPending}
              onChange={(e) => match.mutate({ user: u.id, profile: e.target.value ? Number(e.target.value) : null })}
            >
              <option value="">Don’t log</option>
              {profiles.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
          </label>
        ))}
        {match.isError && <span className={home.error}>{problemMessage(match.error)}</span>}
      </span>
      <span className={home.rowActions}>
        {removing ? (
          <>
            <span className={home.warn}>Stops logging plays from this server.</span>
            <Button variant="text" onClick={() => setRemoving(false)}>
              Keep it
            </Button>
            <Button variant="text" className={home.danger} onClick={() => remove.mutate()}>
              Remove
            </Button>
          </>
        ) : (
          <>
            <Button variant="text" aria-busy={address.isPending} onClick={() => address.mutate()}>
              New address
            </Button>
            <Button variant="text" className={home.danger} onClick={() => setRemoving(true)}>
              Remove
            </Button>
          </>
        )}
      </span>
    </li>
  )
}
