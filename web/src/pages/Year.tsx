import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { type CSSProperties, createContext, type ReactNode, useContext, useEffect, useRef, useState } from 'react'
import { api } from '../api/client'
import type { components } from '../api/schema'
import { Cover } from '../components/Cover'
import { useWatching } from '../components/Shell'
import { Television } from '../components/Television'
import { TitleLink } from '../components/TitleLink'
import { duration, ep, plural } from '../format'
import { image, useDetails } from '../tmdb'
import styles from './Year.module.css'

type Summary = components['schemas']['Year']
type Title = components['schemas']['Title']
type Watch = components['schemas']['Watch']

const zone = Intl.DateTimeFormat().resolvedOptions().timeZone
const wallSize = 36
const months = Array.from({ length: 12 }, (_, m) => m)
const dates = Array.from({ length: 31 }, (_, d) => d + 1)
const clock24 = Array.from({ length: 24 }, (_, h) => h)

const utc = (date: string) => new Date(`${date}T00:00:00Z`)
const dayName = (date: string, options: Intl.DateTimeFormatOptions) =>
  utc(date).toLocaleDateString('en-GB', { timeZone: 'UTC', ...options })
const hourName = (h: number) => (h === 0 ? 'midnight' : h === 12 ? 'noon' : h < 12 ? `${h}am` : `${h - 12}pm`)
const spin = (id: number, by: number) => (((id * 2654435761) % 1000) / 1000 - 0.5) * by
const kindOf = (t: { type?: string }) => (t.type === 'movie' ? 'movie' : 'show')

export function useSeen<T extends Element>() {
  const ref = useRef<T>(null)
  const [seen, setSeen] = useState(false)
  useEffect(() => {
    const el = ref.current
    if (!el || seen) return
    if (!('IntersectionObserver' in window)) {
      setSeen(true)
      return
    }
    const watch = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting)) {
          setSeen(true)
          watch.disconnect()
        }
      },
      { rootMargin: '0px 0px -12% 0px' },
    )
    watch.observe(el)
    return () => watch.disconnect()
  }, [seen])
  return [ref, seen] as const
}

const Seen = createContext(false)

function Section({ title, className, children }: { title: string; className?: string; children: ReactNode }) {
  const [ref, seen] = useSeen<HTMLElement>()
  return (
    <section ref={ref} className={`${styles.section} ${className ?? ''}`} data-seen={seen || undefined}>
      <h2 className={styles.sectionTitle}>{title}</h2>
      <Seen.Provider value={seen}>{children}</Seen.Provider>
    </section>
  )
}

function CountUp({ to }: { to: number }) {
  const [shown, setShown] = useState(to)
  useEffect(() => {
    if (to < 10 || matchMedia('(prefers-reduced-motion: reduce)').matches) {
      setShown(to)
      return
    }
    let frame = 0
    const start = performance.now()
    const tick = (now: number) => {
      const t = Math.min(1, (now - start) / 1100)
      setShown(Math.round(to * (1 - (1 - t) ** 3)))
      if (t < 1) frame = requestAnimationFrame(tick)
    }
    frame = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(frame)
  }, [to])
  return <>{shown.toLocaleString('en-GB')}</>
}

function Marked({ children }: { children: ReactNode }) {
  return (
    <span className={styles.marked}>
      <span className={styles.ink} aria-hidden="true" />
      <span className={styles.markedText}>{children}</span>
    </span>
  )
}

export function useYear(profileID: number, year: number) {
  return useQuery({
    queryKey: ['year', profileID, year, zone],
    queryFn: async () => {
      const ask = (tz?: string) =>
        api.GET('/v1/years/{year}', { params: { path: { year }, query: { profile_id: profileID, tz } } })
      let answer = await ask(zone)
      if (answer.error?.code === 'invalid_time_zone') answer = await ask()
      if (answer.error?.code === 'not_started') return null
      if (answer.error) throw answer.error
      return answer.data
    },
  })
}

