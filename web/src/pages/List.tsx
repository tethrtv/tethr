import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { type PointerEvent, type ReactNode, useRef, useState } from 'react'
import { api, problemMessage } from '../api/client'
import type { components } from '../api/schema'
import { Button } from '../components/Button'
import { Cover } from '../components/Cover'
import { Field } from '../components/Field'
import { TitleLink } from '../components/TitleLink'
import { ep, plural } from '../format'
import { useDetails } from '../tmdb'
import own from './List.module.css'
import styles from './Lists.module.css'
import { privacyWords } from './MyLists'

type List = components['schemas']['List']
type Item = components['schemas']['ListItem']
type Privacy = List['privacy']

const orders = [
  { label: 'List order', by: 'rank', how: 'asc' },
  { label: 'Recently added', by: 'added', how: 'desc' },
  { label: 'Title', by: 'title', how: 'asc' },
  { label: 'Newest', by: 'year', how: 'desc' },
  { label: 'Oldest', by: 'year', how: 'asc' },
] as const
type Order = (typeof orders)[number]

const kinds = [
  { value: 'all', label: 'Everything' },
  { value: 'movie', label: 'Films' },
  { value: 'show', label: 'Shows' },
] as const

function storedView(): 'rows' | 'covers' {
  try {
    return localStorage.getItem('tethr.listView') === 'covers' ? 'covers' : 'rows'
  } catch {
    return 'rows'
  }
}

export function ListPage({ param, profileID }: { param: string; profileID?: number }) {
  const id = Number.parseInt(param, 10) || 0
  const queries = useQueryClient()
  const [kind, setKind] = useState<(typeof kinds)[number]['value']>('all')
  const [hideWatched, setHideWatched] = useState(false)
  const [order, setOrder] = useState<Order | undefined>()
  const [find, setFind] = useState('')
  const [view, setView] = useState(storedView)
  const query = profileID ? { profile_id: profileID } : {}

  const page = useQuery({
    queryKey: ['list', id, profileID ?? 0, kind, hideWatched, order?.label],
    placeholderData: keepPreviousData,
    queryFn: async () => {
      const { data, response } = await api.GET('/v1/lists/{id}/items', {
        params: {
          path: { id },
          query: {
            ...query,
            limit: 1000,
            type: kind === 'all' ? undefined : kind,
            hide_watched: hideWatched || undefined,
            sort_by: order?.by,
            sort_how: order?.how,
          },
        },
      })
      if (response.status === 404) return null
      if (!data) throw new Error('could not load the list')
      return data
    },
  })
  const refresh = () => {
    queries.invalidateQueries({ queryKey: ['list', id] })
    queries.invalidateQueries({ queryKey: ['lists'] })
    queries.invalidateQueries({ queryKey: ['title'] })
  }

  if (page.data === null) {
    return (
      <div className={styles.page}>
        <header className={styles.head}>
          <h1 className={styles.title}>No list here</h1>
          <p className={styles.lead}>
            There’s no list at this address, or it isn’t shared with you.
            {!profileID && ' If it’s yours or your household’s, sign in first.'}
          </p>
        </header>
      </div>
    )
  }
  if (!page.data) return null

  const list = page.data.list
  const mine = list.access === 'mine' && !!profileID
  const chosen = order ?? orders.find((o) => o.by === list.sort_by && o.how === list.sort_how) ?? orders[0]
  const words = find.trim().toLowerCase()
  const items = words
    ? page.data.items.filter((i) => (i.item.title.name ?? '').toLowerCase().includes(words))
    : page.data.items
  const draggable = mine && chosen.by === 'rank' && chosen.how === 'asc' && kind === 'all' && !hideWatched && !words

  return (
    <div className={styles.page}>
      <Head list={list} mine={mine} profileID={profileID} onChange={refresh} />

      <div className={`${styles.filters} ${own.filters}`}>
        <fieldset className={styles.options}>
          <legend className={styles.hidden}>Type</legend>
          {kinds.map((k) => (
            <Option key={k.value} name="kind" checked={k.value === kind} onChange={() => setKind(k.value)}>
              {k.label}
            </Option>
          ))}
        </fieldset>
        {profileID && (
          <label className={hideWatched ? `${styles.option} ${styles.chosen}` : styles.option}>
            <input type="checkbox" checked={hideWatched} onChange={(e) => setHideWatched(e.target.checked)} />
            Hide watched
          </label>
        )}
        <label className={own.select}>
          <span className={styles.faint}>Order</span>
          <select value={chosen.label} onChange={(e) => setOrder(orders.find((o) => o.label === e.target.value))}>
            {orders.map((o) => (
              <option key={o.label}>{o.label}</option>
            ))}
          </select>
        </label>
        <fieldset className={styles.options}>
          <legend className={styles.hidden}>View</legend>
          {(['rows', 'covers'] as const).map((v) => (
            <Option
              key={v}
              name="view"
              checked={v === view}
              onChange={() => {
                setView(v)
                try {
                  localStorage.setItem('tethr.listView', v)
                } catch {}
              }}
            >
              {v === 'rows' ? 'Rows' : 'Covers'}
            </Option>
          ))}
        </fieldset>
        <input
          className={own.find}
          type="search"
          placeholder="Find in this list"
          aria-label="Find in this list"
          value={find}
          onChange={(e) => setFind(e.target.value)}
        />
      </div>

      <section>
        <h2 className={styles.sectionTitle}>{chosen.label}</h2>
        {items.length === 0 ? (
          <p className={styles.quiet}>
            {page.data.total === 0 && kind === 'all' && !hideWatched
              ? mine
                ? 'Nothing on this list yet. Add films and shows from their pages.'
                : 'Nothing on this list yet.'
              : 'Nothing here matches.'}
          </p>
        ) : view === 'covers' ? (
          <ol className={own.covers}>
            {items.map((i) => (
              <CoverItem key={i.id} item={i} />
            ))}
          </ol>
        ) : (
          <Rows
            key={items.map((i) => i.id).join()}
            list={list}
            items={items}
            mine={mine}
            draggable={draggable}
            showRank={chosen.by === 'rank'}
            profileID={profileID}
            onChange={refresh}
          />
        )}
      </section>
    </div>
  )
}

