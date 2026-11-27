import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { api } from '../api/client'
import { plural } from '../format'
import styles from './WaitingPlays.module.css'

export function WaitingPlays() {
  const servers = useQuery({
    queryKey: ['media-servers'],
    queryFn: async () => (await api.GET('/v1/media-servers')).data ?? [],
  })
  const people = (servers.data ?? []).flatMap((s) =>
    s.users.filter((u) => !u.profile_id && !u.ignored && u.held_plays > 0),
  )
  if (!people.length) return null
  const plays = people.reduce((n, u) => n + u.held_plays, 0)
  const names = people.map((u) => u.name)
  const who = names.length > 1 ? `${names.slice(0, -1).join(', ')} and ${names[names.length - 1]}` : names[0]
  return (
    <p className={styles.waiting}>
      {plural(plays, 'play')} from your media server {plays === 1 ? 'is' : 'are'} waiting.{' '}
      <Link to="/media-servers">
        Match {who} to {people.length > 1 ? 'profiles' : 'a profile'}
      </Link>{' '}
      to log {plays === 1 ? 'it' : 'them'}.
    </p>
  )
}