export function YearPage({ year }: { year: number }) {
  const { profile } = useWatching()
  const summary = useYear(profile.id, year)
  const current = year === new Date().getFullYear()
  const heading = current ? `${year} so far` : String(year)

  if (summary.isPending) {
    return (
      <div className={styles.page}>
        <h1 className={styles.title}>{heading}</h1>
        <p className={styles.lead}>Adding up your year…</p>
      </div>
    )
  }
  if (summary.isError) {
    return (
      <div className={styles.page}>
        <h1 className={styles.title}>{heading}</h1>
        <p className={styles.lead}>Tethr couldn’t add up this year. Try again in a moment.</p>
      </div>
    )
  }
  if (!summary.data) {
    return (
      <div className={styles.page}>
        <h1 className={styles.title}>{year}</h1>
        <p className={styles.lead}>That year hasn’t started yet.</p>
      </div>
    )
  }
  const y = summary.data
  const others = y.years.filter((n) => n !== year)

  return (
    <div className={styles.page}>
      <title>{`${heading} · Tethr`}</title>
      <header className={styles.head}>
        <h1 className={styles.title}>{heading}</h1>
        <Lead y={y} current={current} />
        {others.length > 0 && (
          <p className={styles.years}>
            <span className={styles.faint}>Other years</span>
            {others.map((n) => (
              <Link key={n} to="/year/$year" params={{ year: String(n) }} className={styles.yearLink}>
                {n}
              </Link>
            ))}
          </p>
        )}
      </header>

      {y.plays > 0 && (
        <>
          <Section title="The year on one page">
            <p className={styles.small}>Each square is a day. The brighter it is, the longer you watched.</p>
            <Planner y={y} />
          </Section>

          <Section title="Everything you watched" className={styles.wallSection}>
            <Wall y={y} />
          </Section>

          {(y.first || y.latest) && (
            <Section title="On the box">
              <div className={styles.tellies}>
                {y.first && <Telly watch={y.first} label={current ? 'First this year' : `First of ${year}`} />}
                {y.latest && y.latest.id !== y.first?.id && (
                  <Telly watch={y.latest} label={current ? 'Most recent' : `Last of ${year}`} />
                )}
              </div>
            </Section>
          )}

          <div className={styles.columns}>
            {y.shows.length > 0 && (
              <Section title="Most watched">
                <TopShows shows={y.shows.slice(0, 10)} />
              </Section>
            )}
            {y.hours.some((n) => n > 0) && (
              <Section title="When you press play">
                <Hours hours={y.hours} />
              </Section>
            )}
          </div>

          <div className={styles.columns}>
            {y.busiest && (
              <Section title="Biggest day">
                <Busiest day={y.busiest} timed={timed(y)} />
              </Section>
            )}
            {y.run.days > 1 && y.run.from && (
              <Section title="Longest run">
                <Run days={y.run.days} from={y.run.from} />
              </Section>
            )}
          </div>

          {y.film_plays.length > 0 && (
            <Section title="Films, in order">
              <Films films={y.film_plays} />
            </Section>
          )}
        </>
      )}
    </div>
  )
}

function both(a: string | false, b: string | false) {
  if (a && b) {
    return (
      <>
        <strong>{a}</strong> and <strong>{b}</strong>
      </>
    )
  }
  return <strong>{a || b}</strong>
}

const timed = (y: Summary) => y.minutes > 0 && y.plays_without_length * 2 <= y.plays

function Lead({ y, current }: { y: Summary; current: boolean }) {
  if (y.plays === 0) {
    return (
      <p className={styles.lead}>
        {current ? 'Nothing played yet this year.' : `Nothing was played in ${y.year}.`} Everything your apps log, and
        anything you import, adds up here.
      </p>
    )
  }
  const hours = Math.round(y.minutes / 60)
  const active = y.days.filter((d) => d.plays > 0).length
  return (
    <p className={styles.lead}>
      {current ? 'Since 1 January you’ve watched' : `In ${y.year} you watched`}{' '}
      {timed(y) && (
        <>
          about{' '}
          <strong>
            <Marked>{hours >= 2 ? <CountUp to={hours} /> : duration(y.minutes)}</Marked>
            {hours >= 2 && ' hours'}
          </strong>
          :{' '}
        </>
      )}
      {both(y.episodes > 0 && plural(y.episodes, 'episode'), y.films > 0 && plural(y.films, 'film'))}, on {active} of{' '}
      {y.days.length} days.{' '}
      {y.new_shows > 0 && (
        <>
          You started <strong>{plural(y.new_shows, 'new show')}</strong>
          {y.rewatched > 0 ? ' and went back to ' : '. '}
        </>
      )}
      {y.rewatched > 0 && (
        <>
          {y.new_shows > 0 ? '' : 'You went back to '}
          {plural(y.rewatched, 'thing')} you’d seen before.{' '}
        </>
      )}
      {y.run.days > 1 && (
        <>
          Your longest run was <strong>{y.run.days} days</strong> in a row.
        </>
      )}
    </p>
  )
}