function Option({
  name,
  checked,
  onChange,
  children,
}: {
  name: string
  checked: boolean
  onChange: () => void
  children: ReactNode
}) {
  return (
    <label className={checked ? `${styles.option} ${styles.chosen}` : styles.option}>
      <input type="radio" name={name} checked={checked} onChange={onChange} />
      {children}
    </label>
  )
}

function Head({
  list,
  mine,
  profileID,
  onChange,
}: {
  list: List
  mine: boolean
  profileID?: number
  onChange: () => void
}) {
  const [editing, setEditing] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [copied, setCopied] = useState(false)
  const navigate = useNavigate()
  const remove = useMutation({
    mutationFn: async () => {
      const { error } = await api.DELETE('/v1/lists/{id}', {
        params: { path: { id: list.id }, query: { profile_id: profileID } },
      })
      if (error) throw error
    },
    onSuccess: () => {
      onChange()
      navigate({ to: '/lists' })
    },
  })
  const by = list.access === 'mine' ? '' : `${list.owner.name}’s list. `

  if (editing) {
    return (
      <Editor
        list={list}
        profileID={profileID}
        onDone={() => {
          setEditing(false)
          onChange()
        }}
      />
    )
  }
  return (
    <header className={styles.head}>
      <h1 className={styles.title}>{list.name}</h1>
      {list.description && <p className={styles.lead}>{list.description}</p>}
      <p className={styles.small}>
        {by}
        {privacyWords[list.privacy]} can see it.{' '}
        {list.item_count === 0
          ? 'Nothing on it yet.'
          : `${plural(list.item_count, 'thing')}${list.watched_count !== undefined ? `, ${list.watched_count} watched` : ''}.`}
      </p>
      <p className={own.actions}>
        {list.privacy === 'public' && (
          <button
            type="button"
            className={styles.textButton}
            onClick={() =>
              navigator.clipboard
                ?.writeText(list.url)
                .then(() => setCopied(true))
                .catch(() => {})
            }
          >
            {copied ? 'Link copied' : 'Copy the link'}
          </button>
        )}
        {mine && !deleting && (
          <>
            <button type="button" className={styles.textButton} onClick={() => setEditing(true)}>
              Change details
            </button>
            <button type="button" className={`${styles.textButton} ${own.danger}`} onClick={() => setDeleting(true)}>
              Delete the list
            </button>
          </>
        )}
        {deleting && (
          <span className={styles.small}>
            Deleting it takes everything off, and can’t be undone.{' '}
            <button type="button" className={styles.textButton} onClick={() => setDeleting(false)}>
              Keep it
            </button>{' '}
            <button
              type="button"
              className={`${styles.textButton} ${own.danger}`}
              onClick={() => remove.mutate()}
              disabled={remove.isPending}
            >
              Delete
            </button>
          </span>
        )}
        {!profileID && (
          <Link to="/sign-in" className={styles.textButton}>
            Sign in to see what you’ve watched
          </Link>
        )}
      </p>
      {remove.isError && <p className={styles.error}>{problemMessage(remove.error)}</p>}
    </header>
  )
}

