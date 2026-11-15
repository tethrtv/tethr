import type { components } from './api/schema'

type Ids = components['schemas']['Ids']
type Kind = 'movie' | 'show'

export type Link = { name: string; url: string }

const sites: [string, (ids: Ids, kind: Kind) => string | number | undefined][] = [
  ['IMDb', (i) => i.imdb && `https://www.imdb.com/title/${i.imdb}/`],
  ['Letterboxd', (i) => i.letterboxd && `https://letterboxd.com/film/${i.letterboxd}/`],
  ['TMDB', (i, k) => i.tmdb && `https://www.themoviedb.org/${k === 'show' ? 'tv' : 'movie'}/${i.tmdb}`],
  ['TVDB', (i, k) => i.tvdb && `https://thetvdb.com/dereferrer/${k === 'show' ? 'series' : 'movie'}/${i.tvdb}`],
  ['TVmaze', (i) => i.tvmaze && `https://www.tvmaze.com/shows/${i.tvmaze}`],
  ['MyAnimeList', (i) => i.mal && `https://myanimelist.net/anime/${i.mal}`],
  ['AniList', (i) => i.anilist && `https://anilist.co/anime/${i.anilist}`],
  ['AniDB', (i) => i.anidb && `https://anidb.net/anime/${i.anidb}`],
  ['Rotten Tomatoes', (i) => i.rottentomatoes && `https://www.rottentomatoes.com/${i.rottentomatoes}`],
  ['Metacritic', (i) => i.metacritic && `https://www.metacritic.com/${i.metacritic}`],
  ['Wikidata', (i) => i.wikidata && `https://www.wikidata.org/wiki/${i.wikidata}`],
  ['Netflix', (i) => i.netflix && `https://www.netflix.com/title/${i.netflix}`],
  ['Prime Video', (i) => i.primevideo && `https://www.amazon.com/gp/video/detail/${i.primevideo}`],
  [
    'Disney+',
    (i, k) =>
      i.disneyplus &&
      (/^(entity-)?[0-9a-f]{8}-/.test(i.disneyplus)
        ? `https://www.disneyplus.com/browse/entity-${i.disneyplus.replace(/^entity-/, '')}`
        : `https://www.disneyplus.com/${k === 'show' ? 'series/wp' : 'movies/wd'}/${i.disneyplus}`),
  ],
  ['Apple TV', (i, k) => i.appletv && `https://tv.apple.com/${k === 'show' ? 'show' : 'movie'}/${i.appletv}`],
  [
    'Plex',
    (i) =>
      i.plex &&
      `https://app.plex.tv/desktop/#!/provider/tv.plex.provider.metadata/details?key=/library/metadata/${i.plex}`,
  ],
]

export function elsewhere(kind: Kind, ids: Ids): Link[] {
  return sites.flatMap(([name, url]) => {
    const u = url(ids, kind)
    return typeof u === 'string' && u ? [{ name, url: u }] : []
  })
}

export function runYears(started?: string, ended?: string, year?: number) {
  const from = started?.slice(0, 4) ?? year?.toString()
  const to = ended?.slice(0, 4)
  if (from && to) return from === to ? from : `${from} to ${to}`
  return from ?? (to && `Until ${to}`)
}

const genreNames: Record<string, string> = {
  'science-fiction': 'science fiction',
  'talk-and-game': 'talk and game',
}

const typeNames: Record<string, string> = { 'tv-film': 'TV film' }

const sentence = (words: string[]) => {
  const out = words.join(', ')
  return out && out[0].toUpperCase() + out.slice(1)
}

export function genres(slugs?: string[]) {
  return sentence((slugs ?? []).map((g) => genreNames[g] ?? g)) || undefined
}

export function types(kind: Kind, flags?: string[]) {
  const shown = (flags ?? []).filter((f) => !(f === 'animated' && flags?.includes('anime')))
  const named = shown.map((f) => (f === 'web' ? (kind === 'show' ? 'web series' : 'web film') : (typeNames[f] ?? f)))
  return sentence(named) || undefined
}

function displayNames(all: string[] | undefined, type: 'region' | 'language') {
  const codes = all?.filter((c) => c !== 'zxx')
  if (!codes?.length) return undefined
  const names = codes.map((code) => {
    try {
      return new Intl.DisplayNames('en-GB', { type, fallback: 'code' }).of(code) ?? code
    } catch {
      return code
    }
  })
  return new Intl.ListFormat('en-GB').format(names)
}

export const countries = (codes?: string[]) => displayNames(codes, 'region')
export const languages = (tags?: string[]) => displayNames(tags, 'language')