const level = (d: { plays: number; minutes: number }) =>
  d.plays === 0 ? 0 : d.minutes < 60 ? 1 : d.minutes < 120 ? 2 : d.minutes < 240 ? 3 : 4

function Planner({ y }: { y: Summary }) {
  const box = useRef<HTMLDivElement>(null)
  const [tip, setTip] = useState<{ x: number; y: number; text: string } | null>(null)
  const start = Date.UTC(y.year, 0, 1)
  const shown = months.filter((m) => Date.UTC(y.year, m, 1) <= utc(y.through).getTime())
  const show = (e: React.PointerEvent, text: string) => {
    const r = e.currentTarget.getBoundingClientRect()
    const b = box.current?.getBoundingClientRect()
    if (b) setTip({ x: r.left + r.width / 2 - b.left, y: r.top - b.top, text })
  }
  const active = y.days.filter((d) => d.plays > 0).length

  return (
    <div className={styles.plannerScroll}>
      <div
        className={styles.planner}
        ref={box}
        onPointerLeave={() => setTip(null)}
        role="img"
        aria-label={`A calendar of ${y.year}, with something played on ${active} of ${y.days.length} days.`}
      >
        <div className={`${styles.plannerRow} ${styles.dates}`} aria-hidden="true">
          <span />
          {dates.map((n) => (
            <span key={n}>{n}</span>
          ))}
        </div>
        {shown.map((m) => {
          const length = new Date(Date.UTC(y.year, m + 1, 0)).getUTCDate()
          return (
            <div key={m} className={styles.plannerRow} aria-hidden="true">
              <span className={styles.month}>
                {new Date(Date.UTC(y.year, m, 1)).toLocaleDateString('en-GB', { month: 'short', timeZone: 'UTC' })}
              </span>
              {dates.map((n) => {
                const d = n - 1
                const at = Date.UTC(y.year, m, n)
                const index = Math.round((at - start) / 86400000)
                const day = y.days[index]
                if (d >= length || !day) return <span key={n} className={`${styles.day} ${styles.none}`} />
                const weekday = new Date(at).getUTCDay()
                const label = new Date(at).toLocaleDateString('en-GB', {
                  weekday: 'short',
                  day: 'numeric',
                  month: 'short',
                  timeZone: 'UTC',
                })
                const text = day.plays
                  ? `${label}: ${plural(day.plays, 'play')}${day.minutes ? `, ${duration(day.minutes)}` : ''}`
                  : `${label}: nothing`
                return (
                  <span
                    key={n}
                    className={`${styles.day} ${styles[`l${level(day)}`] ?? ''} ${weekday === 0 || weekday === 6 ? styles.weekend : ''}`}
                    style={{ '--i': index } as CSSProperties}
                    onPointerEnter={(e) => show(e, text)}
                  />
                )
              })}
            </div>
          )
        })}
        {tip && (
          <span className={styles.tip} style={{ left: tip.x, top: tip.y }}>
            {tip.text}
          </span>
        )}
      </div>
    </div>
  )
}

type Tile = { title: Title; kind: 'movie' | 'show'; note: string }

function Wall({ y }: { y: Summary }) {
  const tiles: Tile[] = y.shows.map((s) => ({
    title: s.show,
    kind: 'show',
    note: plural(s.episodes, 'episode'),
  }))
  const films = new Set<number>()
  for (const f of y.film_plays) {
    const id = f.film.ids.tethr ?? 0
    if (films.has(id)) continue
    films.add(id)
    tiles.push({ title: f.film, kind: 'movie', note: 'Film' })
  }
  const more = tiles.length - wallSize
  return (
    <>
      <p className={styles.small}>
        {both(y.shows.length > 0 && plural(y.shows.length, 'show'), films.size > 0 && plural(films.size, 'film'))}
        {y.shows.length > 0 && films.size > 0
          ? ': the shows you played most, then the films in the order you saw them.'
          : '.'}
      </p>
      <ul className={styles.wall}>
        {tiles.slice(0, wallSize).map((t, i) => (
          <WallTile key={`${t.kind}-${t.title.ids.tethr}`} tile={t} index={i} />
        ))}
      </ul>
      {more > 0 && <p className={styles.small}>And {plural(more, 'more title')}.</p>}
    </>
  )
}