function Editor({ list, profileID, onDone }: { list: List; profileID?: number; onDone: () => void }) {
  const [name, setName] = useState(list.name)
  const [description, setDescription] = useState(list.description ?? '')
  const [privacy, setPrivacy] = useState<Privacy>(list.privacy)
  const save = useMutation({
    mutationFn: async () => {
      const { error } = await api.PATCH('/v1/lists/{id}', {
        params: { path: { id: list.id }, query: { profile_id: profileID } },
        body: { name, description, privacy },
      })
      if (error) throw error
    },
    onSuccess: onDone,
  })

  return (
    <form
      className={`${styles.head} ${own.editor}`}
      onSubmit={(e) => {
        e.preventDefault()
        save.mutate()
      }}
    >
      <Field label="Name" value={name} onChange={setName} maxLength={100} required />
      <Field label="Description" value={description} onChange={setDescription} maxLength={1000} />
      <fieldset className={styles.options}>
        <legend className={own.legend}>Who can see it</legend>
        {(Object.keys(privacyWords) as Privacy[]).map((p) => (
          <Option key={p} name="privacy" checked={p === privacy} onChange={() => setPrivacy(p)}>
            {privacyWords[p]}
          </Option>
        ))}
      </fieldset>
      {privacy === 'public' && (
        <p className={styles.small}>Anyone with the link sees the list, its notes, and your name and username.</p>
      )}
      {save.isError && <p className={styles.error}>{problemMessage(save.error)}</p>}
      <p className={own.actions}>
        <Button variant="primary" type="submit" disabled={!name.trim() || save.isPending}>
          Save
        </Button>
        <button type="button" className={styles.textButton} onClick={onDone}>
          Cancel
        </button>
      </p>
    </form>
  )
}

