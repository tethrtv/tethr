import { useQuery } from '@tanstack/react-query'

export type Details = {
  name: string
  year?: number
  poster?: string
  backdrop?: string
  overview?: string
  tagline?: string
  genres?: string[]
  runtime?: number
  certificate?: { country: string; rating: string }
  score?: number
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
  aired?: { season: number; number: number }
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
  const res = await fetch(`${apiURL}/meta${path}?v=4`)
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
      if (res.status === 429) throw new Error('That’s a lot of searching. Try again in a moment.')
      if (!res.ok) throw new Error('Search isn’t working right now.')
      return (await res.json()).results
    },
  })
}

export type Trend = Found & { backdrop?: string }

export function useTrending() {
  return useQuery({
    queryKey: ['meta', 'trending'],
    staleTime: 60 * 60 * 1000,
    queryFn: async () => (await meta<{ results: Trend[] }>('/trending'))?.results ?? [],
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

export type Order = { id: string; name: string; type: number; seasons: number; episodes: number }
export type Group = { name: string; type: number; seasons: { number: number; name: string; episodes: Episode[] }[] }

export function useOrders(show?: number) {
  return useQuery({
    queryKey: ['meta', 'orders', show],
    enabled: !!show,
    staleTime: day,
    queryFn: async () => (await meta<{ orders: Order[] }>(`/tv/${show}/orders`))?.orders ?? [],
  }).data
}

export function useGroup(show?: number, group?: string) {
  return useQuery({
    queryKey: ['meta', 'order', show, group],
    enabled: !!show && !!group,
    staleTime: day,
    queryFn: () => meta<Group>(`/tv/${show}/order/${group}`),
  }).data
}

export type Matched = { match: Found | null; candidates: Found[] }

export async function matchFilms(
  films: { name: string; year?: number }[],
  signal: AbortSignal,
  onWait: (waiting: boolean) => void,
): Promise<Matched[]> {
  for (;;) {
    const res = await fetch(`${apiURL}/meta/match`, {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ films: films.map((f) => ({ name: f.name, year: f.year ?? 0 })) }),
      signal,
    })
    if (res.status === 429) {
      onWait(true)
      await new Promise((r) => setTimeout(r, Number(res.headers.get('Retry-After') ?? 30) * 1000))
      signal.throwIfAborted()
      continue
    }
    onWait(false)
    if (!res.ok) throw new Error('Matching isn’t working right now. Try again in a moment.')
    return (await res.json()).results
  }
}
