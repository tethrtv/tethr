import { Link } from '@tanstack/react-router'
import type { ReactNode } from 'react'
import { Plain } from '../components/Plain'
import { TellySet } from '../components/Telly'
import trouble from '../components/Trouble.module.css'
import home from './Home.module.css'
import styles from './Legal.module.css'

const termsChanged = '10 November 2026'
const privacyChanged = '10 November 2026'
const developerChanged = '10 November 2026'
const contact = (
  <a href="mailto:legal@tethr.tv" className={home.inline}>
    legal@tethr.tv
  </a>
)
const reports = (
  <a href="mailto:reports@tethr.tv" className={home.inline}>
    reports@tethr.tv
  </a>
)

function Page({
  title,
  lead,
  updated,
  drawing,
  other,
  children,
}: {
  title: string
  lead: ReactNode
  updated: string
  drawing: ReactNode
  other: ReactNode
  children: ReactNode
}) {
  return (
    <Plain wide end={other}>
      <article className={styles.legal}>
        <title>{`${title} · Tethr`}</title>
        <header className={styles.head}>
          <div className={styles.drawing}>{drawing}</div>
          <div className={home.head}>
            <h1 className={home.title}>{title}</h1>
            <p className={home.lead}>{lead}</p>
            <p className={styles.updated}>Last changed {updated}</p>
          </div>
        </header>
        {children}
        <p className={styles.company}>
          Ellisn Ltd, registered in England and Wales, company number 17155439. Registered office: 167-169 Great
          Portland Street, London W1W 5PF.
        </p>
      </article>
    </Plain>
  )
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className={styles.section}>
      <h2 className={home.sectionTitle}>{title}</h2>
      {children}
    </section>
  )
}

