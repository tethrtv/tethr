import { expect, test } from 'vitest'
import { isLetterboxd, parseCSV, readLetterboxd, unzip } from './letterboxd'

test('reads quoted cells, commas and line breaks', () => {
  expect(parseCSV('Name,Review\r\n"Crouching Tiger, Hidden Dragon","Said ""wow""\nthen left"\r\n\r\n')).toEqual([
    ['Name', 'Review'],
    ['Crouching Tiger, Hidden Dragon', 'Said "wow"\nthen left'],
  ])
})

test('turns an export into films with their plays, ratings and watchlist', () => {
  const files = new Map([
    [
      'diary.csv',
      'Date,Name,Year,Letterboxd URI,Rating,Rewatch,Tags,Watched Date\n' +
        '2026-11-09,Heat,1995,https://boxd.it/a,4.5,,,2026-11-08\n' +
        '2026-11-10,Heat,1995,https://boxd.it/b,,Yes,,2026-11-10\n',
    ],
    [
      'watched.csv',
      'Date,Name,Year,Letterboxd URI\n2026-11-09,Heat,1995,https://boxd.it/x\n2020-01-01,"Crouching Tiger, Hidden Dragon",2000,https://boxd.it/y\n',
    ],
    ['ratings.csv', 'Date,Name,Year,Letterboxd URI,Rating\n2026-11-09,Heat,1995,https://boxd.it/x,4.5\n'],
    ['watchlist.csv', 'Date,Name,Year,Letterboxd URI\n2026-10-01,Lola,2023,https://boxd.it/z\n'],
    ['profile.csv', 'Date Joined,Username,Email Address\n2026-11-09,someone,someone@example.com\n'],
  ])
  expect(isLetterboxd(files)).toBe(true)
  expect(readLetterboxd(files)).toEqual([
    {
      name: 'Heat',
      year: 1995,
      plays: [{ at: '2026-11-08T12:00:00Z' }, { at: '2026-11-10T12:00:00Z' }],
      rating: 9,
      ratedAt: '2026-11-09T12:00:00Z',
    },
    { name: 'Crouching Tiger, Hidden Dragon', year: 2000, plays: [{}] },
    { name: 'Lola', year: 2023, plays: [], listedAt: '2026-10-01T12:00:00Z' },
  ])
})

test('knows other exports are not from Letterboxd', () => {
  expect(isLetterboxd(new Map([['watched-history.json', '[]']]))).toBe(false)
})

async function zip(entries: [string, string, boolean][]) {
  const parts: BlobPart[] = []
  const central: BlobPart[] = []
  let offset = 0
  let size = 0
  for (const [name, body, deflate] of entries) {
    const raw = new Uint8Array(new TextEncoder().encode(body))
    const data = deflate
      ? new Uint8Array(
          await new Response(
            (new Response(raw).body as ReadableStream).pipeThrough(new CompressionStream('deflate-raw')),
          ).arrayBuffer(),
        )
      : raw
    const nameBytes = new Uint8Array(new TextEncoder().encode(name))
    const local = new DataView(new ArrayBuffer(30))
    local.setUint32(0, 0x04034b50, true)
    local.setUint16(8, deflate ? 8 : 0, true)
    local.setUint32(18, data.length, true)
    local.setUint32(22, raw.length, true)
    local.setUint16(26, nameBytes.length, true)
    const dir = new DataView(new ArrayBuffer(46))
    dir.setUint32(0, 0x02014b50, true)
    dir.setUint16(10, deflate ? 8 : 0, true)
    dir.setUint32(20, data.length, true)
    dir.setUint32(24, raw.length, true)
    dir.setUint16(28, nameBytes.length, true)
    dir.setUint32(42, offset, true)
    parts.push(new Uint8Array(local.buffer), nameBytes, data)
    central.push(new Uint8Array(dir.buffer), nameBytes)
    size += 46 + nameBytes.length
    offset += 30 + nameBytes.length + data.length
  }
  const end = new DataView(new ArrayBuffer(22))
  end.setUint32(0, 0x06054b50, true)
  end.setUint16(10, entries.length, true)
  end.setUint32(12, size, true)
  end.setUint32(16, offset, true)
  return new Blob([...parts, ...central, new Uint8Array(end.buffer)])
}

test('unzips stored and deflated CSVs and skips everything else', async () => {
  const files = await unzip(
    await zip([
      ['watched.csv', 'Name,Year\nHeat,1995\n', true],
      ['deleted/diary.csv', 'Name\n', false],
      ['notes.txt', 'hello', false],
      ['ratings.csv', 'Name,Rating\n', false],
    ]),
  )
  expect([...files.keys()]).toEqual(['watched.csv', 'deleted/diary.csv', 'ratings.csv'])
  expect(files.get('watched.csv')).toBe('Name,Year\nHeat,1995\n')
})

test('refuses things that are not zips', async () => {
  await expect(unzip(new Blob(['hello']))).rejects.toThrow()
})
