import { expect, test } from 'vitest'
import { hasHistory, isTVTime, readTVTime, tvtimeFile, unreadableOut } from './tvtime'

const v2 =
  's_no,season_number,key,user_id,episode_id,ep_id,episode_number,gsi,s_id,created_at,series_name,ep_no,is_followed,is_archived,is_for_later,followed_at,bulk_type\n' +
  '1,,watch-episode-aa-bb,1,,349232,,,81189,2024-02-01 20:00:00,Breaking Bad,2,,,,,\n' +
  '1,,rewatch-episode-aa-bb-1,1,,349232,,,81189,2024-05-01 20:00:00,Breaking Bad,2,,,,,\n' +
  '0,3,watch-episode-cc-dd,1,4161692,,7,,79126,2023-01-01 10:00:00,The Wire,0,,,,,\n' +
  '2,,watch-episode-ee-ff,1,,555,,,79126,2023-01-02 10:00:00,The Wire,1,,,,,1\n' +
  ',,user-series-aa,1,,,,,81189,2020-01-01 00:00:00,Breaking Bad,,true,false,false,2020-01-01 00:00:00,\n' +
  ',,user-series-gg,1,,,,,121361,2020-01-01 00:00:00,Game of Thrones,,true,true,false,,\n' +
  ',,user-series-hh,1,,,,,305288,2020-01-01 00:00:00,Stranger Things,,true,false,true,2021-03-04 05:06:07,\n'

const v1 =
  'type,entity_type,series_id,series_name,season_number,episode_number,watch_date,created_at,movie_name,release_date\n' +
  'watch,episode,81189,Breaking Bad,1,2,1.6e+09,,,\n' +
  'watch,episode,81189,Breaking Bad,1,3,1.615156952e+09,,,\n' +
  'watch,movie,,,,,1.615156952e+09,,Heat,1995-12-15\n' +
  'rewatch,movie,,,,,1.7e+09,,Heat,1995-12-15\n' +
  'towatch,movie,,,,,,2022-06-01 08:00:00,Lola,0001-01-01\n' +
  'follow,movie,,,,,,,Ignored,2001-01-01\n'

test('spots an export and opens only the files it needs', () => {
  expect(isTVTime(['gdpr-data/tracking-prod-records-v2.csv', 'gdpr-data/user.csv'])).toBe(true)
  expect(isTVTime(['watched.csv', 'diary.csv'])).toBe(false)
  expect(tvtimeFile('gdpr-data/tracking-prod-records.csv')).toBe(true)
  expect(tvtimeFile('gdpr-data/access_token.csv')).toBe(false)
  expect(tvtimeFile('ip_address.csv')).toBe(false)
})

test('reads episodes, films, shows and ratings', () => {
  const t = readTVTime(
    new Map([
      ['gdpr-data/tracking-prod-records-v2.csv', v2],
      ['gdpr-data/tracking-prod-records.csv', v1],
      ['gdpr-data/tv_show_rate.csv', 'tv_show_id,rate,created_at\n81189,4.5,2024-06-01 00:00:00\n79126,0,\n'],
    ]),
  )
  expect(t.episodes).toEqual([
    { tvdb: 81189, season: 1, number: 2, label: 'Breaking Bad 1x02', at: '2024-02-01T20:00:00.000Z' },
    { tvdb: 81189, season: 1, number: 2, label: 'Breaking Bad 1x02', at: '2024-05-01T20:00:00.000Z' },
    { tvdb: 79126, season: 3, number: 7, label: 'The Wire 3x07', at: '2023-01-01T10:00:00.000Z' },
    { tvdb: 79126, season: 2, number: 1, label: 'The Wire 2x01', at: undefined },
    { tvdb: 81189, season: 1, number: 2, label: 'Breaking Bad 1x02', at: '2020-09-13T12:26:40.000Z' },
    { tvdb: 81189, season: 1, number: 3, label: 'Breaking Bad 1x03', at: '2021-03-07T22:42:32.000Z' },
  ])
  expect(t.films).toEqual([
    { name: 'Heat', year: 1995, plays: [{ at: '2021-03-07T22:42:32.000Z' }, { at: '2023-11-14T22:13:20.000Z' }] },
    { name: 'Lola', year: undefined, plays: [], listedAt: '2022-06-01T08:00:00.000Z' },
  ])
  expect(t.later).toEqual([{ tvdb: 305288, label: 'Stranger Things', at: '2021-03-04T05:06:07.000Z' }])
  expect(t.dropped).toEqual([{ tvdb: 121361, label: 'Game of Thrones' }])
  expect(t.ratings).toEqual([{ tvdb: 81189, label: 'Breaking Bad', rating: 9, at: '2024-06-01T00:00:00.000Z' }])
})

test('reads episode numbers from either pair of columns', () => {
  const t = readTVTime(
    new Map([
      [
        'tracking-prod-records-v2.csv',
        's_no,season_number,key,ep_id,episode_number,s_id,series_name,ep_no\n2,,watch-episode-x,,,79126,The Wire,4\n',
      ],
    ]),
  )
  expect(t.episodes.map((e) => e.label)).toEqual(['The Wire 2x04'])
})

test('drops a play only when the newer file has the same watch', () => {
  const t = readTVTime(
    new Map([
      [
        'tracking-prod-records-v2.csv',
        's_no,key,ep_id,s_id,created_at,series_name,ep_no\n1,watch-episode-x,9,1,2024-01-01 20:00:00,Show,1\n1,watch-movie-y,,,2024-01-01 20:00:00,,\n',
      ],
      [
        'tracking-prod-records.csv',
        'type,entity_type,series_id,series_name,season_number,episode_number,created_at\n' +
          'watch,episode,1,Show,1,1,2024-01-01 20:03:00\n' +
          'watch,episode,1,Show,1,1,2016-05-05 20:00:00\n' +
          'watch,episode,,Lost,1,1,2016-05-05 20:00:00\n' +
          'watch,podcast,,,,,2016-05-05 20:00:00\n',
      ],
    ]),
  )
  expect(t.episodes.map((e) => `${e.label} ${e.at}`)).toEqual([
    'Show 1x01 2024-01-01T20:00:00.000Z',
    'Show 1x01 2016-05-05T20:00:00.000Z',
    'Lost 1x01 2016-05-05T20:00:00.000Z',
  ])
  expect(t.unread).toBe(2)
})

