import { openConsent } from './Consent'

// The privacy policy at /privacy. Keep it true to the code: every retention
// period below is a constant in the backend (store.SessionTTL, MessageTTL,
// deleteGrace, the TTL indexes in store/mongo.go) or the frontend (Consent).

const CONTACT = 'contact@kickoff-tactics.com'

export default function Privacy() {
  return (
    <div className="legal">
      <a className="legal-back" href="/">
        ← Back to the game
      </a>
      <h1>Privacy policy</h1>
      <p className="muted">Last updated: 25 November 2026</p>

      <p>
        Kickoff Tactics is a free browser game made as a hobby project. This page says what data the game keeps about you, why, for how long, and who else sees it. Questions or
        requests: <a href={`mailto:${CONTACT}`}>{CONTACT}</a>.
      </p>

      <h2>What we keep</h2>
      <ul>
        <li>
          <b>Your account:</b> username, email address, whether the address is confirmed, and your password as an Argon2id hash (never the password itself). A guest account
          (“Play now”) has only a generated name.
        </li>
        <li>
          <b>Your game:</b> seasons, the daily challenge, lobbies and series, match history, statistics, achievements, ranked points and your team plan.
        </li>
        <li>
          <b>Social:</b> your friends, friend and game invitations, and chat messages with friends.
        </li>
        <li>
          <b>Feedback</b> you send from the game, with your username and the screen it was sent from.
        </li>
        <li>
          <b>When you were last seen</b>, to show friends whether you are online.
        </li>
      </ul>
      <p>
        We do not keep access logs with IP addresses. Your IP address is used for a few minutes to limit how many requests and sign-in attempts one address can make. Error logs
        contain no personal data.
      </p>

      <h2>Why (legal basis)</h2>
      <ul>
        <li>To run the game and your account: necessary to provide the service you asked for (Art. 6(1)(b) GDPR).</li>
        <li>To keep it secure and fair (rate limits, sign-in throttling): our legitimate interest (Art. 6(1)(f)).</li>
        <li>Visit statistics: only with your consent (Art. 6(1)(a)), see below.</li>
      </ul>
      <p>No ads, no selling of data, no profiling.</p>

      <h2>Cookies and local storage</h2>
      <ul>
        <li>
          <b>footy_sid</b>, an essential cookie that keeps you signed in. It holds a random token and expires after 7 days.
        </li>
        <li>
          <b>Local storage</b> in your browser remembers small settings: your cookie choice, tutorial seen, cached player photo addresses. It never leaves your device.
        </li>
        <li>
          <b>Visit statistics</b>: only if you choose “Accept all”, the page loads Cloudflare Web Analytics, which counts page views without cookies and without tracking you
          across sites. With “Essential only” it is not loaded.{' '}
          <button className="link" onClick={openConsent}>
            Change your choice
          </button>
        </li>
      </ul>

      <h2>How long</h2>
      <ul>
        <li>Sign-in sessions: 7 days. Email confirmation links: 24 hours; password reset links: 30 minutes.</li>
        <li>Chat messages: 30 days. Finished lobbies: 7 days; live series: 1 day.</li>
        <li>Guest accounts that are never turned into a real account: erased after 30 days.</li>
        <li>Everything else: until you delete your account.</li>
      </ul>

      <h2>Deleting your account</h2>
      <p>
        Profile → Account → Delete account. The account is hidden at once and erased for good 30 days later; signing in during those 30 days restores it. Erasing removes your
        email, password, history, statistics, friends and chats. Your name in other managers' match history becomes “Former manager #…”, and feedback you sent stays without
        your name.
      </p>

      <h2>Who else processes data</h2>
      <ul>
        <li>
          <b>Cloudflare</b>: hosts the website and passes requests to our server (it sees your IP address while doing so), and runs the visit statistics if you accept them.
        </li>
        <li>
          <b>Hetzner Online</b> (Germany): the server the game runs on.
        </li>
        <li>
          <b>MongoDB Atlas</b>: the database.
        </li>
        <li>
          <b>Zoho Mail</b>: sends the emails (address confirmation, password reset, account notices).
        </li>
        <li>
          <b>Wikimedia</b>: player photos are loaded by your browser straight from Wikipedia and Wikimedia Commons, which see your IP address like any website you visit. We send
          them no account data.
        </li>
      </ul>
      <p>Where one of them processes data outside the EU, it does so under the European Commission's standard contractual clauses or the EU-US Data Privacy Framework.</p>

      <h2>Your rights</h2>
      <p>
        You can ask for a copy of your data, have it corrected or erased, ask us to limit or stop processing it, and take it elsewhere. Write to{' '}
        <a href={`mailto:${CONTACT}`}>{CONTACT}</a>; we answer within a month. You can also complain to the data protection authority of your country.
      </p>

      <h2>Children</h2>
      <p>The game is meant for players aged 16 and over. If you are younger, ask a parent before creating an account.</p>

      <h2>Changes</h2>
      <p>If this policy changes in a way that matters, the date above changes and signed-in players are told in the game.</p>

      <a className="button primary" href="/">
        Back to the game
      </a>
    </div>
  )
}
