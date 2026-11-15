import { expect, test } from 'vitest'
import { countries, elsewhere, genres, languages, runYears, types } from './facts'

test('links to every site Tethr has an ID for, streaming last', () => {
  const links = elsewhere('show', {
    tethr: 1,
    netflix: '70143836',
    imdb: 'tt0903747',
    tmdb: 1396,
    tvdb: 81189,
    wikidata: 'Q1079',
    rottentomatoes: 'tv/breaking_bad',
    disneyplus: 'entity-7d8b9e2c-1f3a-4b5c-8d9e-0a1b2c3d4e5f',
    eidr: '10.5240/0000',
    trakt: 1388,
  })
  expect(links).toEqual([
    { name: 'IMDb', url: 'https://www.imdb.com/title/tt0903747/' },
    { name: 'TMDB', url: 'https://www.themoviedb.org/tv/1396' },
    { name: 'TVDB', url: 'https://thetvdb.com/dereferrer/series/81189' },
    { name: 'Rotten Tomatoes', url: 'https://www.rottentomatoes.com/tv/breaking_bad' },
    { name: 'Wikidata', url: 'https://www.wikidata.org/wiki/Q1079' },
    { name: 'Netflix', url: 'https://www.netflix.com/title/70143836' },
    { name: 'Disney+', url: 'https://www.disneyplus.com/browse/entity-7d8b9e2c-1f3a-4b5c-8d9e-0a1b2c3d4e5f' },
  ])
  expect(
    elsewhere('movie', { tmdb: 603, letterboxd: 'the-matrix', disneyplus: '1vXLGiOUqEP9', appletv: 'umc.cmc.1' }),
  ).toEqual([
    { name: 'Letterboxd', url: 'https://letterboxd.com/film/the-matrix/' },
    { name: 'TMDB', url: 'https://www.themoviedb.org/movie/603' },
    { name: 'Disney+', url: 'https://www.disneyplus.com/movies/wd/1vXLGiOUqEP9' },
    { name: 'Apple TV', url: 'https://tv.apple.com/movie/umc.cmc.1' },
  ])
  expect(elsewhere('show', { disneyplus: '7d8b9e2c-1f3a-4b5c-8d9e-0a1b2c3d4e5f' })).toEqual([
    { name: 'Disney+', url: 'https://www.disneyplus.com/browse/entity-7d8b9e2c-1f3a-4b5c-8d9e-0a1b2c3d4e5f' },
  ])
})

test('says when a show ran', () => {
  expect(runYears('2008-01-20', '2013-09')).toBe('2008 to 2013')
  expect(runYears('2019', undefined)).toBe('2019')
  expect(runYears('2020-03', '2020-05-01')).toBe('2020')
  expect(runYears(undefined, '2013')).toBe('Until 2013')
  expect(runYears(undefined, '2013', 2008)).toBe('2008 to 2013')
  expect(runYears(undefined, undefined, 2008)).toBe('2008')
  expect(runYears(undefined, undefined)).toBeUndefined()
})

test('names genres, types, countries and languages', () => {
  expect(genres(['science-fiction', 'talk-and-game', 'drama'])).toBe('Science fiction, talk and game, drama')
  expect(genres([])).toBeUndefined()
  expect(types('show', ['anime', 'miniseries', 'web'])).toBe('Anime, miniseries, web series')
  expect(types('movie', ['animated', 'anime'])).toBe('Anime')
  expect(types('movie', ['tv-film', 'short', 'web'])).toBe('TV film, short, web film')
  expect(countries(['GB', 'US'])).toBe('United Kingdom and United States')
  expect(languages(['en', 'ja', 'zh'])).toBe('English, Japanese and Chinese')
  expect(languages(['not a tag'])).toBe('not a tag')
  expect(languages(['zxx'])).toBeUndefined()
})
