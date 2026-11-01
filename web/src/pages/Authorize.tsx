import { useMutation, useQuery } from '@tanstack/react-query'
import { useSearch } from '@tanstack/react-router'
import { useState } from 'react'
import { api, problemMessage } from '../api/client'
import { Button } from '../components/Button'
import { Plain } from '../components/Plain'
import { Permissions } from './Activate'
import auth from './Auth.module.css'
import styles from './Connect.module.css'
import { ProfilePicker, useSignedIn } from './connect'

export function Authorize() {
  const me = useSignedIn()
  const search = useSearch({ from: '/authorize' })
  const [profile, setProfile] = useState<number>()

  const app = useQuery({
    queryKey: ['authorize', search.client_id, search.redirect_uri],
    enabled: !!me,
    retry: false,
    queryFn: async () => {
      const { data, error } = await api.GET('/v1/authorize', {
        params: { query: { client_id: search.client_id, redirect_uri: search.redirect_uri } },
      })
      if (error) throw error
      return data
    },
  })

  const allow = useMutation({
    mutationFn: async () => {
      const { data, error } = await api.POST('/v1/authorize', {
        body: {
          client_id: search.client_id,
          redirect_uri: search.redirect_uri,
          profile_id: profile ?? me?.profiles[0]?.id ?? 0,
          state: search.state,
          code_challenge: search.code_challenge,
          code_challenge_method: search.code_challenge_method,
        },
      })
      if (error) throw error
      return data
    },
    onSuccess: (data) => window.location.assign(data.redirect_to),
  })

  const decline = () => {
    const to = new URL(search.redirect_uri)
    to.searchParams.set('error', 'access_denied')
    if (search.state) to.searchParams.set('state', search.state)
    window.location.assign(to.toString())
  }

  if (!me) return <Plain>{null}</Plain>

  return (
    <Plain>
      <div className={auth.slip}>
        <div className={auth.section}>
          {app.isError ? (
            <>
              <h1 className={auth.title}>That sign-in link didn’t work</h1>
              <p className={auth.quiet}>{problemMessage(app.error)}</p>
            </>
          ) : app.data ? (
            <>
              <h1 className={auth.title}>{app.data.app_name} wants to connect</h1>
              <Permissions />
              <ProfilePicker
                legend="Log plays from it to"
                profiles={me.profiles}
                value={profile ?? me.profiles[0]?.id ?? 0}
                onChange={setProfile}
              />
              <div className={auth.actions}>
                <Button variant="primary" aria-busy={allow.isPending} onClick={() => allow.mutate()}>
                  Connect {app.data.app_name}
                </Button>
                <Button variant="plain" onClick={decline}>
                  Not now
                </Button>
              </div>
              {allow.isError && <p className={`${auth.status} ${auth.error}`}>{problemMessage(allow.error)}</p>}
              <p className={styles.note}>You’ll go back to {app.data.app_name} once you choose.</p>
            </>
          ) : (
            <h1 className={auth.title}>Checking the app…</h1>
          )}
        </div>
      </div>
    </Plain>
  )
}
