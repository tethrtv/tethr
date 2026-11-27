import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { type ReactNode, useEffect, useId, useState } from 'react'
import { api, problemMessage } from '../api/client'
import type { components } from '../api/schema'
import { Button } from '../components/Button'
import { Cover } from '../components/Cover'
import { Field } from '../components/Field'
import { useWatching } from '../components/Shell'
import { TitleLink } from '../components/TitleLink'
import { countries, genres, languages, types } from '../facts'
import { plural } from '../format'
import { useNear } from '../near'
import {
  type Choice,
  choiceOf,
  forSaving,
  genreList,
  isEmpty,
  type Names,
  nextChoice,
  previewQuery,
  type Rule,
  type Span,
  setChoice,
  setSpan,
  typeList,
  unnamed,
  type Values,
} from '../rules'
import { useDetails } from '../tmdb'
import { Option } from './List'
import own from './List.module.css'
import styles from './Lists.module.css'
import { listParams } from './MyLists'
import page from './RuleList.module.css'

const countryCodes =
  'AD AE AF AG AL AM AO AR AT AU AZ BA BB BD BE BF BG BH BI BJ BN BO BR BS BT BW BY BZ CA CD CF CG CH CI CL CM CN CO CR CU CV CY CZ DE DJ DK DM DO DZ EC EE EG ER ES ET FI FJ FM FR GA GB GD GE GH GM GN GQ GR GT GW GY HK HN HR HT HU ID IE IL IN IQ IR IS IT JM JO JP KE KG KH KI KM KN KP KR KW KZ LA LB LC LI LK LR LS LT LU LV LY MA MC MD ME MG MH MK ML MM MN MO MR MT MU MV MW MX MY MZ NA NE NG NI NL NO NP NR NZ OM PA PE PG PH PK PL PR PS PT PW PY QA RO RS RU RW SA SB SC SD SE SG SI SK SL SM SN SO SR SS ST SV SY SZ TD TG TH TJ TL TM TN TO TR TT TV TW TZ UA UG US UY UZ VA VC VE VN VU WS XK YE ZA ZM ZW SU YU CS DD'.split(
    ' ',
  )

const languageCodes =
  'af am ar as az be bg bn bo bs ca cs cy da de dz el en es et eu fa fi fil fo fr ga gl gu he hi hr ht hu hy id is it iu ja jv ka kk km kn ko ku ky la lb lo lt lv mi mk ml mn mr ms mt my nb ne nl nn no or pa pl ps pt qu rm ro ru rw sa sd si sk sl sm so sq sr sv sw ta te tg th ti tk tl tr tt ug uk ur uz vi wo xh yi yo yue zh zu'.split(
    ' ',
  )

const byName = (codes: string[], name: (c: string) => string | undefined) =>
  codes.map((c) => ({ code: c, name: name(c) ?? c })).sort((a, b) => a.name.localeCompare(b.name, 'en-GB'))

const countryNames = byName(countryCodes, (c) => countries([c]))
const languageNames = byName(languageCodes, (l) => languages([l]))

const kinds = [
  { value: undefined, label: 'Films and shows' },
  { value: 'movie', label: 'Films' },
  { value: 'show', label: 'Shows' },
] as const

export type RuleSearch = {
  list?: number
  kind?: 'movie' | 'show'
  genre?: string
  type?: string
  studio?: string
  network?: string
}

function fromSearch(s: RuleSearch): Rule {
  const r: Rule = {}
  if (s.kind) r.kind = s.kind
  if (s.genre) r.genres = { any: [s.genre] }
  if (s.type) r.types = { any: [s.type] }
  if (s.studio) r.studios = { any: [s.studio] }
  if (s.network) r.networks = { any: [s.network] }
  return r
}

function useSettled<T>(value: T, ms: number) {
  const [settled, setSettled] = useState(value)
  useEffect(() => {
    const t = setTimeout(() => setSettled(value), ms)
    return () => clearTimeout(t)
  }, [value, ms])
  return settled
}

