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
import own from './MediaServers.module.css'

type MediaServer = components['schemas']['MediaServer']
type Kind = MediaServer['kind']

const names: Record<Kind, string> = { jellyfin: 'Jellyfin', plex: 'Plex', emby: 'Emby' }

const apiURL = import.meta.env.VITE_API_URL ?? 'http://localhost:8080'

const pluginRepository = 'https://tethrtv.github.io/jellyfin-plugin/manifest.json'

export function MediaServers() {
  const me = useSignedIn()
  const queries = useQueryClient()
  const [address, setAddress] = useState<{ kind: Kind; url: string } | null>(null)
  const servers = useQuery({
    queryKey: ['media-servers'],
    enabled: !!me,
    queryFn: async () => (await api.GET('/v1/media-servers')).data ?? [],
  })
  const add = useMutation({
    mutationFn: async (kind: Kind) => {
      const { data, error } = await api.POST('/v1/media-servers', { body: { kind } })
      if (error) throw error
      return data
    },
    onSuccess: (data) => {
      setAddress({ kind: data.server.kind, url: apiURL + data.path })
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
            Log what everyone watches on your Jellyfin, Plex or Emby server, whichever app they use. Match each person
            on the server to a profile below. Anything they finish before you match them waits here for a week, then
            logs once you do.
          </p>
        </header>

        {address && <Setup kind={address.kind} address={address.url} onDone={() => setAddress(null)} />}

        <section>
          <h2 className={home.sectionTitle}>Tethr for Jellyfin</h2>
          <p className={home.note}>
            On Jellyfin, our plugin does more than a webhook. It brings in what each person has already watched, and
            marks things they watch elsewhere as played. In Jellyfin’s dashboard, open Plugins, then Repositories, and
            add{' '}
            <button
              type="button"
              className={styles.clientID}
              title="Copy the address"
              onClick={() => navigator.clipboard?.writeText(pluginRepository).catch(() => {})}
            >
              {pluginRepository}
            </button>
            . Then install Tethr from the Catalogue, restart Jellyfin, and connect each person in the plugin’s settings.
          </p>
        </section>

        <section>
          <h2 className={home.sectionTitle}>Your servers</h2>
          {servers.data?.length ? (
            <ul className={home.rows}>
              {servers.data.map((s) => (
                <ServerRow
                  key={s.id}
                  server={s}
                  profiles={me.profiles}
                  onAddress={(url) => setAddress({ kind: s.kind, url })}
                />
              ))}
            </ul>
          ) : (
            <p className={home.note}>None yet.</p>
          )}
          <div className={home.rowActions}>
            {(Object.keys(names) as Kind[]).map((kind) => (
              <Button
                key={kind}
                variant="plain"
                aria-busy={add.isPending && add.variables === kind}
                onClick={() => add.mutate(kind)}
              >
                Add {names[kind]}
              </Button>
            ))}
          </div>
          {add.isError && <p className={home.error}>{problemMessage(add.error)}</p>}
        </section>
      </div>
    </Plain>
  )
}

const steps: Record<Kind, string[]> = {
  jellyfin: [
    'In Jellyfin, open Dashboard, then Plugins, then Catalogue. Install Webhook and restart Jellyfin.',
    'Open the Webhook plugin’s settings and choose Add Generic Destination.',
    'Paste the address into the webhook URL and tick Send All Properties.',
    'Under Notification Type, tick Playback Start, Playback Stop and User Data Saved. Under Item Type, tick Movies and Episodes. Save.',
  ],
  plex: [
    'Webhooks need Plex Pass on the account that owns the server.',
    'In Plex, open your account settings and choose Webhooks.',
    'Choose Add Webhook, paste the address and save.',
  ],
  emby: [
    'Webhooks need Emby Premiere.',
    'In Emby’s dashboard, open Notifications, add a notification and choose Webhooks.',
    'Paste the address as the URL, and choose the playback events and Mark played. Save.',
  ],
}

function Setup({ kind, address, onDone }: { kind: Kind; address: string; onDone: () => void }) {
  return (
    <div className={styles.secret} role="alert">
      <p>
        Here’s the address for your {names[kind]} server. Copy it now. It won’t be shown again, and anyone with it can
        log plays to the people you match.
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
        {steps[kind].map((step) => (
          <li key={step}>{step}</li>
        ))}
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
    <li className={own.server}>
      <div className={styles.appMain}>
        <span className={home.name}>{s.name ?? names[s.kind]}</span>
        <span className={home.kind}>
          {s.last_event_at ? `${names[s.kind]}, last heard from ${ago(s.last_event_at)}` : 'Waiting for its first play'}
        </span>
      </div>
      <span className={own.users}>
        {s.users.map((u) => (
          <label key={u.id} className={own.user}>
            {u.name}{' '}
            <select
              value={u.profile_id ?? (u.ignored ? 'none' : '')}
              disabled={match.isPending}
              onChange={(e) =>
                match.mutate({ user: u.id, profile: e.target.value === 'none' ? null : Number(e.target.value) })
              }
            >
              {!u.profile_id && !u.ignored && (
                <option value="" disabled>
                  Choose a profile
                </option>
              )}
              {profiles.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
              <option value="none">Don’t log</option>
            </select>
            {u.held_plays > 0 && (
              <span className={own.waiting}>
                {u.held_plays === 1 ? '1 play waiting' : `${u.held_plays} plays waiting`}
              </span>
            )}
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