export function Terms() {
  return (
    <Page
      title="Terms of use"
      updated={termsChanged}
      lead="The deal for using Tethr, in plain words. It’s free, your history is yours, and there are a few things you can’t do."
      drawing={<SmallPrint />}
      other={
        <Link to="/privacy" className={home.inline}>
          Privacy
        </Link>
      }
    >
      <Section title="About these terms">
        <p>
          These terms are an agreement between you and Ellisn Ltd, a company registered in England and Wales, which runs
          the service at tethr.tv. “We” on this page means Ellisn Ltd. Tethr is also open source, and if you run your
          own copy, these terms don’t apply to it. The code’s own licence does. You can reach us at {contact}.
        </p>
      </Section>

      <Section title="Your account">
        <p>
          You need to be 18 or over to make an account. You can add profiles for other people in your household,
          including kids profiles, and you’re responsible for how those profiles are used.
        </p>
        <p>
          Your passkeys and sign-in emails are how Tethr knows it’s you, so keep them to yourself. If you think someone
          else has got in, sign them out from your account page and tell us.
        </p>
      </Section>

      <Section title="Your history is yours">
        <p>
          What you watch, rate and list belongs to you. You can export all of it from your account page at any time, for
          free, and deleting your account deletes it. You let us store it and show it to the people and apps you choose,
          so the service can work. We don’t use it for anything else. The <Link to="/privacy">privacy notice</Link> has
          the details.
        </p>
      </Section>

      <Section title="Apps you connect">
        <p>
          A player or media server you connect can read and change the watch data of the profile you picked. You can
          disconnect any of them from your account page, which stops them logging and never touches your history. Most
          apps are made by other people. Our developer terms set rules for what they can do with your data, and an app
          may have its own terms as well.
        </p>
        <p>
          Building an app of your own? Our <Link to="/developer-terms">developer terms</Link> apply to it, and you’ll
          need to register it before people can connect it.
        </p>
      </Section>

      <Section title="What you can’t do">
        <p>
          Some of what you write can be seen by other people: names and usernames, and the names, descriptions and notes
          of lists you share. Don’t use any of it to:
        </p>
        <ul className={styles.list}>
          <li>
            post anything illegal, including sexual material involving children, terrorist content, or anything that
            encourages suicide or self-harm
          </li>
          <li>threaten, harass or pretend to be someone else</li>
          <li>share someone’s personal details without their say-so</li>
          <li>advertise, spam, or link to scams</li>
        </ul>
        <p>
          And don’t try to get into accounts that aren’t yours, get round rate limits, overload the service, scrape the
          site, or copy out other people’s data in bulk.
        </p>
      </Section>

      <Section title="Reporting something">
        <p>
          If you see a list that breaks these rules, use Report on the list. For anything else, email {reports} with a
          link to it and what’s wrong. We read every report, usually within a few days, and a reported list stops
          showing up when people browse until we have.
        </p>
        <p>
          We can hide or remove anything that breaks these rules, and suspend or close accounts that do it again or do
          something seriously harmful. If we remove something of yours, we’ll tell you why, unless the law says we
          can’t. If you think we got it wrong, reply to that email and we’ll look again. Where the law requires it, we
          report illegal content to the authorities.
        </p>
      </Section>

      <Section title="The service">
        <p>
          Tethr is in beta. We back it up every day and work to keep it running, but it comes as it is, with no promise
          that it will always be there or never make a mistake. If your history matters to you, keep an export.
        </p>
        <p>
          Features may change or go. If Tethr ever closes, we’ll give at least 30 days’ notice and keep exports working
          until the end.
        </p>
        <p>
          Film and show details come from Wikidata and TMDB. Tethr uses the TMDB API but isn’t endorsed or certified by
          TMDB.
        </p>
      </Section>

      <Section title="If something goes wrong">
        <p>
          As far as the law allows, we aren’t liable for losses that come from using Tethr. Nothing here limits
          liability the law doesn’t let us limit, such as for fraud, or for death or injury caused by negligence, and
          you keep all your rights as a consumer.
        </p>
      </Section>

      <Section title="Leaving, and changes">
        <p>
          You can delete your account whenever you like from your account page. We can close an account that breaks
          these terms, as described above.
        </p>
        <p>
          If these terms change in a way that matters, we’ll email you at least 30 days before the change takes effect.
          Small fixes, like typos or clearer wording, apply straight away. A change the law requires, or one that’s
          urgent for security, can apply sooner, and we’ll say why. The date at the top shows when they last changed.
        </p>
        <p>
          English law applies to these terms, and the courts of England and Wales can hear any dispute. If you live in
          Scotland or Northern Ireland, you can use your own courts too.
        </p>
      </Section>
    </Page>
  )
}