export function RuleListPage({ search }: { search: RuleSearch }) {
  const { profile } = useWatching()
  const editing = useQuery({
    queryKey: ['list', search.list, profile.id, 'rule'],
    enabled: !!search.list,
    queryFn: async () => {
      const { data, error } = await api.GET('/v1/lists/{id}', {
        params: { path: { id: search.list ?? 0 }, query: { profile_id: profile.id } },
      })
      if (error) throw error
      return data
    },
  })
  if (search.list) {
    if (editing.isError) return <p className={styles.error}>{problemMessage(editing.error)}</p>
    if (!editing.data) return null
    if (!editing.data.rule || editing.data.access !== 'mine') {
      return (
        <p className={styles.error}>
          Only the profile that made this list can change its rule, and only lists made from a rule have one.
        </p>
      )
    }
  }
  const list = editing.data
  return (
    <RuleForm
      key={list?.id ?? 'new'}
      start={list?.rule ?? fromSearch(search)}
      list={list ? { id: list.id, name: list.name, leftOut: list.rule?.left_out?.length ?? 0 } : undefined}
    />
  )
}

function RuleForm({ start, list }: { start: Rule; list?: { id: number; name: string; leftOut: number } }) {
  const { profile } = useWatching()
  const queries = useQueryClient()
  const navigate = useNavigate()
  const [rule, setRule] = useState<Rule>(forSaving(start))
  const [names, setNames] = useState<Names>(start.names ?? {})
  const [name, setName] = useState('')
  const [putBack, setPutBack] = useState(false)
  const asked = useSettled(rule, 300)
  const leaving = list && !putBack ? list.id : undefined
  const preview = useQuery({
    queryKey: ['rule preview', profile.id, asked, leaving],
    placeholderData: keepPreviousData,
    queryFn: async () => {
      const { data, error } = await api.GET('/v1/lists/preview', {
        params: { query: { ...previewQuery(asked), list: leaving, profile_id: profile.id, limit: 24 } },
      })
      if (error) throw error
      return data
    },
  })
  const shown = preview.data?.names
  useEffect(() => {
    if (shown) setNames((n) => ({ ...shown, ...n }))
  }, [shown])

  const save = useMutation({
    mutationFn: async () => {
      const params = { query: { profile_id: profile.id } }
      if (list) {
        const { data, error } = await api.PATCH('/v1/lists/{id}', {
          params: { ...params, path: { id: list.id } },
          body: { rule: putBack ? { ...rule, left_out: [] } : rule },
        })
        if (error) throw error
        return data
      }
      const { data, error } = await api.POST('/v1/lists', { params, body: { name: name.trim(), rule } })
      if (error) throw error
      return data
    },
    onSuccess: (l) => {
      queries.invalidateQueries({ queryKey: ['lists'] })
      queries.invalidateQueries({ queryKey: ['list', l.id] })
      navigate({ to: '/@{$username}/lists/$slug', params: listParams(l) })
    },
  })

  const change = (key: keyof Rule) => (v: Rule[keyof Rule]) =>
    setRule((r) => {
      const next = { ...r, [key]: v }
      if (v === undefined) delete next[key]
      return next
    })
  const matched = preview.data?.matched_count

  return (
    <div className={styles.page}>
      <header className={styles.head}>
        <h1 className={styles.title}>{list ? `The rule for ${list.name}` : 'A list that fills itself'}</h1>
        <p className={styles.lead}>
          Pick what goes on it. Tethr keeps it up to date as new films and shows come in, and puts the best known first.
        </p>
      </header>

      <div className={page.layout}>
        <form
          id="rule"
          className={page.form}
          onSubmit={(e) => {
            e.preventDefault()
            if (list || name.trim()) save.mutate()
          }}
        >
          <Row label="Kind">
            <fieldset className={styles.options}>
              <legend className={styles.hidden}>Kind</legend>
              {kinds.map((k) => (
                <Option
                  key={k.label}
                  name="kind"
                  checked={rule.kind === k.value}
                  onChange={() => change('kind')(k.value)}
                >
                  {k.label}
                </Option>
              ))}
            </fieldset>
          </Row>
          <Row label="Genre" hint="Tap once to include, again to exclude.">
            <Chips values={rule.genres} all={genreList} name={(g) => genres([g]) ?? g} onChange={change('genres')} />
          </Row>
          <Row label="Type">
            <Chips
              values={rule.types}
              all={typeList}
              name={(t) => types(rule.kind ?? 'movie', [t]) ?? t}
              onChange={change('types')}
            />
          </Row>
          <Row label="Studio or network">
            <Companies
              rule={rule}
              names={names}
              onName={(qid, n) => setNames((all) => ({ ...all, [qid]: n }))}
              onStudios={change('studios')}
              onNetworks={change('networks')}
            />
          </Row>
          <Row label="Country">
            <Picked values={rule.countries} all={countryNames} add="Add a country" onChange={change('countries')} />
          </Row>
          <Row label="Language">
            <Picked values={rule.languages} all={languageNames} add="Add a language" onChange={change('languages')} />
          </Row>
          <Row label="Year">
            <Between span={rule.year} label="Year" onChange={change('year')} />
          </Row>
          <Row label="Runtime" hint="In minutes. Titles with no known runtime never match.">
            <Between span={rule.runtime} label="Runtime" onChange={change('runtime')} />
          </Row>
          <Row
            label="For whoever’s looking"
            hint="Each person who sees the list gets their own answer. Anyone signed out sees it without these."
          >
            <YesNo
              label="Watched"
              value={rule.watched}
              words={['Watched', 'Not watched']}
              onChange={change('watched')}
            />
            <YesNo label="Rated" value={rule.rated} words={['Rated', 'Not rated']} onChange={change('rated')} />
            <YesNo
              label="Watchlist"
              value={rule.watchlist}
              words={['On the watchlist', 'Not on it']}
              onChange={change('watchlist')}
            />
          </Row>
          {list && list.leftOut > 0 && (
            <Row label="Taken out">
              <label className={putBack ? `${styles.option} ${styles.chosen}` : styles.option}>
                <input type="checkbox" checked={putBack} onChange={(e) => setPutBack(e.target.checked)} />
                Put back the {plural(list.leftOut, 'thing')} taken out
              </label>
            </Row>
          )}
        </form>

        <section className={page.preview} aria-live="polite">
          <h2 className={styles.sectionTitle}>
            {matched === undefined
              ? 'Looking'
              : matched === 0
                ? 'Nothing matches yet'
                : matched > 1000
                  ? `${plural(matched, 'match', 'matches')}, the list shows the best known 1,000`
                  : plural(matched, 'match', 'matches')}
          </h2>
          {preview.isError && <p className={styles.error}>{problemMessage(preview.error)}</p>}
          {isEmpty(rule) && (
            <p className={styles.quiet}>With nothing picked, it’s simply the best known of everything.</p>
          )}
          <div className={page.save}>
            {!list && <Field label="Name" value={name} onChange={setName} maxLength={100} form="rule" required />}
            {save.isError && <p className={styles.error}>{problemMessage(save.error)}</p>}
            <p className={own.actions}>
              <Button variant="primary" type="submit" form="rule" disabled={(!list && !name.trim()) || save.isPending}>
                {list ? 'Save the rule' : 'Make the list'}
              </Button>
              {list && (
                <Button variant="text" onClick={() => history.back()}>
                  Cancel
                </Button>
              )}
            </p>
          </div>
          <ol className={page.grid}>
            {preview.data?.items.map((i) => (
              <Tile key={i.id} item={i} />
            ))}
          </ol>
        </section>
      </div>
    </div>
  )
}

