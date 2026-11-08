import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { type ChangeEvent, useRef, useState } from 'react'
import { api, problemMessage } from '../api/client'
import type { components } from '../api/schema'
import { Button } from '../components/Button'
import { Plain } from '../components/Plain'
import { Slip, SlipBody, SlipFoot } from '../components/Slip'
import { ProfilePicker, useSignedIn } from './connect'
import home from './Home.module.css'
import styles from './Import.module.css'

type Import = components['schemas']['Import']

const parts = [
  ['history', 'Plays', 'play', 'plays'],
  ['watchlist', 'Watchlist', 'watchlist entry', 'watchlist entries'],
  ['playback', 'Resume points', 'resume point', 'resume points'],
  ['ratings', 'Ratings', 'rating', 'ratings'],
  ['hidden', 'Hidden shows', 'hidden show', 'hidden shows'],
] as const

const count = (n: number, one: string, many: string) => `${n.toLocaleString('en-GB')} ${n === 1 ? one : many}`

const day = (iso?: string) =>
  iso ? new Date(iso).toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' }) : ''
const undoable = (i: Import) =>
  i.status === 'applied' && !!i.applied_at && Date.now() - Date.parse(i.applied_at) < 30 * 24 * 60 * 60 * 1000

export function Import() {
  const me = useSignedIn()
  const [profile, setProfile] = useState<number | null>(null)
  const profileID = profile ?? me?.profiles[0]?.id ?? 0

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
          <h1 className={home.title}>Import your history</h1>
          <p className={home.lead}>
            Bring your plays, watchlist, resume points, ratings and hidden shows across from your old tracker. Download
            its data export from your settings there, then add the zip here as it is. If it offers more than one kind,
            choose the full backup, not a simple CSV. Some trackers only give you one backup a month, so download it
            when you’re ready to move. Nothing is saved until you’ve checked what was found.
          </p>
        </header>
        <ProfilePicker legend="Import into" profiles={me.profiles} value={profileID} onChange={setProfile} />
        {profileID > 0 && <Imports key={profileID} profileID={profileID} />}
      </div>
    </Plain>
  )
}

function Imports({ profileID }: { profileID: number }) {
  const queries = useQueryClient()
  const input = useRef<HTMLInputElement>(null)
  const params = { query: { profile_id: profileID } }
  const imports = useQuery({
    queryKey: ['imports', profileID],
    queryFn: async () => (await api.GET('/v1/imports', { params })).data ?? [],
    refetchInterval: (q) => (q.state.data?.some((i) => i.status === 'queued') ? 1000 : false),
  })
  const refresh = () => queries.invalidateQueries({ queryKey: ['imports', profileID] })

  const upload = useMutation({
    mutationFn: async (file: File) => {
      const { error } = await api.POST('/v1/imports', {
        params: { query: { profile_id: profileID, filename: file.name } },
        body: file as unknown as string,
        bodySerializer: (body) => body,
        headers: { 'Content-Type': 'application/zip' },
      })
      if (error) throw error
    },
    onSuccess: refresh,
  })

  const choose = (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    e.target.value = ''
    if (file) upload.mutate(file)
  }

  const waiting = imports.data?.find((i) => i.status === 'parsed')
  const running = imports.data?.some((i) => i.status === 'queued')
  const past = imports.data?.filter((i) => i.status !== 'parsed') ?? []

  return (
    <>
      <section>
        <h2 className={home.sectionTitle}>{waiting ? 'Check what was found' : 'Add your export'}</h2>
        <input ref={input} type="file" accept=".zip,application/zip" hidden onChange={choose} />
        {waiting ? (
          <Review imp={waiting} profileID={profileID} onAnother={() => input.current?.click()} onDone={refresh} />
        ) : (
          <div className={styles.pick}>
            <Button
              variant="primary"
              onClick={() => input.current?.click()}
              aria-busy={upload.isPending}
              disabled={upload.isPending || running}
            >
              {upload.isPending ? 'Reading the export…' : 'Choose the zip'}
            </Button>
            <span className={home.note}>
              {running ? 'An import is running. You can add another when it’s done.' : 'Up to 100 MB.'}
            </span>
          </div>
        )}
        {upload.isError && <p className={home.error}>{problemMessage(upload.error)}</p>}
      </section>

      {past.length > 0 && (
        <section>
          <h2 className={home.sectionTitle}>Imports</h2>
          <ul className={home.rows}>
            {past.map((i) => (
              <PastImport key={i.id} imp={i} profileID={profileID} onDone={refresh} />
            ))}
          </ul>
        </section>
      )}
    </>
  )
}