export function DeveloperTerms() {
  return (
    <Page
      title="Developer terms"
      updated={developerChanged}
      lead="The deal for building an app on Tethr. It’s free for any app, paid or not, as long as you treat people’s data, and Tethr’s, fairly."
      drawing={<SmallPrint />}
      other={
        <Link to="/developers" className={home.inline}>
          Developers
        </Link>
      }
    >
      <Section title="About these terms">
        <p>
          These terms are an agreement between Ellisn Ltd (“we”) and you, the person or company behind an app that uses
          Tethr’s API. They apply when you register an app or use the API. People who connect your app to Tethr have
          agreed to our <Link to="/terms">terms of use</Link> too. If you run your own copy of Tethr’s open-source code,
          these terms don’t apply to it. You can reach us at {contact}.
        </p>
      </Section>

      <Section title="Using the API">
        <p>
          The API is free for any app, whether you charge for it or not. Register each app on the{' '}
          <Link to="/developers">Developers page</Link>.
        </p>
        <p>
          We set the rate limits and can change them. If you need more, ask us, and tell us what your app does and who’s
          behind it.
        </p>
      </Section>

      <Section title="People’s data">
        <p>
          Someone connecting your app lets it read and change their watch data. Use it only to run your app for that
          person, and keep it only as long as your app needs it for that. Delete it when they ask or disconnect your
          app, or when you no longer need it. Don’t sell it, or share it with anyone they haven’t asked you to, other
          than services that run your app for you, like hosting.
        </p>
        <p>
          Your app can also show that person what’s public on Tethr, like other people’s public lists and usernames.
          Keep it only as long as you need it for that.
        </p>
        <p>
          Don’t use anyone’s data to build a profile of a person, for ads, tracking or anything else. Recommendations
          for the person using your app are fine.
        </p>
      </Section>

      <Section title="What you can’t do">
        <ul className={styles.list}>
          <li>sell or license data from Tethr</li>
          <li>build a catalogue or dataset from Tethr to offer to others</li>
          <li>scrape the site, or get round rate limits</li>
          <li>train AI models on data from Tethr</li>
          <li>send data from Tethr to an AI service, except to run your app for the person using it</li>
          <li>suggest Tethr made, endorses or checked your app</li>
        </ul>
        <p>None of these terms limit data Tethr publishes under an open licence. That follows its own licence.</p>
      </Section>

      <Section title="Credit">
        <p>
          Say your app uses Tethr somewhere people can find it, such as its settings or about screen, with a link to
          tethr.tv.
        </p>
      </Section>

      <Section title="If something goes wrong">
        <p>
          For most problems, we’ll email you and give you 14 days to put it right. If it isn’t fixed by then, we can
          suspend your app’s access.
        </p>
        <p>
          For anything serious, such as scraping, selling data from Tethr, building a catalogue or dataset from it to
          offer to others, putting people’s privacy at risk or straining the service, we can suspend access straight
          away. Either way, we can ask you to delete data from Tethr that you hold against these terms, and you must.
        </p>
        <p>
          The API comes as it is, with no promise that it will always be there or never make a mistake. As far as the
          law allows, we aren’t liable for losses that come from using it. Nothing here limits liability the law doesn’t
          let us limit, such as for fraud.
        </p>
      </Section>

      <Section title="Changes">
        <p>
          If these terms change in a way that affects you, we’ll email the address on your account at least 30 days
          before. Small fixes, like typos or clearer wording, apply straight away. A change the law requires, or one
          that’s urgent for security, can apply sooner, and we’ll say why. The date at the top shows when they last
          changed.
        </p>
        <p>English law applies to these terms, and the courts of England and Wales can hear any dispute.</p>
      </Section>
    </Page>
  )
}

const kept = [
  {
    what: 'Your account',
    detail: 'Your email address, and the public half of each passkey. Tethr never sees the private half.',
    until: 'Until you delete your account',
  },
  {
    what: 'Profiles',
    detail: 'The name, username, colour and settings of each profile in your household.',
    until: 'Until the profile is deleted',
  },
  {
    what: 'Watch data',
    detail:
      'What you’ve watched and when, where you stopped, ratings, your watchlist and lists, hidden shows and episode orders.',
    until: 'Until you delete it, or your account',
  },
  {
    what: 'Signing in',
    detail: 'Your sessions with the name of the browser or app, and a log of sign-ins and exports.',
    until: 'Sessions 30 days, the log 90 days',
  },
  {
    what: 'Connected apps',
    detail: 'Which apps you’ve allowed, for which profile, and when each was last used.',
    until: 'Until you disconnect them',
  },
  {
    what: 'Media servers',
    detail: 'The server’s name, the names of its users who send plays, and plays waiting to be matched to a profile.',
    until: 'Waiting plays a week, the rest until you remove the server',
  },
  {
    what: 'Imports',
    detail: 'The history read from an export file you upload, never the file itself.',
    until: 'Cleared once applied, or after 24 hours if you don’t go ahead',
  },
  {
    what: 'Invites',
    detail: 'The invites you make, who used each one, and who invited you.',
    until: 'Until either account is deleted',
  },
  {
    what: 'Following',
    detail: 'Which public lists each profile follows.',
    until: 'Until you stop following',
  },
  {
    what: 'Reports',
    detail: 'Reports you send about a list: the reason, your note, and which profile sent it.',
    until: 'A year after we’ve dealt with it',
  },
  {
    what: 'Backups',
    detail: 'Encrypted copies of the database, so nothing is lost if the server fails.',
    until: '30 days',
  },
]

