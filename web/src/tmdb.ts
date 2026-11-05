import { useQuery } from '@tanstack/react-query'

export type Details = {
  name: string
  year?: number
  poster?: string
  overview?: string
  tagline?: string
  genres?: string[]
  runtime?: number
  certificate?: { country: string; rating: string }
  creators?: string[]
  cast?: string[]
  networks?: string[]
  status?: string
  nextEpisode?: { season: number; number: number; name: string; airDate?: string }
  seasons?: { number: number; episodes: number }[]
}

type Episode = {
  season?: number
  number: number
  name: string
  air_date: string | null
  overview?: string
  runtime?: number
}
type Title = Omit<Details, 'year' | 'poster' | 'nextEpisode'> & {
  year: number | null
  poster: string | null
  next_episode?: Episode
}
export type Season = { overview: string; episodes: Episode[] }

const apiURL = import.meta.env.VITE_API_URL ?? 'http://localhost:8080'
const images = 'https://image.tmdb.org/t/p/'
const day = 24 * 60 * 60 * 1000

async function meta<T>(path: string): Promise<T | null> {
  // Bump v when the shape of /meta's answers changes, or browsers and Cloudflare keep serving the old one.
  const res = await fetch(`${apiURL}/meta${path}?v=2`)
  return res.ok ? res.json() : null
}

export const image = (path: string | undefined, size = 'w185') => (path ? `${images}${size}${path}` : undefined)

export function useDetails(kind: 'movie' | 'show', id?: number) {
  return useQuery({
    queryKey: ['meta', kind, id],
    enabled: !!id,
    staleTime: day,
    queryFn: async (): Promise<Details | null> => {
      const t = await meta<Title>(`/${kind === 'movie' ? 'movie' : 'tv'}/${id}`)
      if (!t) return null
      const { year, poster, next_episode: next, ...rest } = t
      return {
        ...rest,
        year: year ?? undefined,
        poster: poster ?? undefined,
        nextEpisode: next && {
          season: next.season ?? 0,
          number: next.number,
          name: next.name,
          airDate: next.air_date ?? undefined,
        },
      }
    },
  }).data
}

export type Found = { type: 'movie' | 'show'; tmdb: number; name: string; year: number | null; poster: string | null }

export function useFind(q: string) {
  return useQuery({
    queryKey: ['meta', 'search', q],
    enabled: q.length >= 2,
    staleTime: 5 * 60 * 1000,
    queryFn: async (): Promise<Found[]> => {
      const res = await fetch(`${apiURL}/meta/search?q=${encodeURIComponent(q)}`, { credentials: 'include' })
      if (res.status === 429) throw new Error('That’s a lot of searching. Try again in a minute.')
      if (!res.ok) throw new Error('Search isn’t working right now.')
      return (await res.json()).results
    },
  })
}

export function useSeason(show?: number, season?: number) {
  return useQuery({
    queryKey: ['meta', 'season', show, season],
    enabled: !!show && season !== undefined,
    staleTime: day,
    queryFn: () => meta<Season>(`/tv/${show}/season/${season}`),
  }).data
}

export function useEpisode(show?: number, season?: number, number?: number) {
  const e = useSeason(show, season)?.episodes.find((x) => x.number === number)
  return e ? { name: e.name } : undefined
}
