import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import styles from './Lists.module.css'
import { ListRow } from './MyLists'
import { Station } from './Station'

export function UserPage({ username, profileID }: { username: string; profileID?: number }) {
  const user = useQuery({
    queryKey: ['user', username, profileID ?? 0],
    queryFn: async () => {
      const { data, response } = await api.GET('/v1/users/{username}', {
        params: { path: { username }, query: profileID ? { profile_id: profileID } : {} },
      })
      if (response.status === 404 || response.status === 403) return null
      if (!data) throw new Error('could not load the page')
      return data
    },
  })

  if (user.data === null) {
    return (
      <div className={styles.page}>
        <header className={styles.head}>
          <h1 className={styles.title}>Nobody here</h1>
          <p className={styles.lead}>There’s nobody called @{username}, or their page isn’t open to this profile.</p>
        </header>
      </div>
    )
  }
  if (!user.data) return null
  const u = user.data
  if (u.username === 'tethr') return <Station user={u} />

  return (
    <div className={styles.page}>
      <title>{`@${u.username} · Tethr`}</title>
      <header className={styles.head}>
        <h1 className={styles.title}>{u.name ?? `@${u.username}`}</h1>
        <p className={styles.lead}>
          {u.name && <span className={styles.code}>@{u.username}. </span>}
          {u.private ? 'Their profile is private, so you only see the lists they’ve shared.' : 'Lists they’ve shared.'}
        </p>
      </header>

      <section>
        <h2 className={styles.sectionTitle}>Lists</h2>
        {u.lists.length === 0 ? (
          <p className={styles.quiet}>Nothing shared yet.</p>
        ) : (
          <ol className={styles.rows}>
            {u.lists.map((l) => (
              <ListRow key={l.id} list={l} />
            ))}
          </ol>
        )}
      </section>
    </div>
  )
}
