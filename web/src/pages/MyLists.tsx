import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { api, problemMessage } from '../api/client'
import type { components } from '../api/schema'
import { Button } from '../components/Button'
import { Field } from '../components/Field'
import { ListCover } from '../components/ListCover'
import { useWatching } from '../components/Shell'
import { ago, plural } from '../format'
import own from './List.module.css'
import styles from './Lists.module.css'

type List = components['schemas']['List']

export const privacyWords: Record<List['privacy'], string> = {
  private: 'Only you',
  household: 'The household',
  public: 'Anyone',
}

export const goesOutside = (kind: string) => kind !== 'kids' && kind !== 'guest'

export const ownerName = (l: List) => l.owner.name ?? `@${l.owner.username}`

export const listPath = (l: List) => `${l.id}-${l.slug}`

export function MyLists() {
  const { profile } = useWatching()
  const queries = useQueryClient()
  const navigate = useNavigate()
  const [name, setName] = useState('')
  const params = { query: { profile_id: profile.id } }
  const lists = useQuery({
    queryKey: ['lists', profile.id],
    queryFn: async () => (await api.GET('/v1/lists', { params })).data ?? [],
  })
  const watchlist = useQuery({
    queryKey: ['watchlist', profile.id],
    queryFn: async () => (await api.GET('/v1/watchlist', { params })).data ?? [],
  })
  const auto = useQuery({
    queryKey: ['auto-lists', profile.id],
    queryFn: async () => (await api.GET('/v1/auto-lists', { params })).data ?? [],
  })
  const outside = goesOutside(profile.kind)
  const following = useQuery({
    queryKey: ['lists', profile.id, 'following'],
    enabled: outside,
    queryFn: async () => (await api.GET('/v1/followed-lists', { params })).data ?? [],
  })
  const create = useMutation({
    mutationFn: async () => {
      const { data, error } = await api.POST('/v1/lists', { params, body: { name } })
      if (error) throw error
      return data
    },
    onSuccess: (l) => {
      queries.invalidateQueries({ queryKey: ['lists', profile.id] })
      navigate({ to: '/lists/$list', params: { list: listPath(l) } })
    },
  })
  const mine = lists.data?.filter((l) => l.access === 'mine') ?? []
  const shared = lists.data?.filter((l) => l.access !== 'mine') ?? []

  return (
    <div className={styles.page}>
      <header className={styles.head}>
        <h1 className={styles.title}>Lists</h1>
        <p className={styles.lead}>
          Your watchlist, and lists you make yourself. Keep a list to yourself, share it with the household, or let{' '}
          <strong>anyone</strong> find it.
        </p>
        {outside && (
          <p>
            <Link to="/lists/browse" className={styles.titleLink}>
              Browse lists other people have shared
            </Link>
          </p>
        )}
      </header>

      <section>
        <h2 className={styles.sectionTitle}>Yours</h2>
        <ol className={styles.rows}>
          <li className={own.list}>
            <ListCover items={watchlist.data?.slice(0, 4).map((l) => l.item) ?? []} />
            <div className={styles.main}>
              <Link to="/watchlist" className={`${styles.rowTitle} ${styles.titleLink}`}>
                Watchlist
              </Link>
              <span className={styles.quiet}>Only you</span>
            </div>
            <span className={`${styles.code} ${own.count}`}>
              {watchlist.data ? plural(watchlist.data.length, 'thing') : ''}
            </span>
            <span className={styles.faint} />
          </li>
          {mine.map((l) => (
            <ListRow key={l.id} list={l} />
          ))}
        </ol>
        <form
          className={own.create}
          onSubmit={(e) => {
            e.preventDefault()
            if (name.trim()) create.mutate()
          }}
        >
          <Field
            label="New list"
            value={name}
            onChange={setName}
            maxLength={100}
            hint="Films to watch with Dad, the best of 2026, a franchise in story order."
            error={create.isError ? problemMessage(create.error) : undefined}
          />
          <Button variant="primary" type="submit" disabled={!name.trim() || create.isPending}>
            Make the list
          </Button>
        </form>
      </section>

      {!!auto.data?.length && (
        <section>
          <h2 className={styles.sectionTitle}>From what you watch</h2>
          <ol className={styles.rows}>
            {auto.data.map((l) => (
              <li key={l.kind} className={own.list}>
                <ListCover items={l.cover} />
                <div className={styles.main}>
                  <Link
                    to="/lists/auto/$kind"
                    params={{ kind: l.kind }}
                    className={`${styles.rowTitle} ${styles.titleLink}`}
                  >
                    {l.name}
                  </Link>
                  <span className={styles.quiet}>{l.description}</span>
                </div>
                <span className={`${styles.code} ${own.count}`}>{plural(l.item_count, 'thing')}</span>
                <span className={styles.faint}>Only you</span>
              </li>
            ))}
          </ol>
        </section>
      )}

      {!!following.data?.length && (
        <section>
          <h2 className={styles.sectionTitle}>Following</h2>
          <ol className={styles.rows}>
            {following.data.map((l) => (
              <ListRow key={l.id} list={l} />
            ))}
          </ol>
        </section>
      )}

      {shared.length > 0 && (
        <section>
          <h2 className={styles.sectionTitle}>From the household</h2>
          <ol className={styles.rows}>
            {shared.map((l) => (
              <ListRow key={l.id} list={l} />
            ))}
          </ol>
        </section>
      )}
    </div>
  )
}

export function ListRow({ list: l }: { list: List }) {
  return (
    <li className={own.list}>
      <ListCover items={l.cover} />
      <div className={styles.main}>
        <Link to="/lists/$list" params={{ list: listPath(l) }} className={`${styles.rowTitle} ${styles.titleLink}`}>
          {l.name}
        </Link>
        <span className={styles.quiet}>
          {l.access === 'mine' ? (
            privacyWords[l.privacy]
          ) : l.access === 'household' ? (
            `From ${ownerName(l)}`
          ) : (
            <>
              From{' '}
              <Link to="/@{$username}" params={{ username: l.owner.username }} className={styles.titleLink}>
                {ownerName(l)}
              </Link>
              {l.follower_count > 0 && `, ${plural(l.follower_count, 'follower')}`}
            </>
          )}
        </span>
      </div>
      <span className={`${styles.code} ${own.count}`}>
        {plural(l.item_count, 'thing')}
        {l.watched_count ? `, ${l.watched_count} watched` : ''}
      </span>
      <span className={styles.faint}>Changed {ago(l.updated_at)}</span>
    </li>
  )
}