function Rows({
  list,
  items,
  mine,
  draggable,
  showRank,
  profileID,
  onChange,
}: {
  list: List
  items: Item[]
  mine: boolean
  draggable: boolean
  showRank: boolean
  profileID?: number
  onChange: () => void
}) {
  const [order, setOrder] = useState(items)
  const [drag, setDrag] = useState<{ id: number; startY: number; dy: number } | null>(null)
  const rows = useRef(new Map<number, HTMLLIElement>())
  const reorder = useMutation({
    mutationFn: async (ids: number[]) => {
      const { error } = await api.PUT('/v1/lists/{id}/order', {
        params: { path: { id: list.id }, query: { profile_id: profileID } },
        body: { ids },
      })
      if (error) throw error
    },
    onSettled: onChange,
  })

  const move = (from: number, to: number) => {
    const next = [...order]
    const [moved] = next.splice(from, 1)
    next.splice(to, 0, moved)
    setOrder(next)
    return next
  }
  const save = (next: Item[]) => {
    if (next.some((item, i) => item.id !== items[i]?.id)) reorder.mutate(next.map((i) => i.id))
  }

  const start = (e: PointerEvent<HTMLButtonElement>, id: number) => {
    e.currentTarget.setPointerCapture(e.pointerId)
    setDrag({ id, startY: e.clientY, dy: 0 })
  }
  const track = (e: PointerEvent<HTMLButtonElement>) => {
    if (!drag) return
    let { startY } = drag
    const from = order.findIndex((i) => i.id === drag.id)
    let index = from
    for (;;) {
      const dy = e.clientY - startY
      const neighbour = dy > 0 ? order[index + 1] : order[index - 1]
      const el = neighbour && rows.current.get(neighbour.id)
      if (!el || Math.abs(dy) <= el.offsetHeight / 2) break
      startY += dy > 0 ? el.offsetHeight : -el.offsetHeight
      index += dy > 0 ? 1 : -1
    }
    if (index !== from) move(from, index)
    setDrag({ id: drag.id, startY, dy: e.clientY - startY })
  }
  const drop = () => {
    setDrag(null)
    save(order)
  }

  return (
    <ol className={styles.rows}>
      {order.map((item, index) => (
        <Row
          key={item.id}
          ref={(el) => {
            if (el) rows.current.set(item.id, el)
            else rows.current.delete(item.id)
          }}
          item={item}
          list={list}
          mine={mine}
          rank={showRank ? index + 1 : undefined}
          profileID={profileID}
          onChange={onChange}
          lifted={drag?.id === item.id ? drag.dy : undefined}
          handle={
            draggable && (
              <button
                type="button"
                className={own.handle}
                aria-label={`Move ${item.item.title.name ?? 'this'}. Use the arrow keys, or drag.`}
                onPointerDown={(e) => start(e, item.id)}
                onPointerMove={track}
                onPointerUp={drop}
                onPointerCancel={drop}
                onKeyDown={(e) => {
                  const to = e.key === 'ArrowUp' ? index - 1 : e.key === 'ArrowDown' ? index + 1 : -1
                  if (to < 0 || to >= order.length) return
                  e.preventDefault()
                  save(move(index, to))
                }}
              >
                <svg width="12" height="14" viewBox="0 0 12 14" aria-hidden="true">
                  <path d="M1 3h10M1 7h10M1 11h10" stroke="currentColor" strokeWidth="1.5" />
                </svg>
              </button>
            )
          }
        />
      ))}
      {reorder.isError && <p className={styles.error}>{problemMessage(reorder.error)}</p>}
    </ol>
  )
}

function what(i: Item) {
  switch (i.item.type) {
    case 'movie':
      return 'Film'
    case 'season':
      return `Season ${i.item.season}`
    case 'episode':
      return i.item.episode ? ep(i.item.episode.season, i.item.episode.number) : 'Episode'
  }
  return 'Show'
}

function progressWords(i: Item) {
  const p = i.progress
  if (!p) return undefined
  if (p.total > 0 && p.watched >= p.total) return 'Watched'
  if (i.item.type === 'show' || i.item.type === 'season') {
    return p.watched ? `${p.watched} of ${plural(p.total, 'episode')}` : p.total ? 'Not started' : 'Nothing aired yet'
  }
  return 'Not watched'
}