test('keeps ratings it can’t place, and reads a ten-point scale', () => {
  const t = readTVTime(new Map([['tv_show_rate.csv', 'tvdb,score\n1,8\n2,0\n']]))
  expect(t.ratings).toEqual([{ tvdb: undefined, label: 'Unknown show', rating: 8, at: undefined }])
})

test('spots any TV Time export, even without its history files', () => {
  expect(isTVTime(['gdpr-data/user.csv'])).toBe(true)
  expect(isTVTime(['access_token.csv', 'ip_address.csv'])).toBe(true)
  expect(hasHistory(new Map([['gdpr-data/tv_show_rate.csv', '']]))).toBe(false)
})

test('leaves out dates that can’t be right', () => {
  const t = readTVTime(
    new Map([
      [
        'tracking-prod-records-v2.csv',
        's_no,key,ep_id,s_id,created_at,series_name,ep_no\n1,watch-episode-x,9,1,51727513-11-13 21:26:40,Show,1\n1,watch-episode-y,10,1,0001-01-01 00:00:00,Show,2\n',
      ],
    ]),
  )
  expect(t.episodes.map((e) => e.at)).toEqual([undefined, undefined])
  expect(t.undated).toBe(2)
})

const series = JSON.stringify([
  {
    uuid: 'a',
    id: { tvdb: 372264, imdb: null },
    created_at: '2022-05-14T22:04:29.464987Z',
    title: 'Slow Horses',
    status: 'continuing',
    seasons: [
      {
        number: 0,
        is_specials: true,
        episodes: [{ id: { tvdb: 1 }, number: 1, is_watched: false, watched_at: null, rewatch_count: 0 }],
      },
      {
        number: 1,
        episodes: [
          { id: { tvdb: 2 }, number: 1, is_watched: true, watched_at: '2022-05-15T20:00:00Z', rewatch_count: 1 },
          { id: { tvdb: 3 }, number: 2, is_watched: true, watched_at: null, rewatch_count: 0 },
        ],
      },
    ],
  },
  {
    id: { tvdb: 79126 },
    title: 'The Wire',
    status: 'not_started_yet',
    created_at: '2023-01-01T00:00:00Z',
    seasons: [],
  },
  { id: { tvdb: 121361 }, title: 'Game of Thrones', status: 'stopped', seasons: [] },
])

const movies = JSON.stringify([
  {
    id: { tvdb: 56719, imdb: 'tt4119270' },
    title: '1989',
    year: 2014,
    is_watched: true,
    watched_at: '2022-11-22T19:35:27Z',
    rewatch_count: 0,
  },
  {
    tvdbId: 3647,
    imdbId: 'tt0100404',
    title: 'Présumé innocent',
    year: 1990,
    isWatched: false,
    watchedAt: '2024-07-27T09:26:07Z',
  },
  { id: { tvdb: 9, imdb: 'tt12' }, title: 'Lola', is_watched: true, watched_at: 'soon' },
])

test('spots a TV Time Out export', () => {
  const names = ['tvtime-movies-2026-07-02.json', 'tvtime-series-2026-07-02.json', 'tvtime-summary-2026-07-02.html']
  expect(isTVTime(names)).toBe(true)
  expect(names.map(tvtimeFile)).toEqual([true, true, false])
  expect(hasHistory(new Map([['tvtime-movies-2026-07-02.json', '[]']]))).toBe(true)
})

test('reads a TV Time Out export', () => {
  const t = readTVTime(
    new Map([
      ['tvtime-series-2026-07-02.json', series],
      ['tvtime-movies-2026-07-02.json', movies],
    ]),
  )
  expect(t.episodes).toEqual([
    { tvdb: 372264, season: 1, number: 1, label: 'Slow Horses 1x01', at: '2022-05-15T20:00:00.000Z' },
    { tvdb: 372264, season: 1, number: 2, label: 'Slow Horses 1x02', at: undefined },
  ])
  expect(t.later).toEqual([{ tvdb: 79126, label: 'The Wire', at: '2023-01-01T00:00:00.000Z' }])
  expect(t.dropped).toEqual([{ tvdb: 121361, label: 'Game of Thrones' }])
  expect(t.idFilms).toEqual([
    { imdb: 'tt4119270', label: '1989 (2014)', plays: ['2022-11-22T19:35:27.000Z'], listedAt: undefined },
    { imdb: 'tt0100404', label: 'Présumé innocent (1990)', plays: [], listedAt: '2024-07-27T09:26:07.000Z' },
  ])
  expect(t.films).toEqual([{ name: 'Lola', year: undefined, plays: [{ at: undefined }], listedAt: undefined }])
  expect(t.undated).toBe(1)
})

test('stops on a TV Time Out file it cannot read', () => {
  expect(() => readTVTime(new Map([['tvtime-series-2026-07-02.json', '{oops']]))).toThrow(unreadableOut)
  expect(() => readTVTime(new Map([['tvtime-series-2026-07-02.json', '[null]']]))).toThrow(unreadableOut)
  expect(() => readTVTime(new Map([['tvtime-series-2026-07-02.json', '[{"seasons":{}}]']]))).toThrow(unreadableOut)
})