export function Privacy() {
  return (
    <Page
      title="Privacy"
      updated={privacyChanged}
      lead={
        <>
          What Tethr keeps about you, why, and for how long. The short version: your history is{' '}
          <strong>private unless you share it</strong>, and it’s never sold or used for ads.
        </>
      }
      drawing={<DoNotDisturb />}
      other={
        <Link to="/terms" className={home.inline}>
          Terms
        </Link>
      }
    >
      <Section title="Who’s responsible">
        <p>
          Ellisn Ltd runs Tethr and is responsible for your data under UK data protection law. Questions and requests go
          to {contact}.
        </p>
      </Section>

      <Section title="What we keep">
        <dl className={styles.kept}>
          {kept.map((k) => (
            <div key={k.what} className={styles.row}>
              <dt>{k.what}</dt>
              <dd>{k.detail}</dd>
              <dd className={styles.until}>{k.until}</dd>
            </div>
          ))}
        </dl>
        <p>
          We don’t store your IP address. The server uses it for a moment to stop abuse, and our network provider sees
          it on the way through. There are no analytics, no ad trackers and no third-party cookies.
        </p>
      </Section>

      <Section title="Why we keep it">
        <p>
          Most of it is simply what Tethr is: you asked us to keep your history and share it with your apps, and we
          can’t do that without storing it. The sign-in log, rate limits and backups are there to keep your account and
          the service safe, which is in both our interests. If the law asks us to keep or hand over something, we do
          what it requires and no more.
        </p>
      </Section>

      <Section title="Who else sees it">
        <p>
          <strong>Your household.</strong> The account owner can see and manage every profile. Others in the household
          see the lists you share with them.
        </p>
        <p>
          <strong>People you share with.</strong> Anyone can find a list you make public, along with your username,
          including through other people’s apps. If your profile is public, they see your name too, but profiles are
          private until you say otherwise. When you follow someone’s list, they can see your username, and your name if
          your profile is public.
        </p>
        <p>
          <strong>People you invite.</strong> They see your username when they use your invite, and you see theirs once
          they’ve joined.
        </p>
        <p>
          <strong>Everyone on Tethr, as a count.</strong> Popular on Tethr lists the films and shows played by the most
          households in the last week, with a rough count. A title only appears once at least 10 households have played
          it, households only count after two weeks here, and it never says who. Kids’ and guest profiles aren’t
          counted, and the account owner can leave out any profile, their own included, by editing it on the account
          page.
        </p>
        <p>
          <strong>Reports.</strong> If you report a list, only we see that you did. The list’s owner is never told who
          reported it.
        </p>
        <p>
          <strong>Apps you connect.</strong> They see the profile you connected them to, until you disconnect them.
        </p>
        <p>
          <strong>The companies that help run Tethr.</strong> We use a hosting provider for the server and database, a
          network provider that serves the site and keeps our encrypted backups, and an email service for sign-in
          emails. They only handle your data to run Tethr for us. Some of them store it outside the UK, and when they
          do, it’s protected the way UK law requires.
        </p>
        <p>
          <strong>TMDB.</strong> Posters and stills load straight from TMDB’s image servers, so TMDB sees your IP
          address when your browser fetches one. We never send TMDB anything about you.
        </p>
        <p>Nobody else. We don’t sell your data or share it for advertising.</p>
      </Section>

      <Section title="Cookies">
        <p>
          There’s one cookie, which keeps you signed in for up to 30 days. The site also remembers a few choices in your
          browser, such as which profile you’re watching as and how you like lists laid out. None of it tracks you, so
          there’s no cookie banner.
        </p>
      </Section>

      <Section title="Kids profiles">
        <p>
          Accounts are for adults, who can add kids profiles for children in their household. A kids profile keeps the
          same kind of watch data, it’s private, it can’t make public lists, and it can’t browse or follow lists from
          outside the household.
        </p>
      </Section>

      <Section title="Your rights">
        <p>
          You can see, download, correct and delete your data yourself: export it from your account page, edit it where
          it appears, and delete it or your whole account at any time. You can also ask us to stop using it in a
          particular way, or for a copy of anything the export doesn’t cover. Email {contact} and we’ll reply within a
          month.
        </p>
        <p>
          If you’re unhappy with how we’ve handled your data, you can complain to the Information Commissioner’s Office
          at <a href="https://ico.org.uk/make-a-complaint/">ico.org.uk</a>. We’d like the chance to put it right first.
        </p>
      </Section>

      <Section title="Keeping it safe">
        <p>
          Tethr uses passkeys and emailed codes, so there are no passwords to leak. Sign-in tokens are stored scrambled,
          backups are encrypted, and everything travels over HTTPS. If something does go wrong, we’ll tell you and the
          ICO when the law says we must.
        </p>
      </Section>

      <Section title="Changes">
        <p>
          If this notice changes in a way that matters, we’ll email you at least 30 days before the change takes effect.
          Small fixes, like typos or clearer wording, apply straight away. A change the law requires, or one that’s
          urgent for security, can apply sooner, and we’ll say why. The date at the top shows when it last changed.
        </p>
      </Section>
    </Page>
  )
}

