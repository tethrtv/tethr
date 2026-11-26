import type { Film } from './letterboxd'
import { parseCSV } from './letterboxd'

export type ShowItem = { tvdb?: number; label: string; at?: string }
export type Episode = ShowItem & { season?: number; number?: number }

export type IDFilm = { imdb: string; label: string; plays: (string | undefined)[]; listedAt?: string }

export type TVTime = {
  films: Film[]
  idFilms: IDFilm[]
  episodes: Episode[]
  later: ShowItem[]
  dropped: ShowItem[]
  ratings: (ShowItem & { rating: number })[]
  unread: number
  undated: number
}

const history = ['tracking-prod-records-v2.csv', 'tracking-prod-records.csv']

const outFile = /^tvtime-(series|movies)-[\d-]+\.json$/

export const unreadableOut =
  'This looks like a TV Time Out export, but Tethr couldn’t read its files, so nothing was sent. Check it’s the zip the extension saved.'

// Only these are opened: the rest of the export holds password hashes, tokens, IP addresses and comments.
const wanted = new Set([...history, 'tv_show_rate.csv'])

const markers = [
  /^tracking-prod-records/,
  /^tv_show_rate\.csv$/,
  /^seen_episode/,
  /^followed_tv_show\.csv$/,
  /^user_tv_show_data\.csv$/,
  /^access_token\.csv$/,
  /^auth-prod-login\.csv$/,
  outFile,
]

const base = (path: string) => path.slice(path.lastIndexOf('/') + 1).toLowerCase()

export const isTVTime = (names: string[]) =>
  names.some((n) => n.toLowerCase().startsWith('gdpr-data/') || markers.some((m) => m.test(base(n))))

export const tvtimeFile = (name: string) => wanted.has(base(name)) || outFile.test(base(name))

export const hasHistory = (files: Map<string, string>) =>
  [...files.keys()].some((p) => history.includes(base(p)) || outFile.test(base(p)))

function rows(files: Map<string, string>, name: string) {
  const path = [...files.keys()].find((p) => base(p) === name)
  const [head, ...body] = parseCSV(path ? (files.get(path) ?? '') : '')
  if (!head) return []
  const keys = head.map((h) => h.replace(/^﻿/, '').trim())
  return body.map((r) => Object.fromEntries(keys.map((k, i) => [k, (r[i] ?? '').trim()])))
}

const bulk = (v?: string) => !!v && v !== '0' && v.toLowerCase() !== 'false'

const yes = (v?: string) => v === 'true' || v === 'True' || v === 'TRUE' || v === '1'

const whole = (v?: string) => {
  const n = Number.parseInt(v ?? '', 10)
  return Number.isFinite(n) && n >= 0 ? n : undefined
}

const id = (v?: string) => {
  const n = Math.round(Number.parseFloat(v ?? ''))
  return n > 0 && n < 2 ** 31 ? n : undefined
}

function when(v?: string): string | undefined {
  if (!v) return undefined
  const epoch = Number(v)
  const d = Number.isFinite(epoch)
    ? new Date(epoch > 1e14 ? epoch / 1000 : epoch > 1e11 ? epoch : epoch * 1000)
    : new Date(`${v.replace(' ', 'T')}${/[zZ]|[+-]\d\d:?\d\d$/.test(v) ? '' : 'Z'}`)
  const year = d.getUTCFullYear()
  return Number.isNaN(d.getTime()) || year < 1990 || d.getTime() > Date.now() + 864e5 ? undefined : d.toISOString()
}

const episodeLabel = (show: string, season?: number, number?: number) =>
  `${show || 'Unknown show'} ${season ?? '?'}x${number === undefined ? '?' : String(number).padStart(2, '0')}`

const sameWatch = 10 * 60 * 1000