function Review({
  imp,
  profileID,
  onAnother,
  onDone,
}: {
  imp: Import
  profileID: number
  onAnother: () => void
  onDone: () => void
}) {
  const apply = useMutation({
    mutationFn: async () => {
      const { error } = await api.POST('/v1/imports/{id}/apply', {
        params: { path: { id: imp.id }, query: { profile_id: profileID } },
      })
      if (error) throw error
    },
    onSuccess: onDone,
  })
  const found = parts.filter(([key]) => imp.summary[key].total > 0)
  const unmatched = parts.reduce((n, [key]) => n + imp.summary[key].unmatched, 0)

  return (
    <div className={styles.review}>
      <table className={styles.counts}>
        <thead>
          <tr>
            <th scope="col">
              <span className={home.visuallyHidden}>Type</span>
            </th>
            <th scope="col">In the file</th>
            <th scope="col">Can’t match</th>
          </tr>
        </thead>
        <tbody>
          {found.map(([key, label]) => (
            <tr key={key}>
              <th scope="row">{label}</th>
              <td>{imp.summary[key].total.toLocaleString('en-GB')}</td>
              <td>{imp.summary[key].unmatched ? imp.summary[key].unmatched.toLocaleString('en-GB') : '–'}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {unmatched > 0 && <Unmatched names={imp.summary.unmatched} total={unmatched} />}
      <p className={home.note}>
        Plays already in your history are skipped, so importing the same file twice is safe. You can undo the whole
        import for 30 days.
      </p>
      <div className={styles.actions}>
        <Button variant="primary" onClick={() => apply.mutate()} aria-busy={apply.isPending}>
          Import
        </Button>
        <Button variant="text" onClick={onAnother}>
          Choose another file
        </Button>
      </div>
      {apply.isError && <p className={home.error}>{problemMessage(apply.error)}</p>}
    </div>
  )
}

function Unmatched({ names, total }: { names: string[]; total: number }) {
  return (
    <details className={styles.unmatched}>
      <summary>
        {total === 1
          ? '1 item has no TMDB, IMDb or TVDB ID, so it'
          : `${total.toLocaleString('en-GB')} items have no TMDB, IMDb or TVDB ID, so they`}{' '}
        won’t come across
      </summary>
      <ul>
        {names.map((name, i) => (
          // biome-ignore lint/suspicious/noArrayIndexKey: names repeat when the same film was watched twice
          <li key={i}>{name}</li>
        ))}
      </ul>
      {total > names.length && <p className={home.note}>And {total - names.length} more.</p>}
    </details>
  )
}

function PastImport({ imp, profileID, onDone }: { imp: Import; profileID: number; onDone: () => void }) {
  const [confirming, setConfirming] = useState(false)
  const undo = useMutation({
    mutationFn: async () => {
      const { error } = await api.POST('/v1/imports/{id}/undo', {
        params: { path: { id: imp.id }, query: { profile_id: profileID } },
      })
      if (error) throw error
    },
    onSuccess: () => {
      setConfirming(false)
      onDone()
    },
  })
  const s = imp.summary
  const done = parts.filter(([key]) => s[key].added > 0).map(([key, , one, many]) => count(s[key].added, one, many))
  const skipped = parts.reduce((n, [key]) => n + s[key].skipped, 0)

  let status: string
  switch (imp.status) {
    case 'queued':
      status = 'Importing…'
      break
    case 'failed':
      status = 'Didn’t finish, so nothing was changed. Try adding the file again.'
      break
    case 'undone':
      status = `Imported ${day(imp.applied_at)}, undone ${day(imp.undone_at)}.`
      break
    default:
      status = `${day(imp.applied_at)}. ${done.length ? `Added ${done.join(', ')}.` : 'Everything was already here.'}`
      if (done.length && skipped) status += ` ${count(skipped, 'item was', 'items were')} already here.`
  }

  return (
    <li className={styles.past}>
      <div className={styles.pastMain}>
        <span className={home.name}>{imp.filename || 'Export file'}</span>
        <span className={home.kind}>{status}</span>
      </div>
      {undoable(imp) && done.length > 0 && (
        <span className={home.rowActions}>
          <Button
            variant="text"
            onClick={() => {
              undo.reset()
              setConfirming(true)
            }}
          >
            Undo
          </Button>
        </span>
      )}
      <Slip open={confirming} onClose={() => setConfirming(false)} title="Undo this import?">
        <SlipBody>
          <p>
            Everything it added comes out: {done.join(', ')}. Anything that was already here before the import stays.
          </p>
          {undo.isError && <p className={home.error}>{problemMessage(undo.error)}</p>}
        </SlipBody>
        <SlipFoot>
          <Button variant="danger" onClick={() => undo.mutate()} aria-busy={undo.isPending}>
            Undo the import
          </Button>
          <Button variant="text" onClick={() => setConfirming(false)}>
            Keep it
          </Button>
        </SlipFoot>
      </Slip>
    </li>
  )
}