function WallTile({ tile, index }: { tile: Tile; index: number }) {
  const meta = useDetails(tile.kind, tile.title.ids.tmdb)
  const id = tile.title.ids.tethr ?? index
  const name = meta?.name ?? tile.title.name ?? 'Untitled'
  return (
    <li className={styles.tile} style={{ '--i': index, '--r': `${spin(id, 7).toFixed(2)}deg` } as CSSProperties}>
      <TitleLink type={tile.kind} slug={tile.title.slug} className={styles.tileLink}>
        <Cover id={id} name={name} poster={meta?.poster} />
        <span className={styles.tileName}>{name}</span>
        <span className={styles.tileNote}>{tile.note}</span>
      </TitleLink>
    </li>
  )
}

function Telly({ watch, label }: { watch: Watch; label: string }) {
  const item = watch.item
  const meta = useDetails(kindOf(item), item.title.ids.tmdb)
  const name = meta?.name ?? item.title.name ?? 'Untitled'
  const poster = image(meta?.poster, 'w342')
  const when =
    watch.precision === 'day'
      ? new Date(watch.watched_at).toLocaleDateString('en-GB', { day: 'numeric', month: 'long', timeZone: 'UTC' })
      : new Date(watch.watched_at).toLocaleString('en-GB', {
          day: 'numeric',
          month: 'long',
          hour: '2-digit',
          minute: '2-digit',
        })
  const seen = useContext(Seen)
  return (
    <figure className={styles.telly}>
      <svg viewBox="0 0 240 230" className={styles.tellySet} role="img" aria-label={`${name} on an old television`}>
        <Television still={image(meta?.backdrop, 'w300')} poster={poster} name={name} on={seen} />
      </svg>
      <figcaption>
        <span className={styles.faint}>{label}</span>
        <TitleLink type={kindOf(item)} slug={item.title.slug} className={styles.tellyName}>
          {name}
        </TitleLink>
        <span className={styles.small}>
          {item.episode ? `${ep(item.episode.season, item.episode.number)}, ` : ''}
          {when}
        </span>
      </figcaption>
    </figure>
  )
}

function TopShows({ shows }: { shows: Summary['shows'] }) {
  const most = Math.max(1, ...shows.map((s) => s.episodes))
  return (
    <ol className={styles.top}>
      {shows.map((s, i) => (
        <TopShow key={s.show.ids.tethr} show={s} rank={i + 1} share={s.episodes / most} />
      ))}
    </ol>
  )
}

function TopShow({ show, rank, share }: { show: Summary['shows'][number]; rank: number; share: number }) {
  const meta = useDetails('show', show.show.ids.tmdb)
  return (
    <li className={styles.topRow} style={{ '--i': rank, '--share': share } as CSSProperties}>
      <span className={styles.rank}>{rank}</span>
      <span className={styles.topMain}>
        <TitleLink type="show" slug={show.show.slug} className={styles.topName}>
          {meta?.name ?? show.show.name ?? 'Untitled'}
        </TitleLink>
        <span className={styles.bar} aria-hidden="true" />
      </span>
      <span className={styles.topNum}>{plural(show.episodes, 'episode')}</span>
    </li>
  )
}

function Hours({ hours }: { hours: number[] }) {
  const most = Math.max(...hours)
  const peak = hours.indexOf(most)
  const verdict =
    peak >= 21 || peak < 4
      ? 'A night owl, then.'
      : peak >= 17
        ? 'Evenings, mostly.'
        : peak >= 12
          ? 'An afternoon watcher.'
          : 'An early riser.'
  return (
    <div className={styles.hours}>
      <p className={styles.quiet}>
        Most plays start around <strong>{hourName(peak)}</strong>. {verdict}
      </p>
      <div
        className={styles.hoursPlot}
        role="img"
        aria-label={`Plays by the hour they started, most around ${hourName(peak)}.`}
      >
        {clock24.map((h) => (
          <span key={h} className={styles.hour} title={`${hourName(h)}: ${plural(hours[h] ?? 0, 'play')}`}>
            <span
              className={h === peak ? `${styles.hourBar} ${styles.peak}` : styles.hourBar}
              style={
                {
                  '--i': h,
                  height: `${Math.max(hours[h] ? 4 : 1.5, ((hours[h] ?? 0) / most) * 100)}%`,
                } as CSSProperties
              }
            />
          </span>
        ))}
      </div>
      <div className={styles.axis} aria-hidden="true">
        <span>00</span>
        <span>06</span>
        <span>12</span>
        <span>18</span>
        <span>23</span>
      </div>
    </div>
  )
}

