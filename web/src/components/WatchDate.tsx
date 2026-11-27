import { useMutation } from '@tanstack/react-query'
import { createContext, type ReactNode, useContext, useRef, useState } from 'react'
import { api, problemMessage } from '../api/client'
import type { components } from '../api/schema'
import { date } from '../format'
import { dated, moved, problem, type When } from '../watchDate'
import { Button } from './Button'
import { Field } from './Field'
import { Slip, SlipBody, SlipFoot } from './Slip'
import { useToast } from './Toast'
import styles from './WatchDate.module.css'

type ItemRef = components['schemas']['ItemRef']

type Logged = {
  items: ItemRef[]
  ids: number[]
  released?: string | boolean
  profileID: number
  done: () => void
}

const PickContext = createContext<(l: Logged) => void>(() => {})

const today = () => {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

export function WatchDates({ children }: { children: ReactNode }) {
  const [logged, setLogged] = useState<Logged | null>(null)
  const [open, setOpen] = useState(false)
  const pick = (l: Logged) => {
    setLogged(l)
    setOpen(true)
  }
  return (
    <PickContext.Provider value={pick}>
      {children}
      <Slip open={open} onClose={() => setOpen(false)} title="When did you watch it?">
        {logged && <Choose key={logged.ids.join()} logged={logged} close={() => setOpen(false)} />}
      </Slip>
    </PickContext.Provider>
  )
}

function Choose({ logged, close }: { logged: Logged; close: () => void }) {
  const toast = useToast()
  const [kind, setKind] = useState<When['kind']>(logged.released ? 'release' : 'day')
  const [day, setDay] = useState(today)
  const [time, setTime] = useState('')
  const [tried, setTried] = useState(false)
  const when: When = kind === 'day' ? { kind, day, time } : { kind }
  const wrong = tried ? problem(when) : undefined
  const one = logged.items.length === 1

  const posted = useRef(new Map<number, boolean>())
  const removed = useRef(new Set<number>())
  const save = useMutation({
    mutationFn: async () => {
      const query = { profile_id: logged.profileID }
      const watches = dated(logged.items, when)
      for (const [i, id] of logged.ids.entries()) {
        if (!posted.current.has(i)) {
          const { data, error } = await api.POST('/v1/history', { params: { query }, body: { items: [watches[i]] } })
          if (error) throw error
          posted.current.set(i, !!data.ids?.length)
        }
        if (posted.current.get(i) && !removed.current.has(i)) {
          const { error, response } = await api.DELETE('/v1/history/{id}', { params: { path: { id }, query } })
          if (error && response.status !== 404) throw error
          removed.current.add(i)
        }
      }
      return [...posted.current.values()].filter((added) => !added).length
    },
    onSuccess: (kept) => {
      logged.done()
      close()
      toast({
        text: kept
          ? already(kept, logged.ids.length)
          : moved(when, typeof logged.released === 'string' ? logged.released : undefined),
      })
    },
    onError: logged.done,
  })

  const submit = () => {
    setTried(true)
    if (!problem(when)) save.mutate()
  }

  const choice = (k: When['kind'], label: string, extra?: ReactNode) => (
    <label className={k === kind ? `${styles.choice} ${styles.chosen}` : styles.choice}>
      <input type="radio" name="when" checked={k === kind} onChange={() => setKind(k)} />
      {label}
      {extra}
    </label>
  )

  return (
    <>
      <SlipBody>
        <p className={styles.note}>
          {one ? 'It’s logged as just now.' : `All ${logged.items.length} are logged as just now.`} Pick when it really
          was.
        </p>
        <fieldset className={styles.choices} disabled={save.isPending || posted.current.size > 0}>
          <legend className={styles.hidden}>When you watched it</legend>
          {logged.released &&
            choice(
              'release',
              'On release',
              <span className={styles.code}>
                {typeof logged.released === 'string' ? date(logged.released) : 'each on the day it aired'}
              </span>,
            )}
          {choice('day', 'Another day')}
          {choice('unknown', 'Don’t know', <span className={styles.code}>counts, with no date</span>)}
        </fieldset>
        <div className={kind === 'day' ? `${styles.reveal} ${styles.revealed}` : styles.reveal} inert={kind !== 'day'}>
          <div>
            <div className={styles.when}>
              <Field label="Day" type="date" value={day} max={today()} onChange={setDay} error={wrong} />
              <Field label="Time" type="time" value={time} onChange={setTime} hint="If you remember it" />
            </div>
          </div>
        </div>
        {save.isError && (
          <p className={styles.error}>
            {posted.current.size ? 'Only some of them moved. Try again to move the rest.' : problemMessage(save.error)}
          </p>
        )}
      </SlipBody>
      <SlipFoot>
        <Button variant="primary" onClick={submit} aria-busy={save.isPending} disabled={save.isPending}>
          Change the date
        </Button>
        <Button variant="text" onClick={close}>
          Keep it as now
        </Button>
      </SlipFoot>
    </>
  )
}

const already = (kept: number, all: number) =>
  all === 1
    ? 'You’d already logged it on that date, so the play from just now stays too.'
    : `${kept} of ${all} were already logged on that date, so their plays from just now stay too.`

export function useLogWatch(profileID: number, done: () => void) {
  const toast = useToast()
  const pick = useContext(PickContext)
  return useMutation({
    mutationFn: async ({ items }: { items: ItemRef[]; label: string; released?: string | boolean }) => {
      const { data, error } = await api.POST('/v1/history', {
        params: { query: { profile_id: profileID } },
        body: { items: items.map((item) => ({ item })) },
      })
      if (error) throw error
      return data.ids ?? []
    },
    onSuccess: (ids, { items, label, released }) => {
      done()
      toast({
        text: `Ticked off ${label}.`,
        action:
          ids.length === items.length
            ? { label: 'Another date', run: () => pick({ items, ids, released, profileID, done }) }
            : undefined,
      })
    },
  })
}