export function readTVTime(files: Map<string, string>): TVTime {
  if ([...files.keys()].some((p) => outFile.test(base(p)))) return readTVTimeOut(files)
  let unread = 0
  let undated = 0
  const date = (...values: (string | undefined)[]) => {
    const given = values.filter(Boolean)
    const at = given.map(when).find(Boolean)
    if (!at && given.length) undated++
    return at
  }

  const episodes: Episode[] = []
  const newer = new Map<string, number[]>()
  const ep = (e: Episode) => `${e.tvdb}|${e.season}|${e.number}`
  const shows = new Map<string, ShowItem & { later: boolean; dropped: boolean }>()
  const films = new Map<string, Film>()
  const film = (name: string, released?: string) => {
    const year = whole(released?.slice(0, 4))
    const known = year && year > 1800 ? year : undefined
    const key = `${name}\n${known ?? ''}`
    let f = films.get(key)
    if (!f) {
      f = { name, year: known, plays: [] }
      films.set(key, f)
    }
    return f
  }

  for (const r of rows(files, 'tracking-prod-records-v2.csv')) {
    const key = r.key || ''
    const gsi = r.gsi || ''
    const tvdb = id(r.s_id)
    if (/^(re)?watch-episode-/.test(key) || /^(re)?watch-episode-/.test(gsi)) {
      // Older rows leave ep_id empty and put the numbers in season_number and episode_number instead.
      const legacy = !r.ep_id && !!r.season_number
      const season = whole(legacy ? r.season_number : r.s_no)
      const number = whole(legacy ? r.episode_number : r.ep_no)
      const e = {
        tvdb,
        season,
        number,
        label: episodeLabel(r.series_name, season, number),
        at: bulk(r.bulk_type) ? undefined : date(r.created_at),
      }
      episodes.push(e)
      if (tvdb) newer.set(ep(e), [...(newer.get(ep(e)) ?? []), e.at ? Date.parse(e.at) : Number.NaN])
    } else if (key.startsWith('user-series-')) {
      const label = r.series_name || 'Unknown show'
      shows.set(tvdb ? `${tvdb}` : `${label}\n${shows.size}`, {
        tvdb,
        label,
        later: yes(r.is_for_later),
        dropped: yes(r.is_archived),
        at: when(r.followed_at) ?? when(r.created_at),
      })
    } else if (/^(re)?watch-/.test(key) || /^(re)?watch-/.test(gsi)) {
      unread++
    }
  }

  for (const r of rows(files, 'tracking-prod-records.csv')) {
    const kind = r.entity_type
    const type = r.type
    const watched = type === 'watch' || type === 'rewatch'
    if (kind === 'episode' && watched) {
      const season = whole(r.season_number)
      const number = whole(r.episode_number)
      const e = {
        tvdb: id(r.series_id),
        season,
        number,
        label: episodeLabel(r.series_name, season, number),
        at: date(r.watch_date, r.created_at),
      }
      // Both files can hold the same watch a few seconds apart, so a play the newer file already has is left out.
      const plays = e.tvdb ? newer.get(ep(e)) : undefined
      const at = e.at ? Date.parse(e.at) : Number.NaN
      const twin = plays?.findIndex((t) => (Number.isNaN(at) ? Number.isNaN(t) : Math.abs(t - at) <= sameWatch)) ?? -1
      if (plays && twin >= 0) plays.splice(twin, 1)
      else episodes.push(e)
    } else if (kind === 'movie' && r.movie_name) {
      const f = film(r.movie_name, r.release_date)
      if (watched) f.plays.push({ at: date(r.watch_date, r.created_at) })
      else if (type === 'towatch') f.listedAt = when(r.created_at) ?? new Date().toISOString()
    } else if (watched) {
      unread++
    }
  }

  const rated = rows(files, 'tv_show_rate.csv').map((r) => ({
    r,
    stars: Number.parseFloat(r.rate || r.rating || r.value || r.score),
  }))
  const scale = Math.max(0, ...rated.map(({ stars }) => stars || 0)) > 5 ? 1 : 2
  const ratings: TVTime['ratings'] = []
  for (const { r, stars } of rated) {
    const rating = Math.round(stars * scale)
    if (!(rating >= 1 && rating <= 10)) continue
    const tvdb = id(r.tv_show_id || r.show_id || r.series_id || r.tvdb_id)
    const name = r.tv_show_name || r.series_name || (tvdb && shows.get(`${tvdb}`)?.label) || 'Unknown show'
    ratings.push({ tvdb, label: name, rating, at: when(r.updated_at) ?? when(r.created_at) })
  }

  const all = [...shows.values()]
  return {
    films: [...films.values()].filter((f) => f.plays.length || f.listedAt),
    idFilms: [],
    episodes,
    later: all.filter((s) => s.later && !s.dropped).map(({ tvdb, label, at }) => ({ tvdb, label, at })),
    dropped: all.filter((s) => s.dropped).map(({ tvdb, label }) => ({ tvdb, label })),
    ratings,
    unread,
    undated,
  }
}

