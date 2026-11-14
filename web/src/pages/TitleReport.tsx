import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { api, problemMessage } from '../api/client'
import type { components } from '../api/schema'
import { Button } from '../components/Button'
import { Field } from '../components/Field'
import { useWatching } from '../components/Shell'
import { Slip, SlipBody, SlipFoot } from '../components/Slip'
import { Option } from './List'
import own from './List.module.css'
import styles from './Lists.module.css'
import { goesOutside } from './MyLists'
import page from './Title.module.css'

type Report = components['schemas']['NewTitleReport']
type Reason = Report['reason']
type Provider = NonNullable<Report['provider']>
type Kind = 'movie' | 'show'

const providers: { value: Provider; label: string; only?: Kind }[] = [
  { value: 'imdb', label: 'IMDb' },
  { value: 'tmdb', label: 'TMDB' },
  { value: 'tvdb', label: 'TheTVDB', only: 'show' },
  { value: 'tvmaze', label: 'TVmaze', only: 'show' },
  { value: 'letterboxd', label: 'Letterboxd', only: 'movie' },
  { value: 'mal', label: 'MyAnimeList' },
  { value: 'anidb', label: 'AniDB' },
  { value: 'anilist', label: 'AniList' },
  { value: 'rottentomatoes', label: 'Rotten Tomatoes' },
  { value: 'metacritic', label: 'Metacritic' },
  { value: 'appletv', label: 'Apple TV' },
  { value: 'netflix', label: 'Netflix' },
  { value: 'primevideo', label: 'Prime Video' },
  { value: 'disneyplus', label: 'Disney+' },
  { value: 'plex', label: 'Plex' },
]

const reasons = (kind: Kind): { value: Reason; label: string }[] => [
  { value: 'name', label: 'The name' },
  { value: 'year', label: 'The year' },
  { value: 'kind', label: kind === 'movie' ? 'It’s a show, not a film' : 'It’s a film, not a show' },
  { value: 'link', label: 'A link to another site' },
  { value: 'duplicate', label: 'It’s on Tethr twice' },
]

export function SomethingWrong({ id, kind, name }: { id?: number; kind: Kind; name: string }) {
  const { profile } = useWatching()
  const [open, setOpen] = useState(false)
  const [sent, setSent] = useState(false)
  const [round, setRound] = useState(0)
  if (!id || !goesOutside(profile.kind)) return null
  const close = () => setOpen(false)
  return (
    <div className={page.wrong}>
      <button
        type="button"
        className={`${styles.textButton} ${own.quietAction}`}
        onClick={() => {
          setSent(false)
          setRound(round + 1)
          setOpen(true)
        }}
      >
        Something wrong?
      </button>
      <Slip
        open={open}
        onClose={close}
        step={sent ? 'sent' : 'form'}
        title={sent ? 'Thanks for telling us' : `What’s wrong with ${name}?`}
      >
        {sent ? (
          <>
            <SlipBody>
              <p>
                Tethr gets these details from Wikidata, so we’ll check them and fix them there if they’re wrong. Tethr
                usually picks up a fix a day or two later, though some changes wait a week.
              </p>
            </SlipBody>
            <SlipFoot>
              <Button variant="primary" onClick={close}>
                Done
              </Button>
            </SlipFoot>
          </>
        ) : (
          <ReportForm
            key={round}
            id={id}
            kind={kind}
            profileID={profile.id}
            onSent={() => setSent(true)}
            onClose={close}
          />
        )}
      </Slip>
    </div>
  )
}

function ReportForm({
  id,
  kind,
  profileID,
  onSent,
  onClose,
}: {
  id: number
  kind: Kind
  profileID: number
  onSent: () => void
  onClose: () => void
}) {
  const [reason, setReason] = useState<Reason>()
  const [provider, setProvider] = useState<Provider>()
  const [suggestion, setSuggestion] = useState('')
  const [note, setNote] = useState('')
  const send = useMutation({
    mutationFn: async (body: Report) => {
      const { error } = await api.POST('/v1/titles/{id}/reports', {
        params: { path: { id: String(id) }, query: { profile_id: profileID } },
        body,
      })
      if (error) throw error
    },
    onSuccess: onSent,
  })
  const pick = (r: Reason) => {
    setReason(r)
    setSuggestion('')
    send.reset()
  }
  const ready =
    !!reason && (reason !== 'link' || !!provider) && (reason !== 'duplicate' || !!suggestion.trim()) && !send.isPending
  const other: Kind = kind === 'movie' ? 'show' : 'movie'

  return (
    <>
      <SlipBody>
        <form
          id="title-report"
          className={own.form}
          onSubmit={(e) => {
            e.preventDefault()
            if (!ready || !reason) return
            send.mutate({
              reason,
              provider: reason === 'link' ? provider : undefined,
              suggestion: reason === 'kind' ? other : suggestion.trim() || undefined,
              note: note.trim() || undefined,
            })
          }}
        >
          <fieldset className={styles.options}>
            <legend className={own.legend}>What’s wrong?</legend>
            {reasons(kind).map((r) => (
              <Option key={r.value} name="reason" checked={r.value === reason} onChange={() => pick(r.value)}>
                {r.label}
              </Option>
            ))}
          </fieldset>
          {reason === 'name' && (
            <Field label="The right name (optional)" value={suggestion} onChange={setSuggestion} maxLength={500} />
          )}
          {reason === 'year' && (
            <Field
              label="The right year (optional)"
              value={suggestion}
              onChange={setSuggestion}
              inputMode="numeric"
              maxLength={4}
            />
          )}
          {reason === 'link' && (
            <>
              <label className={own.select}>
                <span className={styles.faint}>Site</span>
                <select value={provider ?? ''} onChange={(e) => setProvider(e.target.value as Provider)}>
                  <option value="" disabled>
                    Pick one
                  </option>
                  {providers
                    .filter((p) => !p.only || p.only === kind)
                    .map((p) => (
                      <option key={p.value} value={p.value}>
                        {p.label}
                      </option>
                    ))}
                </select>
              </label>
              <Field
                label="The right link or ID (optional)"
                hint={provider === 'tvdb' || provider === 'plex' ? 'Just the ID for this site.' : undefined}
                value={suggestion}
                onChange={setSuggestion}
                maxLength={500}
              />
            </>
          )}
          {reason === 'duplicate' && (
            <Field
              label="Link to the other one"
              hint="From Tethr, Wikidata, IMDb or TMDB, or a Wikidata ID like Q42."
              value={suggestion}
              onChange={setSuggestion}
              maxLength={500}
            />
          )}
          {reason && (
            <Field label="Anything that would help us (optional)" value={note} onChange={setNote} maxLength={500} />
          )}
          {send.isError && <p className={styles.error}>{problemMessage(send.error)}</p>}
        </form>
      </SlipBody>
      <SlipFoot>
        <Button variant="primary" type="submit" form="title-report" disabled={!ready}>
          Send the report
        </Button>
        <Button variant="text" onClick={onClose}>
          Cancel
        </Button>
      </SlipFoot>
    </>
  )
}