function Row({
  ref,
  item: i,
  list,
  mine,
  rank,
  profileID,
  onChange,
  lifted,
  handle,
}: {
  ref: (el: HTMLLIElement | null) => void
  item: Item
  list: List
  mine: boolean
  rank?: number
  profileID?: number
  onChange: () => void
  lifted?: number
  handle: ReactNode
}) {
  const title = useDetails(i.item.type === 'movie' ? 'movie' : 'show', i.item.title.ids.tmdb)
  const name = title?.name ?? i.item.title.name ?? 'Untitled'
  const [noting, setNoting] = useState(false)
  const [notes, setNotes] = useState(i.notes ?? '')
  const params = { path: { id: list.id, item: i.id }, query: { profile_id: profileID } }
  const remove = useMutation({
    mutationFn: async () => {
      const { error } = await api.DELETE('/v1/lists/{id}/items/{item}', { params })
      if (error) throw error
    },
    onSuccess: onChange,
  })
  const note = useMutation({
    mutationFn: async () => {
      const { error } = await api.PATCH('/v1/lists/{id}/items/{item}', { params, body: { notes } })
      if (error) throw error
    },
    onSuccess: () => {
      setNoting(false)
      onChange()
    },
  })
  const progress = progressWords(i)
  const done = progress === 'Watched'
  const facts = [what(i), title?.year ?? i.item.title.year, title?.certificate?.rating].filter(Boolean).join(', ')

  return (
    <li
      ref={ref}
      className={`${own.item} ${lifted !== undefined ? own.lifted : ''}`}
      style={lifted !== undefined ? { transform: `translateY(${lifted}px)` } : undefined}
    >
      <span className={own.grip}>{handle}</span>
      <span className={`${styles.code} ${styles.rank}`}>{rank}</span>
      <Cover id={i.item.title.ids.tethr ?? 0} name={name} poster={title?.poster} />
      <div className={own.body}>
        <TitleLink type={i.item.type} slug={i.item.title.slug} className={`${styles.rowTitle} ${styles.titleLink}`}>
          {name}
        </TitleLink>
        <p className={styles.quiet}>{facts}</p>
        {noting ? (
          <form
            className={own.note}
            onSubmit={(e) => {
              e.preventDefault()
              note.mutate()
            }}
          >
            <input
              aria-label="Note"
              value={notes}
              maxLength={500}
              onChange={(e) => setNotes(e.target.value)}
              // biome-ignore lint/a11y/noAutofocus: the field opens because the person asked to write a note
              autoFocus
            />
            <button type="submit" className={styles.textButton} disabled={note.isPending}>
              Save
            </button>
            <button type="button" className={styles.textButton} onClick={() => setNoting(false)}>
              Cancel
            </button>
          </form>
        ) : (
          i.notes && <p className={styles.small}>{i.notes}</p>
        )}
        {(note.isError || remove.isError) && (
          <p className={styles.error}>{problemMessage(note.error ?? remove.error)}</p>
        )}
      </div>
      <span className={done ? `${own.progress} ${own.done}` : own.progress}>
        {progress}
        {i.progress && !done && i.progress.total > 1 && i.progress.watched > 0 && (
          <span className={styles.meter}>
            <span style={{ transform: `scaleX(${i.progress.watched / i.progress.total})` }} />
          </span>
        )}
      </span>
      <span className={own.tools}>
        {mine && !noting && (
          <>
            <button type="button" className={styles.textButton} onClick={() => setNoting(true)}>
              {i.notes ? 'Change note' : 'Add a note'}
            </button>
            <button
              type="button"
              className={`${styles.textButton} ${styles.remove}`}
              onClick={() => remove.mutate()}
              disabled={remove.isPending}
            >
              Remove
            </button>
          </>
        )}
      </span>
    </li>
  )
}

function CoverItem({ item: i }: { item: Item }) {
  const title = useDetails(i.item.type === 'movie' ? 'movie' : 'show', i.item.title.ids.tmdb)
  const name = title?.name ?? i.item.title.name ?? 'Untitled'
  const progress = progressWords(i)
  return (
    <li className={own.cover}>
      <TitleLink type={i.item.type} slug={i.item.title.slug} className={styles.titleLink}>
        <Cover id={i.item.title.ids.tethr ?? 0} name={name} poster={title?.poster} size="l" />
        <span className={own.coverName}>{name}</span>
      </TitleLink>
      <span className={styles.faint}>
        {[what(i), title?.year ?? i.item.title.year].filter(Boolean).join(', ')}
        {progress === 'Watched' && ', watched'}
      </span>
    </li>
  )
}
