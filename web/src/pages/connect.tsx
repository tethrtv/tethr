import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useEffect } from 'react'
import { api, type Me } from '../api/client'
import { returnTo } from '../returnTo'
import styles from './Connect.module.css'

export function useMe() {
  return useQuery({
    queryKey: ['me'],
    queryFn: async () => {
      const { data, response } = await api.GET('/v1/me')
      if (response.status === 401) return null
      if (!data) throw new Error('could not load your account')
      return data
    },
  })
}

export function useSignedIn() {
  const navigate = useNavigate()
  const me = useMe()
  useEffect(() => {
    if (me.data !== null) return
    returnTo.set(location.pathname + location.search)
    navigate({ to: '/sign-in', replace: true })
  }, [me.data, navigate])
  useTimeZone(me.data)
  return me.data
}

export function useTimeZone(me: Me | null | undefined) {
  const queries = useQueryClient()
  const missing = me && !me.account.time_zone
  useEffect(() => {
    if (!missing) return
    const time_zone = Intl.DateTimeFormat().resolvedOptions().timeZone
    api
      .PATCH('/v1/me', { body: { time_zone } })
      .then(({ data }) => {
        if (data) queries.setQueryData(['me'], (old: Me | null | undefined) => old && { ...old, account: data })
      })
      .catch(() => {})
  }, [missing, queries])
}

export function ProfilePicker({
  legend,
  profiles,
  value,
  onChange,
}: {
  legend: string
  profiles: Me['profiles']
  value: number
  onChange: (id: number) => void
}) {
  if (profiles.length < 2) return null
  return (
    <fieldset className={styles.who}>
      <legend className={styles.rowLabel}>{legend}</legend>
      {profiles.map((p) => (
        <label key={p.id} className={value === p.id ? `${styles.option} ${styles.chosen}` : styles.option}>
          <input type="radio" name="profile" checked={value === p.id} onChange={() => onChange(p.id)} />
          {p.name}
        </label>
      ))}
    </fieldset>
  )
}
