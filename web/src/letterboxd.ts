export type Film = {
  name: string
  year?: number
  plays: { at?: string }[]
  rating?: number
  ratedAt?: string
  listedAt?: string
}

export async function unzip(blob: Blob): Promise<Map<string, string>> {
  const bytes = new Uint8Array(await blob.arrayBuffer())
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength)
  let end = -1
  for (let i = bytes.length - 22; i >= Math.max(0, bytes.length - 65557); i--) {
    if (view.getUint32(i, true) === 0x06054b50) {
      end = i
      break
    }
  }
  if (end < 0) throw new Error('not a zip')
  const count = view.getUint16(end + 10, true)
  let at = view.getUint32(end + 16, true)
  const files = new Map<string, string>()
  const text = new TextDecoder()
  for (let n = 0; n < count; n++) {
    if (view.getUint32(at, true) !== 0x02014b50) throw new Error('not a zip')
    const method = view.getUint16(at + 10, true)
    const size = view.getUint32(at + 20, true)
    const nameLength = view.getUint16(at + 28, true)
    const extra = view.getUint16(at + 30, true)
    const comment = view.getUint16(at + 32, true)
    const local = view.getUint32(at + 42, true)
    const name = text.decode(bytes.subarray(at + 46, at + 46 + nameLength))
    at += 46 + nameLength + extra + comment
    if (name.endsWith('/') || !name.toLowerCase().endsWith('.csv')) continue
    const start = local + 30 + view.getUint16(local + 26, true) + view.getUint16(local + 28, true)
    const data = bytes.subarray(start, start + size)
    if (method === 0) files.set(name, text.decode(data))
    else if (method === 8) {
      const stream = (new Response(data).body as ReadableStream).pipeThrough(new DecompressionStream('deflate-raw'))
      files.set(name, await new Response(stream).text())
    }
  }
  return files
}

export function parseCSV(text: string): string[][] {
  const rows: string[][] = []
  let row: string[] = []
  let cell = ''
  let quoted = false
  for (let i = 0; i < text.length; i++) {
    const c = text[i]
    if (quoted) {
      if (c === '"' && text[i + 1] === '"') {
        cell += '"'
        i++
      } else if (c === '"') quoted = false
      else cell += c
    } else if (c === '"') quoted = true
    else if (c === ',') {
      row.push(cell)
      cell = ''
    } else if (c === '\n' || c === '\r') {
      if (c === '\r' && text[i + 1] === '\n') i++
      row.push(cell)
      rows.push(row)
      row = []
      cell = ''
    } else cell += c
  }
  if (cell || row.length) {
    row.push(cell)
    rows.push(row)
  }
  return rows.filter((r) => r.some(Boolean))
}

function table(files: Map<string, string>, name: string) {
  const [head, ...rows] = parseCSV(files.get(name) ?? '')
  if (!head) return []
  return rows.map((r) => Object.fromEntries(head.map((h, i) => [h.replace(/^﻿/, ''), r[i] ?? ''])))
}

const day = (date: string) => (/^\d{4}-\d{2}-\d{2}$/.test(date) ? `${date}T12:00:00Z` : undefined)

export const isLetterboxd = (files: Map<string, string>) => files.has('watched.csv') || files.has('diary.csv')

export function readLetterboxd(files: Map<string, string>): Film[] {
  const films = new Map<string, Film>()
  const film = (row: Record<string, string>) => {
    const name = row.Name.trim()
    const year = Number.parseInt(row.Year, 10) || undefined
    const key = `${name}\n${year ?? ''}`
    let f = films.get(key)
    if (!f) {
      f = { name, year, plays: [] }
      films.set(key, f)
    }
    return f
  }

  for (const row of table(files, 'diary.csv')) {
    if (row.Name) film(row).plays.push({ at: day(row['Watched Date']) ?? day(row.Date) })
  }
  for (const row of table(files, 'watched.csv')) {
    if (!row.Name) continue
    const f = film(row)
    if (!f.plays.length) f.plays.push({})
  }
  for (const row of table(files, 'ratings.csv')) {
    const rating = Math.round(Number.parseFloat(row.Rating) * 2)
    if (!row.Name || !(rating >= 1 && rating <= 10)) continue
    const f = film(row)
    f.rating = rating
    f.ratedAt = day(row.Date)
  }
  for (const row of table(files, 'watchlist.csv')) {
    if (row.Name) film(row).listedAt = day(row.Date) ?? new Date().toISOString()
  }
  return [...films.values()]
}

export const label = (f: Pick<Film, 'name' | 'year'>) => (f.year ? `${f.name} (${f.year})` : f.name)
