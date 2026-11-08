import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { api, problemMessage } from '../api/client'
import type { components } from '../api/schema'
import { Cover } from '../components/Cover'
import { ListCover } from '../components/ListCover'
import { useWatching } from '../components/Shell'
import { TitleLink } from '../components/TitleLink'
import { ago, plural } from '../format'
import { useDetails } from '../tmdb'
import { kinds, Option, progressWords, what } from './List'
import own from './List.module.css'
import styles from './Lists.module.css'
import { listParams } from './MyLists'

type Kind = components['schemas']['AutoListKind']
type Item = components['schemas']['AutoListItem']

const orders = [
  { label: 'List order', by: 'rank', how: 'asc' },
  { label: 'Title', by: 'title', how: 'asc' },
  { label: 'Newest', by: 'year', how: 'desc' },
  { label: 'Oldest', by: 'year', how: 'asc' },
] as const

const atWords: Record<Kind, string> = {
  stopped: 'Last watched',
  favourites: 'Rated',
  rewatch: 'Last watched',
  household: 'Rated',
}

export function AutoListPage({ kind }: { kind: string }) {
  const { profile } = useWatching()
  const queries = useQueryClient()
  const navigate = useNavigate()
  const [type, setType] = useState<(typeof kinds)[number]['value']>('all')
  const [hideWatched, setHideWatched] = useState(false)
  const [order, setOrder] = useState<(typeof orders)[number]>(orders[0])
  const page = useQuery({
    queryKey: ['auto-list', kind, profile.id, type, hideWatched, order.label],
    placeholderData: keepPreviousData,
    queryFn: async () => {
      const { data, response } = await api.GET('/v1/auto-lists/{kind}', {
        params: {
          path: { kind: kind as Kind },
          query: {
            profile_id: profile.id,
            limit: 1000,
            type: type === 'all' ? undefined : type,
            hide_watched: hideWatched || undefined,
            sort_by: order.by,
            sort_how: order.how,
          },
        },
      })
      if (response.status === 404) return null
      if (!data) throw new Error('could not load the list')
      return data
    },
  })
  const copy = useMutation({
    mutationFn: async () => {
      const { data, error } = await api.POST('/v1/auto-lists/{kind}/copy', {
        params: { path: { kind: kind as Kind }, query: { profile_id: profile.id } },
      })
      if (error) throw error
      return data
    },
    onSuccess: (l) => {
      queries.invalidateQueries({ queryKey: ['lists', profile.id] })
      navigate({ to: '/@{$username}/lists/$slug', params: listParams(l) })
    },
  })

  if (page.isError) return <p className={styles.error}>{problemMessage(page.error)}</p>
  if (page.data === null) {
    return (
      <div className={styles.page}>
        <header className={styles.head}>
          <h1 className={styles.title}>No list here</h1>
          <p className={styles.lead}>There’s no list like this for {profile.name}.</p>
        </header>
      </div>
    )
  }
  if (!page.data) return null

  const list = page.data.list
  return (
    <div className={styles.page}>
      <header className={own.top}>
        {list.cover.length > 0 && <ListCover items={list.cover} size="l" />}
        <div className={styles.head}>
          <h1 className={styles.title}>{list.name}</h1>
          <p className={styles.lead}>{list.description}</p>
          <p className={styles.small}>
            Only you can see it. It fills itself from what you watch and rate,{' '}
            {list.item_count === 0
              ? 'and there’s nothing on it yet.'
              : `and has ${plural(list.item_count, 'thing')} on it now.`}
          </p>
          {list.item_count > 0 && (
            <p className={own.actions}>
              <button
                type="button"
                className={styles.textButton}
                onClick={() => copy.mutate()}
                disabled={copy.isPending}
              >
                Keep a copy as a list of your own
              </button>
            </p>
          )}
          {copy.isError && <p className={styles.error}>{problemMessage(copy.error)}</p>}
        </div>
      </header>

      <div className={`${styles.filters} ${own.filters}`}>
        <fieldset className={styles.options}>
          <legend className={styles.hidden}>Type</legend>
          {kinds.map((k) => (
            <Option key={k.value} name="kind" checked={k.value === type} onChange={() => setType(k.value)}>
              {k.label}
            </Option>
          ))}
        </fieldset>
        <label className={hideWatched ? `${styles.option} ${styles.chosen}` : styles.option}>
          <input type="checkbox" checked={hideWatched} onChange={(e) => setHideWatched(e.target.checked)} />
          Hide watched
        </label>
        <label className={own.select}>
          <span className={styles.faint}>Order</span>
          <select
            value={order.label}
            onChange={(e) => setOrder(orders.find((o) => o.label === e.target.value) ?? orders[0])}
          >
            {orders.map((o) => (
              <option key={o.label}>{o.label}</option>
            ))}
          </select>
        </label>
      </div>

      <section>
        <h2 className={styles.sectionTitle}>{order.label}</h2>
        {page.data.items.length === 0 ? (
          <p className={styles.quiet}>
            {page.data.total === 0 && type === 'all' && !hideWatched
              ? 'Nothing on this list yet.'
              : 'Nothing here matches.'}
          </p>
        ) : (
          <ol className={styles.rows}>
            {page.data.items.map((i) => (
              <Row key={i.rank} item={i} kind={list.kind} showRank={order.by === 'rank'} />
            ))}
          </ol>
        )}
      </section>
    </div>
  )
}

function Row({ item: i, kind, showRank }: { item: Item; kind: Kind; showRank: boolean }) {
  const title = useDetails(i.item.type === 'movie' ? 'movie' : 'show', i.item.title.ids.tmdb)
  const name = title?.name ?? i.item.title.name ?? 'Untitled'
  const progress = progressWords(i)
  const done = progress === 'Watched'
  const facts = [what(i), title?.year ?? i.item.title.year, i.rating && `you rated it ${i.rating}`]
    .filter(Boolean)
    .join(', ')
  return (
    <li className={own.item}>
      <span className={own.grip} />
      <span className={`${styles.code} ${styles.rank}`}>{showRank ? i.rank : undefined}</span>
      <Cover id={i.item.title.ids.tethr ?? 0} name={name} poster={title?.poster} />
      <div className={own.body}>
        <TitleLink type={i.item.type} slug={i.item.title.slug} className={`${styles.rowTitle} ${styles.titleLink}`}>
          {name}
        </TitleLink>
        <p className={styles.quiet}>{facts}</p>
        {title?.overview && <p className={`${styles.small} ${own.synopsis}`}>{title.overview}</p>}
      </div>
      <span className={done ? `${own.progress} ${own.done}` : own.progress}>
        {progress}
        {i.progress && !done && i.progress.total > 1 && i.progress.watched > 0 && (
          <span className={styles.meter}>
            <span style={{ transform: `scaleX(${i.progress.watched / i.progress.total})` }} />
          </span>
        )}
      </span>
      <span className={`${own.tools} ${styles.faint}`}>
        {atWords[kind]} {ago(i.at)}
      </span>
    </li>
  )
}