function Busiest({ day, timed }: { day: NonNullable<Summary['busiest']>; timed: boolean }) {
  const item = day.item
  const kind = kindOf(item)
  const meta = useDetails(kind, item.title.ids.tmdb)
  const name = meta?.name ?? item.title.name ?? 'Untitled'
  const others = day.plays - day.item_plays
  return (
    <div className={styles.busiest}>
      <div className={styles.busiestArt}>
        <Cover id={item.title.ids.tethr ?? 0} name={name} poster={meta?.poster} size="l" />
        <span className={styles.stamp} aria-hidden="true">
          {dayName(day.date, { day: 'numeric', month: 'short' })}
        </span>
      </div>
      <p className={styles.busiestText}>
        <strong>{dayName(day.date, { weekday: 'long', day: 'numeric', month: 'long' })}.</strong>{' '}
        {kind === 'movie' ? 'You watched ' : `You played ${plural(day.item_plays, 'episode')} of `}
        <TitleLink type={kind} slug={item.title.slug} className={styles.inlineLink}>
          {name}
        </TitleLink>
        {others > 0 ? ` and ${plural(others, 'other thing')}` : ''}
        {timed && day.minutes ? `, ${duration(day.minutes)} in all.` : '.'}
      </p>
    </div>
  )
}

function Run({ days, from }: { days: number; from: string }) {
  const drawn = Math.min(days, 60)
  const strokes = Array.from({ length: drawn }, (_, i) => i)
  const width = Math.ceil(drawn / 5) * 46
  return (
    <div className={styles.run}>
      <svg
        viewBox={`0 0 ${width} 56`}
        width={width * 1.4}
        height={78}
        className={styles.tally}
        role="img"
        aria-label={`${days} tally marks`}
      >
        {strokes.map((i) => {
          const g = Math.floor(i / 5)
          const x = g * 46 + 6 + (i % 5) * 8
          const d = i % 5 === 4 ? `M${g * 46 + 2} 40L${g * 46 + 40} 14` : `M${x} 8L${x + 1} 48`
          return <path key={i} d={d} style={{ '--i': i } as CSSProperties} />
        })}
      </svg>
      <p className={styles.quiet}>
        <strong>{days} days</strong> in a row from {dayName(from, { day: 'numeric', month: 'long' })}, with something
        played every one of them{days > drawn ? `. The first ${drawn} are marked here.` : '.'}
      </p>
    </div>
  )
}

function Films({ films }: { films: Summary['film_plays'] }) {
  return (
    <ol className={styles.films}>
      {films.map((f, i) => (
        <FilmRow key={`${f.film.ids.tethr}-${f.watched_at}`} film={f} index={i} />
      ))}
    </ol>
  )
}

function FilmRow({ film, index }: { film: Summary['film_plays'][number]; index: number }) {
  const meta = useDetails('movie', film.film.ids.tmdb)
  const name = meta?.name ?? film.film.name ?? 'Untitled'
  return (
    <li className={styles.film} style={{ '--i': index } as CSSProperties}>
      <span className={styles.filmDate}>
        {new Date(film.watched_at).toLocaleDateString('en-GB', {
          day: 'numeric',
          month: 'short',
          ...(film.precision === 'day' ? { timeZone: 'UTC' } : {}),
        })}
      </span>
      <TitleLink type="movie" slug={film.film.slug} className={styles.filmName}>
        {name}
      </TitleLink>
      <span className={styles.filmYear}>{film.film.year ?? ''}</span>
      <span>{film.again ? <span className={styles.again}>Again</span> : null}</span>
    </li>
  )
}