function SmallPrint() {
  const rows = [52, 76, 100, 124, 148, 172, 196]
  return (
    <svg className={trouble.telly} viewBox="0 0 320 240" role="img" aria-hidden="true">
      <g className={`${trouble.lines} ${trouble.draw}`}>
        <path
          pathLength={1}
          d="M30 30l196-12q8 0 9 8l12 192q0 8-8 9l-196 10q-8 0-9-8L22 39q0-8 8-9z"
          className={trouble.body}
        />
      </g>
      <path d="M70 99q56-5 118-3" className={`${styles.highlight} ${trouble.label}`} />
      <g className={`${trouble.lines} ${trouble.draw}`}>
        {rows.map((y, i) => (
          <path
            key={y}
            pathLength={1}
            d={`M44 ${y - i}h18M72 ${y - i}h${112 - (i % 3) * 24}`}
            className={styles.print}
          />
        ))}
        <circle pathLength={1} cx="214" cy="146" r="46" className={trouble.body} />
        <path pathLength={1} d="M248 178l40 40" className={styles.handle} />
      </g>
      <g className={trouble.label}>
        <text x="214" y="141" textAnchor="middle" className={styles.lensText}>
          NO
        </text>
        <text x="214" y="165" textAnchor="middle" className={styles.lensText}>
          CATCH
        </text>
      </g>
    </svg>
  )
}

function DoNotDisturb() {
  const folds = [86, 106, 124]
  return (
    <svg className={trouble.telly} viewBox="0 0 332 250" role="img" aria-hidden="true">
      <TellySet />
      <g className={`${trouble.lines} ${trouble.draw}`}>
        <path pathLength={1} d="M66 96q36 4 77 0q-4 56 1 112q-38 4-76 0q4-56-2-112z" className={trouble.body} />
        <path pathLength={1} d="M143 96q40 4 78-2q-2 58 2 112q-40 6-78 2q-6-56-2-112z" className={trouble.body} />
        {folds.map((x) => (
          <path key={x} pathLength={1} d={`M${x} 100q-3 52 1 104M${x + 80} 100q3 52-1 104`} className={styles.fold} />
        ))}
        <path pathLength={1} d="M62 94h166" />
      </g>
      <g className={trouble.label}>
        <path d="M218 25l18 42" className={trouble.lines} />
        <g transform="rotate(8 268 71)">
          <rect x="226" y="48" width="86" height="46" rx="4" className={trouble.band} />
          <circle cx="238" cy="71" r="3.5" className={trouble.lines} />
          <text x="276" y="68" textAnchor="middle" className={styles.tagText}>
            DO NOT
          </text>
          <text x="276" y="84" textAnchor="middle" className={styles.tagText}>
            DISTURB
          </text>
        </g>
      </g>
    </svg>
  )
}
