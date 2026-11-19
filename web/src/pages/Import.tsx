import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { type ChangeEvent, useRef, useState } from 'react'
import { api, problemMessage } from '../api/client'
import type { components } from '../api/schema'
import { Button } from '../components/Button'
import { Plain } from '../components/Plain'
import { Slip, SlipBody, SlipFoot } from '../components/Slip'
import { plural } from '../format'
import { type Film, type FilmList, isLetterboxd, readLetterboxd, unzip } from '../letterboxd'
import { ProfilePicker, useSignedIn } from './connect'
import home from './Home.module.css'
import styles from './Import.module.css'
import listStyles from './Lists.module.css'
import { MatchFilms } from './MatchFilms'

type Import = components['schemas']['Import']
type ImportList = components['schemas']['ImportListSummary']

const parts = [
  ['history', 'Plays', 'play', 'plays'],
  ['watchlist', 'Watchlist', 'watchlist entry', 'watchlist entries'],
  ['playback', 'Resume points', 'resume point', 'resume points'],
  ['ratings', 'Ratings', 'rating', 'ratings'],
  ['hidden', 'Hidden shows', 'hidden show', 'hidden shows'],
  ['list_entries', 'List entries', 'list entry', 'list entries'],
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
            Bring your plays, watchlist, resume points, ratings, hidden shows and Letterboxd lists across from your old
            tracker. Download its data export from your settings there, then add the zip here as it is. If it offers
            more than one kind, choose the full backup, not a simple CSV. Some trackers only give you one backup a
            month, so download it when you’re ready to move. Nothing is saved until you’ve checked what was found.
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

  const [letterboxd, setLetterboxd] = useState<{
    films: Film[]
    lists: FilmList[]
    unreadable: string[]
    filename: string
  } | null>(null)
  const read = useMutation({
    mutationFn: async (file: File) => {
      const files = await unzip(file).catch((e) => {
        if (e instanceof Error && e.message === 'not a zip') return null
        throw e
      })
      if (files && isLetterboxd(files)) setLetterboxd({ ...readLetterboxd(files), filename: file.name })
      else upload.mutate(file)
    },
  })

  const choose = (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    e.target.value = ''
    upload.reset()
    read.reset()
    setLetterboxd(null)
    if (file) read.mutate(file)
  }

  const waiting = imports.data?.find((i) => i.status === 'parsed')
  const running = imports.data?.some((i) => i.status === 'queued')
  const past = imports.data?.filter((i) => i.status !== 'parsed') ?? []

  return (
    <>
      <section>
        <h2 className={home.sectionTitle}>
          {waiting ? 'Check what was found' : letterboxd ? 'Match your films' : 'Add your export'}
        </h2>
        <input ref={input} type="file" accept=".zip,application/zip" hidden onChange={choose} />
        {waiting ? (
          <Review imp={waiting} profileID={profileID} onAnother={() => input.current?.click()} onDone={refresh} />
        ) : letterboxd ? (
          <MatchFilms
            films={letterboxd.films}
            lists={letterboxd.lists}
            unreadable={letterboxd.unreadable}
            filename={letterboxd.filename}
            profileID={profileID}
            onDone={() => {
              setLetterboxd(null)
              refresh()
            }}
            onAnother={() => input.current?.click()}
          />
        ) : (
          <div className={styles.pick}>
            <Button
              variant="primary"
              onClick={() => input.current?.click()}
              aria-busy={upload.isPending || read.isPending}
              disabled={upload.isPending || read.isPending || running}
            >
              {upload.isPending || read.isPending ? 'Reading the export…' : 'Choose the zip'}
            </Button>
            <span className={home.note}>
              {running ? 'An import is running. You can add another when it’s done.' : 'Up to 100 MB.'}
            </span>
          </div>
        )}
        {upload.isError && <p className={home.error}>{problemMessage(upload.error)}</p>}
        {read.isError && (
          <p className={home.error}>This browser couldn’t open the export. Try again in an up-to-date browser.</p>
        )}
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
  const lists = imp.summary.lists
  const [chosen, setChosen] = useState(() => {
    let room = imp.summary.list_room
    const picked: number[] = []
    lists.forEach((_, i) => {
      if (!needsRoom(lists, picked, i) || room-- > 0) picked.push(i)
    })
    return picked
  })
  const apply = useMutation({
    mutationFn: async () => {
      const { error } = await api.POST('/v1/imports/{id}/apply', {
        params: { path: { id: imp.id }, query: { profile_id: profileID } },
        body: lists.length ? { lists: chosen } : undefined,
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
      {unmatched > 0 && (
        <Unmatched names={imp.summary.unmatched} total={unmatched} leftOut={imp.source === 'letterboxd'} />
      )}
      {lists.length > 0 && (
        <ListPicker lists={lists} room={imp.summary.list_room} chosen={chosen} onChange={setChosen} />
      )}
      <p className={home.note}>
        Plays already in your history are skipped, so importing the same file twice is safe.
        {lists.length > 0 && ' Films already on the list they’re going into are skipped too.'} You can undo the whole
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

const sameName = (a: ImportList, b: ImportList) => a.name.toLowerCase() === b.name.toLowerCase()

const joinsEarlier = (lists: ImportList[], chosen: number[], i: number) =>
  lists.some((o, j) => j < i && chosen.includes(j) && o.unmatched < o.total && sameName(o, lists[i]))

const needsRoom = (lists: ImportList[], chosen: number[], i: number) =>
  !lists[i].existing && lists[i].unmatched < lists[i].total && !joinsEarlier(lists, chosen, i)

function listNote(l: ImportList, earlier: boolean) {
  const matched = l.total - l.unmatched
  let note = plural(l.total, 'film')
  if (l.unmatched) note += `, ${l.unmatched.toLocaleString('en-GB')} left out`
  note += '.'
  if (earlier) note += ' Goes into the list above with the same name.'
  else if (l.existing) note += ' Adds to your list with this name.'
  else if (!matched) note += ' Nothing to bring across.'
  if (matched > 1000) note += ' A list holds up to 1,000 films.'
  return note
}

function ListPicker({
  lists,
  room,
  chosen,
  onChange,
}: {
  lists: ImportList[]
  room: number
  chosen: number[]
  onChange: (chosen: number[]) => void
}) {
  const taken = chosen.filter((i) => needsRoom(lists, chosen, i)).length
  const full = taken >= room && lists.some((_, i) => !chosen.includes(i) && needsRoom(lists, chosen, i))
  const toggle = (i: number) =>
    onChange(chosen.includes(i) ? chosen.filter((c) => c !== i) : [...chosen, i].sort((a, b) => a - b))

  return (
    <fieldset className={styles.lists}>
      <legend>Lists</legend>
      <ul>
        {lists.map((l, i) => {
          const on = chosen.includes(i)
          return (
            // biome-ignore lint/suspicious/noArrayIndexKey: lists are chosen by their place in the import
            <li key={i}>
              <label className={styles.listRow}>
                <input
                  type="checkbox"
                  checked={on}
                  disabled={!on && needsRoom(lists, chosen, i) && taken >= room}
                  onChange={() => toggle(i)}
                />
                <span className={on ? `${listStyles.tickbox} ${listStyles.on}` : listStyles.tickbox}>
                  <svg viewBox="0 0 20 20" width="20" height="20" aria-hidden="true">
                    <path
                      d="M4.5 10.5l3.6 3.6L15.5 6"
                      fill="none"
                      stroke="currentColor"
                      strokeWidth="2.4"
                      strokeLinecap="round"
                    />
                  </svg>
                </span>
                <span>
                  <span className={styles.listName}>{l.name}</span>
                  <span className={styles.listNote}>{listNote(l, joinsEarlier(lists, chosen, i))}</span>
                </span>
              </label>
            </li>
          )
        })}
      </ul>
      <p className={home.note}>
        {!full
          ? 'New lists come across as private, with their films in the same order.'
          : room > 0
            ? `A profile holds up to 100 lists, so there’s room for ${plural(room, 'more list', 'more lists')}. Untick one to choose another.`
            : 'This profile already has 100 lists. Delete some to bring new ones across.'}
      </p>
    </fieldset>
  )
}

function Unmatched({ names, total, leftOut }: { names: string[]; total: number; leftOut: boolean }) {
  return (
    <details className={styles.unmatched}>
      <summary>
        {leftOut
          ? `${count(total, 'item', 'items')} from films you left out won’t come across`
          : `${
              total === 1
                ? '1 item has no TMDB, IMDb or TVDB ID, so it'
                : `${total.toLocaleString('en-GB')} items have no TMDB, IMDb or TVDB ID, so they`
            } won’t come across`}
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
  const made = (s.lists ?? []).filter((l) => l.created).length
  const done = parts.filter(([key]) => s[key].added > 0).map(([key, , one, many]) => count(s[key].added, one, many))
  if (made) done.push(count(made, 'new list', 'new lists'))
  const skipped = parts.reduce((n, [key]) => n + s[key].skipped, 0)
  const full = (s.lists ?? []).reduce((n, l) => n + l.full, 0)

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
      status = `${day(imp.applied_at)}. ${done.length ? `Added ${done.join(', ')}.` : full ? 'Nothing was added.' : 'Everything was already here.'}`
      if (done.length && skipped) status += ` ${count(skipped, 'item was', 'items were')} already here.`
      if (full)
        status += ` ${count(full, 'film', 'films')} didn’t fit, as a list holds up to 1,000 films and a profile up to 100 lists.`
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
            {made > 0 && ' Lists it made are deleted, unless you’ve added something to them since.'}
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
