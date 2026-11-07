import { keepPreviousData, useInfiniteQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { api, problemMessage } from '../api/client'
import { Button } from '../components/Button'
import { useWatching } from '../components/Shell'
import { Option } from './List'
import own from './List.module.css'
import styles from './Lists.module.css'
import { goesOutside, ListRow } from './MyLists'

const orders = [
  { value: 'newest', label: 'Newest' },
  { value: 'popular', label: 'Most followed' },
] as const

export function Browse() {
  const { profile } = useWatching()
  const [order, setOrder] = useState<(typeof orders)[number]['value']>('newest')
  const [search, setSearch] = useState('')
  const q = search.trim()
  const lists = useInfiniteQuery({
    queryKey: ['public-lists', profile.id, order, q],
    enabled: goesOutside(profile.kind),
    placeholderData: keepPreviousData,
    initialPageParam: 1,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await api.GET('/v1/public-lists', {
        params: { query: { profile_id: profile.id, order, q: q || undefined, page: pageParam } },
      })
      if (error) throw error
      return data
    },
    getNextPageParam: (last) => (last.page * last.limit < last.total ? last.page + 1 : undefined),
  })
  const found = lists.data?.pages.flatMap((p) => p.lists) ?? []
  const total = lists.data?.pages[0]?.total

  if (!goesOutside(profile.kind)) {
    return (
      <div className={styles.page}>
        <header className={styles.head}>
          <h1 className={styles.title}>Browse lists</h1>
          <p className={styles.lead}>Kids and guest profiles see lists from their own household only.</p>
        </header>
      </div>
    )
  }
  return (
    <div className={styles.page}>
      <title>Browse lists · Tethr</title>
      <header className={styles.head}>
        <h1 className={styles.title}>Browse lists</h1>
        <p className={styles.lead}>
          Lists people have made public. Follow one to keep it on your lists page, or watch it in order in Up next.
        </p>
      </header>

      <div className={`${styles.filters} ${own.filters}`}>
        <fieldset className={styles.options}>
          <legend className={styles.hidden}>Order</legend>
          {orders.map((o) => (
            <Option key={o.value} name="order" checked={o.value === order} onChange={() => setOrder(o.value)}>
              {o.label}
            </Option>
          ))}
        </fieldset>
        <input
          className={own.find}
          type="search"
          placeholder="Find a list by name"
          aria-label="Find a list by name"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
      </div>

      <section>
        <h2 className={styles.sectionTitle}>
          {orders.find((o) => o.value === order)?.label}
          {total !== undefined && <span className={styles.faint}> {total}</span>}
        </h2>
        {lists.isError && <p className={styles.error}>{problemMessage(lists.error)}</p>}
        {total === 0 ? (
          <p className={styles.quiet}>
            {q ? 'No public lists have that in their name.' : 'Nobody has made a list public yet.'}
          </p>
        ) : (
          <ol className={styles.rows}>
            {found.map((l) => (
              <ListRow key={l.id} list={l} />
            ))}
          </ol>
        )}
        {lists.hasNextPage && (
          <p className={own.actions}>
            <Button onClick={() => lists.fetchNextPage()} disabled={lists.isFetchingNextPage}>
              Show more
            </Button>
          </p>
        )}
      </section>
    </div>
  )
}
