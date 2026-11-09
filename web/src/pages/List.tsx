import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate, useRouterState } from '@tanstack/react-router'
import { type PointerEvent, type ReactNode, useEffect, useRef, useState } from 'react'
import { api, problemMessage } from '../api/client'
import type { components } from '../api/schema'
import { Button } from '../components/Button'
import { Cover } from '../components/Cover'
import { Field } from '../components/Field'
import { ListCover } from '../components/ListCover'
import { Slip, SlipBody, SlipFoot } from '../components/Slip'
import { TitleLink } from '../components/TitleLink'
import { useToast } from '../components/Toast'
import { MissingList } from '../components/Trouble'
import { ep, plural } from '../format'
import { useDetails } from '../tmdb'
import own from './List.module.css'
import styles from './Lists.module.css'
import { goesOutside, listParams, MadeByTethr, ownerName, privacyWords } from './MyLists'

type List = components['schemas']['List']
type Item = components['schemas']['ListItem']
type Privacy = List['privacy']
type Reason = components['schemas']['NewReport']['reason']

const reasons: { value: Reason; label: string }[] = [
  { value: 'illegal', label: 'Something illegal' },
  { value: 'harassment', label: 'Threats or abuse' },
  { value: 'personal', label: 'Someone’s private details' },
  { value: 'spam', label: 'Adverts or scams' },
  { value: 'other', label: 'Something else' },
]

const orders = [
  { label: 'List order', by: 'rank', how: 'asc' },
  { label: 'Recently added', by: 'added', how: 'desc' },
  { label: 'Title', by: 'title', how: 'asc' },
  { label: 'Newest', by: 'year', how: 'desc' },
  { label: 'Oldest', by: 'year', how: 'asc' },
] as const
type Order = (typeof orders)[number]