function Row({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
  return (
    <div className={page.row}>
      <div>
        <p className={page.label}>{label}</p>
        {hint && <p className={styles.faint}>{hint}</p>}
      </div>
      <div className={page.control}>{children}</div>
    </div>
  )
}

function Chip({
  choice,
  label,
  onClick,
  onRemove,
}: {
  choice: Choice
  label: string
  onClick: () => void
  onRemove?: () => void
}) {
  const cls =
    choice === 'any'
      ? `${styles.option} ${styles.chosen}`
      : choice === 'none'
        ? `${styles.option} ${page.left}`
        : styles.option
  return (
    <span className={page.chip}>
      <button type="button" className={cls} aria-pressed={choice !== 'off'} onClick={onClick}>
        {choice === 'none' ? `Not ${label.toLowerCase()}` : label}
      </button>
      {onRemove && (
        <button type="button" className={page.drop} aria-label={`Remove ${label} from the rule`} onClick={onRemove}>
          ×
        </button>
      )}
    </span>
  )
}

function Chips({
  values,
  all,
  name,
  onChange,
}: {
  values?: Values
  all: readonly string[]
  name: (v: string) => string
  onChange: (v?: Values) => void
}) {
  return (
    <div className={styles.options}>
      {all.map((v) => {
        const choice = choiceOf(values, v)
        return (
          <Chip
            key={v}
            choice={choice}
            label={name(v)}
            onClick={() => onChange(setChoice(values, v, nextChoice[choice]))}
          />
        )
      })}
    </div>
  )
}

function Chosen({
  values,
  name,
  onChange,
}: {
  values?: Values
  name: (v: string) => string
  onChange: (v?: Values) => void
}) {
  const picked = [...(values?.any ?? []), ...(values?.none ?? [])]
  if (!picked.length) return null
  return (
    <div className={styles.options}>
      {picked.map((v) => {
        const choice = choiceOf(values, v)
        return (
          <Chip
            key={v}
            choice={choice}
            label={name(v)}
            onClick={() => onChange(setChoice(values, v, choice === 'any' ? 'none' : 'any'))}
            onRemove={() => onChange(setChoice(values, v, 'off'))}
          />
        )
      })}
    </div>
  )
}

function Picked({
  values,
  all,
  add,
  onChange,
}: {
  values?: Values
  all: { code: string; name: string }[]
  add: string
  onChange: (v?: Values) => void
}) {
  const named = (c: string) => all.find((a) => a.code === c)?.name ?? c
  return (
    <>
      <Chosen values={values} name={named} onChange={onChange} />
      <label className={own.select}>
        <span className={styles.hidden}>{add}</span>
        <select value="" onChange={(e) => e.target.value && onChange(setChoice(values, e.target.value, 'any'))}>
          <option value="">{add}</option>
          {all
            .filter((a) => choiceOf(values, a.code) === 'off')
            .map((a) => (
              <option key={a.code} value={a.code}>
                {a.name}
              </option>
            ))}
        </select>
      </label>
    </>
  )
}

function Companies({
  rule,
  names,
  onName,
  onStudios,
  onNetworks,
}: {
  rule: Rule
  names: Names
  onName: (qid: string, name: string) => void
  onStudios: (v?: Values) => void
  onNetworks: (v?: Values) => void
}) {
  const [q, setQ] = useState('')
  const asked = useSettled(q.trim(), 250)
  const found = useQuery({
    queryKey: ['companies', asked],
    enabled: asked.length >= 2,
    placeholderData: keepPreviousData,
    queryFn: async () => {
      const { data, error } = await api.GET('/v1/companies', { params: { query: { q: asked } } })
      if (error) throw error
      return data
    },
  })
  const named = (qid: string) => names[qid] ?? unnamed
  const offers = (found.data ?? []).flatMap((c) => [
    ...(c.studio_of > 0 && choiceOf(rule.studios, c.wikidata) === 'off'
      ? [
          {
            key: `s${c.wikidata}`,
            label: `${c.name}, studio of ${plural(c.studio_of, 'title')}`,
            pick: () => onStudios(setChoice(rule.studios, c.wikidata, 'any')),
            c,
          },
        ]
      : []),
    ...(c.network_of > 0 && choiceOf(rule.networks, c.wikidata) === 'off'
      ? [
          {
            key: `n${c.wikidata}`,
            label: `${c.name}, network of ${plural(c.network_of, 'title')}`,
            pick: () => onNetworks(setChoice(rule.networks, c.wikidata, 'any')),
            c,
          },
        ]
      : []),
  ])
  return (
    <>
      <Chosen values={rule.studios} name={(v) => `${named(v)} (studio)`} onChange={onStudios} />
      <Chosen values={rule.networks} name={(v) => `${named(v)} (network)`} onChange={onNetworks} />
      <input
        className={own.find}
        type="search"
        placeholder="Find a studio or network"
        aria-label="Find a studio or network"
        value={q}
        onChange={(e) => setQ(e.target.value)}
      />
      {asked.length >= 2 && found.data && (
        <ul className={page.found}>
          {offers.length === 0 && <li className={styles.faint}>Nothing by that name.</li>}
          {offers.map((o) => (
            <li key={o.key}>
              <button
                type="button"
                className={styles.textButton}
                onClick={() => {
                  onName(o.c.wikidata, o.c.name)
                  o.pick()
                  setQ('')
                }}
              >
                {o.label}
              </button>
            </li>
          ))}
        </ul>
      )}
    </>
  )
}

function Between({ span, label, onChange }: { span?: Span; label: string; onChange: (s?: Span) => void }) {
  const id = useId()
  return (
    <span className={page.between}>
      <label htmlFor={`${id}-from`} className={styles.hidden}>
        {label} from
      </label>
      <input
        id={`${id}-from`}
        type="number"
        inputMode="numeric"
        min={0}
        placeholder="From"
        value={span?.from ?? ''}
        onChange={(e) => onChange(setSpan(span, 'from', e.target.value))}
      />
      <span className={styles.faint}>to</span>
      <label htmlFor={`${id}-to`} className={styles.hidden}>
        {label} to
      </label>
      <input
        id={`${id}-to`}
        type="number"
        inputMode="numeric"
        min={0}
        placeholder="To"
        value={span?.to ?? ''}
        onChange={(e) => onChange(setSpan(span, 'to', e.target.value))}
      />
    </span>
  )
}

function YesNo({
  label,
  value,
  words,
  onChange,
}: {
  label: string
  value?: boolean
  words: [string, string]
  onChange: (v?: boolean) => void
}) {
  const options = [
    { v: undefined, text: 'Either' },
    { v: true, text: words[0] },
    { v: false, text: words[1] },
  ]
  return (
    <fieldset className={styles.options}>
      <legend className={styles.hidden}>{label}</legend>
      {options.map((o) => (
        <Option key={o.text} name={label} checked={value === o.v} onChange={() => onChange(o.v)}>
          {o.text}
        </Option>
      ))}
    </fieldset>
  )
}

function Tile({ item: i }: { item: components['schemas']['ListItem'] }) {
  const [ref, near] = useNear<HTMLLIElement>()
  const title = useDetails(i.item.type === 'movie' ? 'movie' : 'show', i.item.title.ids.tmdb, near)
  const name = title?.name ?? i.item.title.name ?? 'Untitled'
  return (
    <li ref={ref} className={page.tile}>
      <TitleLink type={i.item.type} slug={i.item.title.slug} className={page.tileLink}>
        <Cover id={i.item.title.ids.tethr ?? 0} name={name} poster={title?.poster} />
        <span className={page.tileName}>{name}</span>
        <span className={styles.faint}>{title?.year ?? i.item.title.year}</span>
      </TitleLink>
    </li>
  )
}
