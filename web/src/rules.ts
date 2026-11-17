import type { components, operations } from './api/schema'
import { countries, genres, languages, types } from './facts'

export type Rule = components['schemas']['ListRule']
export type Values = components['schemas']['RuleValues']
export type Span = components['schemas']['RuleRange']
export type Names = Record<string, string>

export const genreList = [
  'drama',
  'comedy',
  'documentary',
  'crime',
  'romance',
  'action',
  'thriller',
  'horror',
  'adventure',
  'fantasy',
  'science-fiction',
  'music',
  'mystery',
  'kids',
  'history',
  'biography',
  'western',
  'family',
  'war',
  'soap',
  'reality',
  'talk-and-game',
  'sport',
  'news',
] as const

export const typeList = [
  'animated',
  'anime',
  'miniseries',
  'tv-film',
  'short',
  'special',
  'event',
  'silent',
  'web',
] as const

export type Choice = 'any' | 'none' | 'off'

export function choiceOf(v: Values | undefined, value: string): Choice {
  if (v?.any?.includes(value)) return 'any'
  if (v?.none?.includes(value)) return 'none'
  return 'off'
}

const without = (list: string[] | undefined, value: string) => (list ?? []).filter((v) => v !== value)

function tidy(v: Values): Values | undefined {
  const out: Values = {}
  if (v.any?.length) out.any = v.any
  if (v.none?.length) out.none = v.none
  return out.any || out.none ? out : undefined
}

export function setChoice(v: Values | undefined, value: string, to: Choice): Values | undefined {
  const any = without(v?.any, value)
  const none = without(v?.none, value)
  if (to === 'any') any.push(value)
  if (to === 'none') none.push(value)
  return tidy({ any, none })
}

export const nextChoice: Record<Choice, Choice> = { off: 'any', any: 'none', none: 'off' }

export function setSpan(s: Span | undefined, end: 'from' | 'to', text: string): Span | undefined {
  const n = text.trim() === '' ? undefined : Number(text)
  const out: Span = { ...s, [end]: n !== undefined && Number.isFinite(n) ? Math.trunc(n) : undefined }
  if (out.from === undefined) delete out.from
  if (out.to === undefined) delete out.to
  return out.from === undefined && out.to === undefined ? undefined : out
}

export function forSaving(r: Rule): Rule {
  const { names: _, left_out: __, ...rest } = r
  return rest
}

export const unnamed = 'one with no name yet'

export type PreviewQuery = operations['previewRule']['parameters']['query']

export function previewQuery(r: Rule): NonNullable<PreviewQuery> {
  return {
    kind: r.kind,
    genre: r.genres?.any as NonNullable<PreviewQuery>['genre'],
    not_genre: r.genres?.none as NonNullable<PreviewQuery>['not_genre'],
    type: r.types?.any as NonNullable<PreviewQuery>['type'],
    not_type: r.types?.none as NonNullable<PreviewQuery>['not_type'],
    studio: r.studios?.any,
    not_studio: r.studios?.none,
    network: r.networks?.any,
    not_network: r.networks?.none,
    country: r.countries?.any,
    not_country: r.countries?.none,
    language: r.languages?.any,
    not_language: r.languages?.none,
    year_from: r.year?.from,
    year_to: r.year?.to,
    runtime_from: r.runtime?.from,
    runtime_to: r.runtime?.to,
    watched: r.watched,
    rated: r.rated,
    watchlist: r.watchlist,
  }
}

const or = (words: string[]) => new Intl.ListFormat('en-GB', { type: 'disjunction' }).format(words)

function values(v: Values | undefined, name: (s: string) => string) {
  const parts = []
  if (v?.any?.length) parts.push(or(v.any.map(name)))
  if (v?.none?.length) parts.push(`not ${or(v.none.map(name))}`)
  const out = parts.join(', ')
  return out ? out[0].toUpperCase() + out.slice(1) : undefined
}

function span(s: Span | undefined, unit = '') {
  if (s?.from !== undefined && s?.to !== undefined) return `${s.from} to ${s.to}${unit}`
  if (s?.from !== undefined) return `${s.from}${unit} or more`
  if (s?.to !== undefined) return `Up to ${s.to}${unit}`
  return undefined
}

function yes(v: boolean | undefined, said: string, unsaid: string) {
  return v === undefined ? undefined : v ? said : unsaid
}

export function describe(r: Rule, names: Names = {}): [string, string | undefined][] {
  const named = (q: string) => names[q] ?? unnamed
  const lower = (s: string | undefined) => s?.toLowerCase() ?? ''
  return [
    ['Kind', r.kind === 'movie' ? 'Films' : r.kind === 'show' ? 'Shows' : undefined],
    ['Genre', values(r.genres, (g) => lower(genres([g])))],
    ['Type', values(r.types, (t) => lower(types(r.kind ?? 'movie', [t])))],
    ['Studio', values(r.studios, named)],
    ['Network', values(r.networks, named)],
    ['Country', values(r.countries, (c) => countries([c]) ?? c)],
    ['Language', values(r.languages, (l) => languages([l]) ?? l)],
    ['Year', span(r.year)],
    ['Runtime', span(r.runtime, ' minutes')],
    ['Watched', yes(r.watched, 'Yes, by whoever’s looking', 'Not yet, by whoever’s looking')],
    ['Rated', yes(r.rated, 'Yes, by whoever’s looking', 'Not yet, by whoever’s looking')],
    [
      'Watchlist',
      yes(r.watchlist, 'On the watchlist of whoever’s looking', 'Not on the watchlist of whoever’s looking'),
    ],
  ]
}

export function isEmpty(r: Rule) {
  return describe(r).every(([, v]) => v === undefined)
}