type OutIDs = { tvdb?: number | null; imdb?: string | null }

type OutEpisode = {
  number?: number
  is_watched?: boolean
  isWatched?: boolean
  watched_at?: string | null
  watchedAt?: string | null
}

type OutShow = {
  id?: OutIDs
  tvdbId?: number
  title?: string
  status?: string
  created_at?: string
  createdAt?: string
  seasons?: { number?: number; episodes?: OutEpisode[] }[]
}

type OutMovie = OutEpisode & {
  id?: OutIDs
  tvdbId?: number
  imdbId?: string | null
  title?: string
  year?: number
  created_at?: string
  createdAt?: string
}

function outList<T>(files: Map<string, string>, kind: string): T[] {
  const path = [...files.keys()].find((p) => base(p).startsWith(`tvtime-${kind}-`) && outFile.test(base(p)))
  if (!path) return []
  let parsed: unknown
  try {
    parsed = JSON.parse(files.get(path) ?? '')
  } catch {
    throw new Error(unreadableOut)
  }
  if (!Array.isArray(parsed)) throw new Error(unreadableOut)
  return parsed as T[]
}

function readTVTimeOut(files: Map<string, string>): TVTime {
  try {
    return readOut(files)
  } catch {
    throw new Error(unreadableOut)
  }
}

function readOut(files: Map<string, string>): TVTime {
  let undated = 0
  const date = (v?: string | null) => {
    const at = when(v ?? undefined)
    if (v && !at) undated++
    return at
  }
  // Rewatches come only as a count with no dates, and Tethr keeps an undated play only when there's no other, so each comes across once.
  const plays = (e: OutEpisode) => ((e.is_watched ?? e.isWatched) ? [date(e.watched_at ?? e.watchedAt)] : [])

  const episodes: Episode[] = []
  const later: ShowItem[] = []
  const dropped: ShowItem[] = []
  for (const show of outList<OutShow>(files, 'series')) {
    const tvdb = id(String(show.id?.tvdb ?? show.tvdbId ?? ''))
    const label = show.title || 'Unknown show'
    if (show.status === 'stopped') dropped.push({ tvdb, label })
    else if (show.status === 'not_started_yet') later.push({ tvdb, label, at: when(show.created_at ?? show.createdAt) })
    for (const season of show.seasons ?? []) {
      for (const e of season.episodes ?? []) {
        const s = whole(String(season.number ?? ''))
        const n = whole(String(e.number ?? ''))
        for (const at of plays(e)) episodes.push({ tvdb, season: s, number: n, label: episodeLabel(label, s, n), at })
      }
    }
  }

  const films: Film[] = []
  const idFilms: IDFilm[] = []
  for (const m of outList<OutMovie>(files, 'movies')) {
    const name = m.title || 'Unknown film'
    const watched = plays(m)
    const listedAt = watched.length
      ? undefined
      : (when(m.created_at ?? m.createdAt) ??
        when(m.watched_at ?? m.watchedAt ?? undefined) ??
        new Date().toISOString())
    const imdb = m.id?.imdb ?? m.imdbId
    const year = m.year && m.year > 1800 ? m.year : undefined
    if (imdb && /^tt\d{7,9}$/.test(imdb))
      idFilms.push({ imdb, label: year ? `${name} (${year})` : name, plays: watched, listedAt })
    else films.push({ name, year, plays: watched.map((at) => ({ at })), listedAt })
  }

  return { films, idFilms, episodes, later, dropped, ratings: [], unread: 0, undated }
}
