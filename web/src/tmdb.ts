import { useQuery } from '@tanstack/react-query'

export type Details = { name: string; year?: number; poster?: string }

const apiURL = import.meta.env.VITE_API_URL ?? 'http://localhost:8080'
const images = 'https://image.tmdb.org/t/p/'
const day = 24 * 60 * 60 * 1000

async function meta<T>(path: string): Promise<T | null> {
  const res = await fetch(`${apiURL}/meta${path}`)
  return res.ok ? res.json() : null
}

export const image = (path: string | undefined, size = 'w185') => (path ? `${images}${size}${path}` : undefined)

export function useDetails(kind: 'movie' | 'show', id?: number) {
  return useQuery({
    queryKey: ['meta', kind, id],
    enabled: !!id,
    staleTime: day,
    queryFn: async (): Promise<Details | null> => {
      const t = await meta<{ name: string; year: number | null; poster: string | null }>(
        `/${kind === 'movie' ? 'movie' : 'tv'}/${id}`,
      )
      return t && { name: t.name, year: t.year ?? undefined, poster: t.poster ?? undefined }
    },
  }).data
}

export function useEpisode(show?: number, season?: number, number?: number) {
  const data = useQuery({
    queryKey: ['meta', 'season', show, season],
    enabled: !!show && season !== undefined,
    staleTime: day,
    queryFn: () => meta<{ episodes: { number: number; name: string }[] }>(`/tv/${show}/season/${season}`),
  }).data
  const e = data?.episodes.find((x) => x.number === number)
  return e ? { name: e.name } : undefined
}