export const kinds = [
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

type Where = { profileID?: number; profileKind?: string }

const addressKey = (username: string, slug: string, profileID?: number) => [
  'list address',
  username.toLowerCase(),
  slug,
  profileID ?? 0,
]

export function ListAt({ username, slug, profileID, profileKind }: { username: string; slug: string } & Where) {
  const found = useQuery({
    queryKey: addressKey(username, slug, profileID),
    queryFn: async () => {
      const { data, response } = await api.GET('/v1/users/{username}/lists/{slug}', {
        params: { path: { username, slug }, query: profileID ? { profile_id: profileID } : {} },
      })
      if (response.status === 404) return null
      if (!data) throw new Error('could not find the list')
      return data.id
    },
  })
  if (found.isError) return <p className={styles.error}>{problemMessage(found.error)}</p>
  if (found.data === null) return <MissingList signedIn={!!profileID} />
  if (!found.data) return null
  return <ListPage key={found.data} id={found.data} profileID={profileID} profileKind={profileKind} />
}

function ListPage({ id, profileID, profileKind }: { id: number } & Where) {
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
  const cover = useMutation({
    mutationFn: async (item: number) => {
      const { error } = await api.PATCH('/v1/lists/{id}', {
        params: { path: { id }, query },
        body: { cover_item_id: item },
      })
      if (error) throw error
    },
    onSuccess: refresh,
  })

  const navigate = useNavigate()
  const here = useRouterState({ select: (s) => s.location.pathname })
  const shown = page.data?.list
  useEffect(() => {
    if (!shown) return
    const params = listParams(shown)
    if (decodeURIComponent(here) === `/@${params.username}/lists/${params.slug}`) return
    queries.setQueryData(addressKey(params.username, params.slug, profileID), shown.id)
    navigate({ to: '/@{$username}/lists/$slug', params, replace: true })
  }, [shown, here, profileID, queries, navigate])

  if (page.data === null) return <MissingList signedIn={!!profileID} />
  if (!page.data) return null

  const list = page.data.list
  const mine = list.access === 'mine' && !!profileID
  const editable = list.can_edit_items && !!profileID
  const chosen = order ?? orders.find((o) => o.by === list.sort_by && o.how === list.sort_how) ?? orders[0]
  const words = find.trim().toLowerCase()
  const items = words
    ? page.data.items.filter((i) => (i.item.title.name ?? '').toLowerCase().includes(words))
    : page.data.items
  const draggable = editable && chosen.by === 'rank' && chosen.how === 'asc' && kind === 'all' && !hideWatched && !words

  return (
    <div className={styles.page}>
      <Head list={list} mine={mine} profileID={profileID} profileKind={profileKind} onChange={refresh} />

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
              ? editable
                ? 'Nothing on this list yet. Add films and shows from their pages.'
                : 'Nothing on this list yet.'
              : 'Nothing here matches.'}
          </p>
        ) : view === 'covers' ? (
          <ol className={own.covers}>
            {items.map((i) => (
              <CoverItem
                key={i.id}
                item={i}
                isCover={i.id === list.cover_item_id}
                onPick={mine && !cover.isPending ? cover.mutate : undefined}
              />
            ))}
          </ol>
        ) : (
          <Rows
            key={items.map((i) => i.id).join()}
            list={list}
            items={items}
            editable={editable}
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

export function Option({
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
  profileKind,
  onChange,
}: {
  list: List
  mine: boolean
  profileID?: number
  profileKind?: string
  onChange: () => void
}) {
  const [slip, setSlip] = useState<'details' | 'delete' | 'report' | 'followers' | null>(null)
  const [copied, setCopied] = useState(false)
  const queries = useQueryClient()
  const upNext = useMutation({
    mutationFn: async () => {
      const params = { params: { path: { id: list.id }, query: { profile_id: profileID } } }
      const { error } = list.in_up_next
        ? await api.DELETE('/v1/up-next/lists/{id}', params)
        : await api.PUT('/v1/up-next/lists/{id}', params)
      if (error) throw error
    },
    onSuccess: () => {
      onChange()
      queries.invalidateQueries({ queryKey: ['up-next-lists', profileID] })
    },
  })
  const follow = useMutation({
    mutationFn: async () => {
      const params = { params: { path: { id: list.id }, query: { profile_id: profileID } } }
      const { error } = list.following
        ? await api.DELETE('/v1/followed-lists/{id}', params)
        : await api.PUT('/v1/followed-lists/{id}', params)
      if (error) throw error
    },
    onSuccess: onChange,
  })
  const outsider = list.access === 'public' && !!profileID
  const canFollow = outsider && goesOutside(profileKind ?? '')
  const close = () => setSlip(null)
  const by =
    list.access === 'mine' ? null : list.access === 'household' ? (
      `${ownerName(list)}’s list. `
    ) : list.owner.official ? (
      <>
        <MadeByTethr />.{' '}
      </>
    ) : (
      <>
        A list by{' '}
        <Link to="/@{$username}" params={{ username: list.owner.username }} className={styles.titleLink}>
          {ownerName(list)}
        </Link>
        .{' '}
      </>
    )

  return (
    <header className={own.top}>
      {list.cover.length > 0 && <ListCover items={list.cover} size="l" />}
      <div className={styles.head}>
        <h1 className={styles.title}>{list.name}</h1>
        {list.description && <p className={styles.lead}>{list.description}</p>}
        <p className={styles.small}>
          {by}
          {!mine && list.can_edit_items && profileID && 'You can add to it, take things off and change the order. '}
          {list.item_count === 0
            ? 'Nothing on it yet.'
            : `${plural(list.item_count, 'thing')}${list.watched_count !== undefined ? `, ${list.watched_count} watched` : ''}.`}
          {list.in_up_next && ' Up next shows what to watch from it next.'}
          {list.privacy === 'public' && list.follower_count > 0 && ` ${plural(list.follower_count, 'follower')}.`}
        </p>
        {(upNext.isError || follow.isError) && (
          <p className={styles.error}>{problemMessage(upNext.error ?? follow.error)}</p>
        )}
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
          {mine && (
            <>
              <button type="button" className={styles.textButton} onClick={() => setSlip('details')}>
                Change details
              </button>
              <button type="button" className={`${styles.textButton} ${own.danger}`} onClick={() => setSlip('delete')}>
                Delete the list
              </button>
            </>
          )}
          {canFollow && (
            <button
              type="button"
              className={styles.textButton}
              onClick={() => follow.mutate()}
              disabled={follow.isPending}
            >
              {list.following ? 'Stop following' : 'Follow'}
            </button>
          )}
          {profileID && (
            <button
              type="button"
              className={styles.textButton}
              onClick={() => upNext.mutate()}
              disabled={upNext.isPending}
            >
              {list.in_up_next ? 'Take it out of Up next' : 'Watch it in order in Up next'}
            </button>
          )}
          {mine && list.follower_count > 0 && (
            <button type="button" className={styles.textButton} onClick={() => setSlip('followers')}>
              See who follows it
            </button>
          )}
          {outsider && (
            <button
              type="button"
              className={`${styles.textButton} ${own.quietAction}`}
              onClick={() => setSlip('report')}
            >
              Report
            </button>
          )}
          {!profileID && (
            <Link to="/sign-in" className={styles.textButton}>
              Sign in to see what you’ve watched
            </Link>
          )}
        </p>
      </div>
      {mine && (
        <DetailsSlip
          list={list}
          profileID={profileID}
          open={slip === 'details' || slip === 'delete'}
          step={slip === 'delete' ? 'delete' : 'details'}
          onStep={setSlip}
          onClose={close}
          onChange={onChange}
        />
      )}
      {mine && (
        <Slip open={slip === 'followers'} onClose={close} title="Who follows it">
          <SlipBody>
            <Followers list={list} profileID={profileID} />
          </SlipBody>
        </Slip>
      )}
      {outsider && <ReportSlip list={list} profileID={profileID} open={slip === 'report'} onClose={close} />}
    </header>
  )
}

function DetailsSlip({
  list,
  profileID,
  open,
  step,
  onStep,
  onClose,
  onChange,
}: {
  list: List
  profileID?: number
  open: boolean
  step: 'details' | 'delete'
  onStep: (step: 'details' | 'delete') => void
  onClose: () => void
  onChange: () => void
}) {
  const [name, setName] = useState(list.name)
  const [description, setDescription] = useState(list.description ?? '')
  const [privacy, setPrivacy] = useState<Privacy>(list.privacy)
  const [wasOpen, setWasOpen] = useState(open)
  if (open !== wasOpen) {
    setWasOpen(open)
    if (open) {
      setName(list.name)
      setDescription(list.description ?? '')
      setPrivacy(list.privacy)
    }
  }
  return (
    <Slip
      open={open}
      onClose={onClose}
      step={step}
      title={step === 'delete' ? `Delete ${list.name}?` : `Change ${list.name}`}
    >
      {step === 'delete' ? (
        <DeleteList list={list} profileID={profileID} onKeep={() => onStep('details')} onChange={onChange} />
      ) : (
        <ListDetails
          list={list}
          profileID={profileID}
          fields={{ name, description, privacy }}
          set={{ name: setName, description: setDescription, privacy: setPrivacy }}
          onDelete={() => onStep('delete')}
          onClose={onClose}
          onChange={onChange}
        />
      )}
    </Slip>
  )
}

function ListDetails({
  list,
  profileID,
  fields,
  set,
  onDelete,
  onClose,
  onChange,
}: {
  list: List
  profileID?: number
  fields: { name: string; description: string; privacy: Privacy }
  set: { name: (v: string) => void; description: (v: string) => void; privacy: (v: Privacy) => void }
  onDelete: () => void
  onClose: () => void
  onChange: () => void
}) {
  const save = useMutation({
    mutationFn: async () => {
      const { error } = await api.PATCH('/v1/lists/{id}', {
        params: { path: { id: list.id }, query: { profile_id: profileID } },
        body: fields,
      })
      if (error) throw error
    },
    onSuccess: () => {
      onChange()
      onClose()
    },
  })
  return (
    <>
      <SlipBody>
        <form
          id="list-details"
          className={own.form}
          onSubmit={(e) => {
            e.preventDefault()
            save.mutate()
          }}
        >
          <Field label="Name" value={fields.name} onChange={set.name} maxLength={100} required />
          <Field label="Description" value={fields.description} onChange={set.description} maxLength={1000} />
          <fieldset className={styles.options}>
            <legend className={own.legend}>Who can see it</legend>
            {(Object.keys(privacyWords) as Privacy[]).map((p) => (
              <Option key={p} name="privacy" checked={p === fields.privacy} onChange={() => set.privacy(p)}>
                {privacyWords[p]}
              </Option>
            ))}
          </fieldset>
          <div className={fields.privacy === 'public' ? `${own.reveal} ${own.revealed}` : own.reveal}>
            <p className={own.warning}>
              Anyone can find this list when they browse, and see its notes and your username. If your profile is
              public, they see your name too.
            </p>
          </div>
          <p className={styles.small}>
            {list.cover_item_id
              ? 'To change the cover, switch to Covers and pick another.'
              : 'The cover is the first four things on the list. To pick one instead, switch to Covers.'}
          </p>
          {save.isError && <p className={styles.error}>{problemMessage(save.error)}</p>}
          <p>
            <button type="button" className={`${styles.textButton} ${own.danger}`} onClick={onDelete}>
              Delete the list
            </button>
          </p>
        </form>
      </SlipBody>
      <SlipFoot>
        <Button variant="primary" type="submit" form="list-details" disabled={!fields.name.trim() || save.isPending}>
          Save
        </Button>
        <Button variant="text" onClick={onClose}>
          Cancel
        </Button>
      </SlipFoot>
    </>
  )
}

function DeleteList({
  list,
  profileID,
  onKeep,
  onChange,
}: {
  list: List
  profileID?: number
  onKeep: () => void
  onChange: () => void
}) {
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
  return (
    <>
      <SlipBody>
        <p>
          Everything on it comes off, and it can’t be undone. Your history stays as it is.
          {list.follower_count > 0 && ` Its ${plural(list.follower_count, 'follower')} will lose it too.`}
        </p>
        {remove.isError && <p className={styles.error}>{problemMessage(remove.error)}</p>}
      </SlipBody>
      <SlipFoot>
        <Button variant="danger" onClick={() => remove.mutate()} disabled={remove.isPending}>
          Delete it
        </Button>
        <Button variant="text" onClick={onKeep}>
          Keep it
        </Button>
      </SlipFoot>
    </>
  )
}

function Rows({
  list,
  items,
  editable,
  draggable,
  showRank,
  profileID,
  onChange,
}: {
  list: List
  items: Item[]
  editable: boolean
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
          editable={editable}
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

export function what(i: Pick<Item, 'item'>) {
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

export function progressWords(i: Pick<Item, 'item' | 'progress'>) {
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
  editable,
  rank,
  profileID,
  onChange,
  lifted,
  handle,
}: {
  ref: (el: HTMLLIElement | null) => void
  item: Item
  list: List
  editable: boolean
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
  const [gone, setGone] = useState(false)
  const toast = useToast()
  const params = { path: { id: list.id, item: i.id }, query: { profile_id: profileID } }
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
  const remove = () => {
    setGone(true)
    toast({
      text: `Took ${name} off ${list.name}.`,
      commit: async () => {
        const { error } = await api.DELETE('/v1/lists/{id}/items/{item}', { params })
        if (error) throw error
        onChange()
      },
      undo: () => setGone(false),
    })
  }
  const progress = progressWords(i)
  const done = progress === 'Watched'
  const facts = [
    what(i),
    title?.year ?? i.item.title.year,
    title?.certificate?.rating,
    title?.score && `TMDB ${title.score.toFixed(1)}`,
  ]
    .filter(Boolean)
    .join(', ')
  const synopsis = (i.item.type === 'movie' || i.item.type === 'show') && title?.overview

  if (gone) return null
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
        {synopsis && <p className={`${styles.small} ${own.synopsis}`}>{synopsis}</p>}
        {i.notes && <p className={`${styles.small} ${own.noteText}`}>{i.notes}</p>}
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
        {editable && (
          <>
            <button
              type="button"
              className={styles.textButton}
              onClick={() => {
                setNotes(i.notes ?? '')
                note.reset()
                setNoting(true)
              }}
            >
              {i.notes ? 'Change note' : 'Add a note'}
            </button>
            <button type="button" className={`${styles.textButton} ${styles.remove}`} onClick={remove}>
              Remove
            </button>
          </>
        )}
      </span>
      {editable && (
        <Slip open={noting} onClose={() => setNoting(false)} title={`Note on ${name}`}>
          <SlipBody>
            <form
              id={`note-${i.id}`}
              className={own.form}
              onSubmit={(e) => {
                e.preventDefault()
                note.mutate()
              }}
            >
              <Field
                label="Note"
                value={notes}
                onChange={setNotes}
                maxLength={500}
                hint="Anyone who can see the list sees its notes."
              />
              {note.isError && <p className={styles.error}>{problemMessage(note.error)}</p>}
            </form>
          </SlipBody>
          <SlipFoot>
            <Button variant="primary" type="submit" form={`note-${i.id}`} disabled={note.isPending}>
              Save
            </Button>
            <Button variant="text" onClick={() => setNoting(false)}>
              Cancel
            </Button>
          </SlipFoot>
        </Slip>
      )}
    </li>
  )
}

function CoverItem({ item: i, isCover, onPick }: { item: Item; isCover: boolean; onPick?: (item: number) => void }) {
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
      {onPick &&
        (isCover ? (
          <span className={styles.faint}>
            The cover.{' '}
            <button type="button" className={styles.textButton} onClick={() => onPick(0)}>
              Use the first four
            </button>
          </span>
        ) : (
          <button type="button" className={`${styles.textButton} ${own.pick}`} onClick={() => onPick(i.id)}>
            Use as the cover
          </button>
        ))}
    </li>
  )
}

function Followers({ list, profileID }: { list: List; profileID?: number }) {
  const followers = useQuery({
    queryKey: ['followers', list.id],
    queryFn: async () =>
      (
        await api.GET('/v1/lists/{id}/followers', {
          params: { path: { id: list.id }, query: { profile_id: profileID } },
        })
      ).data ?? [],
  })
  if (!followers.data) return <p className={styles.quiet}>Looking for followers…</p>
  return (
    <ul className={own.followers}>
      {followers.data.map((f) => (
        <li key={f.username}>
          <Link to="/@{$username}" params={{ username: f.username }} className={styles.titleLink}>
            @{f.username}
          </Link>
          {f.name && <span className={styles.faint}> {f.name}</span>}
        </li>
      ))}
    </ul>
  )
}

function ReportSlip({
  list,
  profileID,
  open,
  onClose,
}: {
  list: List
  profileID?: number
  open: boolean
  onClose: () => void
}) {
  const [sent, setSent] = useState(false)
  const [wasOpen, setWasOpen] = useState(open)
  if (open !== wasOpen) {
    setWasOpen(open)
    if (open) setSent(false)
  }
  return (
    <Slip
      open={open}
      onClose={onClose}
      step={sent ? 'sent' : 'form'}
      title={sent ? 'Thanks for telling us' : `Report ${list.name}`}
    >
      {sent ? (
        <>
          <SlipBody>
            <p>We’ll take a look, usually within a few days. Until then it won’t show up when people browse lists.</p>
          </SlipBody>
          <SlipFoot>
            <Button variant="primary" onClick={onClose}>
              Done
            </Button>
          </SlipFoot>
        </>
      ) : (
        <ReportForm list={list} profileID={profileID} onSent={() => setSent(true)} onClose={onClose} />
      )}
    </Slip>
  )
}

function ReportForm({
  list,
  profileID,
  onSent,
  onClose,
}: {
  list: List
  profileID?: number
  onSent: () => void
  onClose: () => void
}) {
  const [reason, setReason] = useState<Reason>()
  const [note, setNote] = useState('')
  const send = useMutation({
    mutationFn: async (r: Reason) => {
      const { error } = await api.POST('/v1/lists/{id}/reports', {
        params: { path: { id: list.id }, query: { profile_id: profileID } },
        body: { reason: r, note: note.trim() || undefined },
      })
      if (error) throw error
    },
    onSuccess: onSent,
  })
  return (
    <>
      <SlipBody>
        <form
          id="report"
          className={own.form}
          onSubmit={(e) => {
            e.preventDefault()
            if (reason) send.mutate(reason)
          }}
        >
          <fieldset className={styles.options}>
            <legend className={own.legend}>What’s wrong with it?</legend>
            {reasons.map((r) => (
              <Option key={r.value} name="reason" checked={r.value === reason} onChange={() => setReason(r.value)}>
                {r.label}
              </Option>
            ))}
          </fieldset>
          <Field label="Anything that would help us (optional)" value={note} onChange={setNote} maxLength={500} />
          {send.isError && <p className={styles.error}>{problemMessage(send.error)}</p>}
        </form>
      </SlipBody>
      <SlipFoot>
        <Button variant="primary" type="submit" form="report" disabled={!reason || send.isPending}>
          Send the report
        </Button>
        <Button variant="text" onClick={onClose}>
          Cancel
        </Button>
      </SlipFoot>
    </>
  )
}
