import styles from './ComingSoon.module.css'

const listing = [
  { slot: 'Logs itself', body: 'Zuno and other players tell it what you play, while you play it.' },
  { slot: 'In order', body: 'Aired, DVD or absolute order, chosen per show. A wrong match is yours to fix.' },
  { slot: 'Skips', body: 'Intro and credits markers matched to the exact cut of your file.' },
  { slot: 'Household', body: 'One account with a profile each, a kids’ profile and a guest.' },
  { slot: 'Leaving', body: 'Export everything, any time, free. Nothing is locked in.' },
]

export function ComingSoon() {
  return (
    <div className={styles.page}>
      <header className={styles.top}>
        <span className={styles.wordmark}>
          <Mark />
          tethr
        </span>
        <a className={styles.invite} href="/join?invite=">
          I have an invite
        </a>
      </header>

      <main className={styles.main}>
        <section className={styles.head}>
          <span className={styles.stamp}>Coming soon</span>
          <h1 className={styles.title}>A watch history that follows you between players.</h1>
          <p className={styles.lead}>
            Tethr keeps track of what you’ve watched and what’s next, in whichever app you watch it.{' '}
            <span className={styles.hl}>Free and open source</span>, with nothing to upgrade to.
          </p>
        </section>

        <section className={styles.section} aria-labelledby="listing">
          <h2 id="listing" className={styles.sectionTitle}>
            In the works
          </h2>
          <dl>
            {listing.map((l) => (
              <div key={l.slot} className={styles.row}>
                <dt>{l.slot}</dt>
                <dd>{l.body}</dd>
              </div>
            ))}
          </dl>
        </section>
      </main>

      <footer className={styles.foot}>
        <span>
          Questions? <a href="mailto:hello@tethr.tv">hello@tethr.tv</a> ·{' '}
          <a href="https://github.com/tethrtv">GitHub</a>
        </span>
        <span className={styles.faint}>Made by the people behind Zuno, kept separate so any player can use it.</span>
      </footer>
    </div>
  )
}

function Mark() {
  return (
    <svg width="26" height="26" viewBox="0 0 32 32" aria-hidden="true">
      <rect x="0.5" y="0.5" width="31" height="31" rx="7.5" fill="#141412" stroke="var(--rule-2)" />
      <path d="M9 21c3-9 11-1 14-10" fill="none" stroke="#d4fa3c" strokeWidth="3" strokeLinecap="round" />
      <circle cx="9" cy="21" r="3.2" fill="#d4fa3c" />
      <circle cx="23" cy="11" r="3.2" fill="#d4fa3c" />
    </svg>
  )
}
