import { useCallback, useEffect, useState } from 'react'
import { api, ApiError } from './api'
import type { AchievementView, GameRecord, HistoryPage, ProfileView } from './api'
import { cupEnding } from './story'

const pct = (a: number, b: number) => (b > 0 ? `${Math.round((a / b) * 100)}%` : '–')
const day = (ms: number) =>
  new Date(ms).toLocaleDateString(undefined, {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
  })

const MODE: Record<GameRecord['mode'], string> = {
  solo: 'Solo season',
  ranked: 'Ranked lobby',
  normal: 'Normal lobby',
  friends: 'Friends lobby',
  series: 'Live series',
}
const SOLO: Record<string, string> = {
  champion: '🏆 Champions',
  europe: '🎟 European place',
  survived: '😮‍💨 Mid-table',
  relegated: '⬇️ Relegated',
  sacked: '📦 Sacked',
}

function outcome(g: GameRecord): { text: string; good: boolean } {
  switch (g.mode) {
    case 'solo':
      return {
        text: `${SOLO[g.result] ?? g.result}${g.place && g.result !== 'sacked' ? ` · ${g.place} of ${g.of}, ${g.points ?? 0} pts` : ''}${g.cup ? ` · ${cupEnding(...(g.cup.split(':') as [string, string])).title}` : ''}`,
        good: g.result !== 'sacked' && g.result !== 'relegated',
      }
    case 'series':
      return {
        text: g.result === 'won' ? `Beat ${g.opponent}` : `Lost to ${g.opponent}`,
        good: g.result === 'won',
      }
    default:
      return {
        text: g.place === 1 ? '🏆 Won the lobby' : `#${g.place} of ${g.of}`,
        good: (g.place ?? 9) <= 4,
      }
  }
}

const LEVELS = ['Locked', 'Bronze', 'Silver', 'Gold', 'Platinum', 'Diamond']

function Achievements({ list }: { list: AchievementView[] }) {
  const reached = list.reduce((n, a) => n + a.level, 0)
  const latest = (a: AchievementView) => (a.at.length ? a.at[a.at.length - 1] : 0)
  const sorted = [...list].sort((x, y) => y.level - x.level || latest(y) - latest(x))
  return (
    <>
      <p className="muted">
        {reached} of {list.length * 5} levels · Bronze the first time, then Silver, Gold, Platinum and Diamond as you repeat it
      </p>
      <div className="ach-grid">
        {sorted.map((a) => {
          const from = a.level ? a.levels[a.level - 1] : 0
          const pct = a.next ? Math.min(100, ((a.count - from) / Math.max(1, a.next - from)) * 100) : 100
          return (
            <div
              key={a.id}
              className={`ach lvl-${a.level} ${a.level ? 'got' : ''} ${a.new ? 'new' : ''}`}
              title={a.at.map((t, i) => `${LEVELS[i + 1]}: ${day(t)}`).join('\n') || a.desc}
            >
              <div className="ach-top">
                <span className="ach-icon">{a.icon}</span>
                <span className={`ach-tier t${a.level}`}>
                  {a.new ? '✨ ' : ''}
                  {LEVELS[a.level]}
                </span>
              </div>
              <b>{a.name}</b>
              <small>{a.level === 5 ? `Maxed: ${a.desc.replace(/\.$/, '')} ✔` : a.desc}</small>
              <div className="ach-pips" aria-label={`${LEVELS[a.level]}, level ${a.level} of 5`}>
                {[1, 2, 3, 4, 5].map((n) => (
                  <i key={n} className={n <= a.level ? `on t${n}` : ''} title={`${LEVELS[n]}: ${a.levels[n - 1]}`} />
                ))}
              </div>
              {a.next > 0 && (
                <div className="ach-bar" aria-label={`${a.count} of ${a.next}`}>
                  <i style={{ width: `${pct}%` }} />
                  <small>
                    {a.count}/{a.next} → {LEVELS[a.level + 1]}
                  </small>
                </div>
              )}
            </div>
          )
        })}
      </div>
    </>
  )
}

function History() {
  const [page, setPage] = useState(1)
  const [data, setData] = useState<HistoryPage | null>(null)
  const [error, setError] = useState(false)
  const [open, setOpen] = useState<string | null>(null)
  useEffect(() => {
    let alive = true
    setError(false)
    api
      .history(page)
      .then((d) => alive && setData(d))
      .catch(() => alive && setError(true))
    return () => {
      alive = false
    }
  }, [page])
  if (error) return <p className="form-error">Could not load your match history.</p>
  if (!data) return <p className="muted">Loading…</p>
  if (data.total === 0) return <p className="muted">No finished games yet. Seasons, lobbies and series appear here when they end.</p>
  return (
    <div className="history">
      {data.games.map((g) => {
        const o = outcome(g)
        return (
          <div key={g.id} className={`game-row ${open === g.id ? 'open' : ''}`}>
            <button className="game-head" onClick={() => setOpen(open === g.id ? null : g.id)} aria-expanded={open === g.id}>
              <span className="game-when">{day(g.at)}</span>
              <span className="game-mode">
                {MODE[g.mode]}
                {g.season ? <small> · S{g.season}</small> : null}
              </span>
              <b className={o.good ? 'win' : 'loss'}>{o.text}</b>
              <span className="game-record">
                {g.wins}W {g.draws ? `${g.draws}D ` : ''}{g.losses}L
              </span>
              {g.mode === 'ranked' && (
                <b className={(g.points ?? 0) >= 0 ? 'win' : 'loss'}>
                  {(g.points ?? 0) >= 0 ? '+' : ''}
                  {g.points ?? 0}
                </b>
              )}
              <span className="game-caret">{open === g.id ? '▴' : '▾'}</span>
            </button>
            {open === g.id && (
              <div className="game-matches">
                {g.matches.length === 0 && <p className="muted">No matches recorded.</p>}
                {g.matches.map((m, i) => (
                  <p key={i} className={m.won ? 'win' : m.draw ? 'draw' : 'loss'}>
                    {m.round}. {m.opponent}{' '}
                    <b>
                      {m.gf}–{m.ga}
                    </b>
                    {m.pens ? ' (pens)' : ''}
                  </p>
                ))}
              </div>
            )}
          </div>
        )
      })}
      <div className="pager">
        <button disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>
          ← Newer
        </button>
        <span>
          Page {data.page} of {data.pages} · {data.total} games
        </span>
        <button disabled={page >= data.pages} onClick={() => setPage((p) => p + 1)}>
          Older →
        </button>
      </div>
    </div>
  )
}

type Tab = 'overview' | 'achievements' | 'history' | 'account'

// Your own profile, or (name set) a friend's, which shows only what friends may see.
export default function Profile({
  name,
  onClose,
  onChanged,
  onDeleted,
}: {
  name?: string
  onClose: () => void
  onChanged: () => void
  onDeleted?: (purgeAt: number) => void // the account was deleted: leave the signed-in app
}) {
  const [p, setP] = useState<ProfileView | null>(null)
  const [tab, setTab] = useState<Tab>('overview')
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null)
  const [busy, setBusy] = useState(false)
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [again, setAgain] = useState('')
  const [email, setEmail] = useState('')
  const [emailPw, setEmailPw] = useState('')
  const [delPw, setDelPw] = useState('')
  const [delSure, setDelSure] = useState(false)

  const load = useCallback(
    () =>
      api
        .profile(name)
        .then(setP)
        .catch((e) =>
          setMsg({
            ok: false,
            text: e instanceof ApiError ? e.message : 'Could not load the profile.',
          }),
        ),
    [name],
  )
  useEffect(() => {
    void load()
  }, [load])
  // Opening your own profile counts as seeing your new achievements: they stay
  // marked as new on this visit and stop being counted on the home page.
  const unseen = !name && !!p?.achievements.some((a) => a.new)
  useEffect(() => {
    if (unseen)
      void api
        .achievementsSeen()
        .then(onChanged)
        .catch(() => {})
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [unseen])
  const fresh = p?.achievements.filter((a) => a.new).length ?? 0
  const tabs: [Tab, string][] = [
    ['overview', 'Overview'],
    ['achievements', `Achievements${fresh ? ` ✨${fresh}` : ''}`],
    ...((name
      ? []
      : [
          ['history', 'Match history'],
          ['account', 'Account'],
        ]) as [Tab, string][]),
  ]

  const run = async (f: () => Promise<unknown>, done: string) => {
    setBusy(true)
    setMsg(null)
    try {
      await f()
      setMsg({ ok: true, text: done })
      await load()
      onChanged()
    } catch (e) {
      setMsg({
        ok: false,
        text: e instanceof ApiError ? e.message : "We can't reach the game right now. Try again in a moment.",
      })
    } finally {
      setBusy(false)
    }
  }
  const s = p?.stats ?? {}
  const n = (k: string) => s[k] ?? 0

  return (
    <div className="overlay" onClick={onClose}>
      <div className="dialog profile" onClick={(e) => e.stopPropagation()}>
        <h2>{p ? p.name : (name ?? 'Profile')}</h2>
        {p && (
          <>
            <p className="muted">
              Manager since {new Date(p.since).toLocaleDateString()} · <b>{p.points}</b> points <span className={`tier t-${p.tier.toLowerCase()}`}>{p.tier}</span> · Season{' '}
              {p.season.number}: {p.seasonLobbies} ranked {p.seasonLobbies === 1 ? 'lobby' : 'lobbies'}
            </p>
            <div className="tabs" role="tablist">
              {tabs.map(([id, label]) => (
                <button key={id} role="tab" aria-selected={tab === id} className={tab === id ? 'on' : ''} onClick={() => setTab(id)}>
                  {label}
                </button>
              ))}
            </div>
            {tab === 'achievements' && <Achievements list={p.achievements} />}
            {tab === 'history' && <History />}
            {tab === 'overview' && (
              <>
                <div className="stat-grid">
                  <div>
                    <b>{n('matches')}</b>
                    <small>Matches</small>
                  </div>
                  <div>
                    <b>{pct(n('wins'), n('matches'))}</b>
                    <small>Won ({n('wins')})</small>
                  </div>
                  <div>
                    <b>
                      {n('gf')} : {n('ga')}
                    </b>
                    <small>Goals for : against</small>
                  </div>
                  <div>
                    <b>{n('matches') ? (n('gf') / n('matches')).toFixed(2) : '–'}</b>
                    <small>Goals per match</small>
                  </div>
                  <div>
                    <b>{n('lobbies')}</b>
                    <small>Lobbies ({n('ranked')} ranked)</small>
                  </div>
                  <div>
                    <b>{n('lobbyWins')}</b>
                    <small>Lobbies won</small>
                  </div>
                  <div>
                    <b>{pct(n('lobbyTop4'), n('lobbies'))}</b>
                    <small>Top 4</small>
                  </div>
                  <div>
                    <b>{n('lobbies') ? (n('placeSum') / n('lobbies')).toFixed(1) : '–'}</b>
                    <small>Average place</small>
                  </div>
                  <div>
                    <b>{n('seasons')}</b>
                    <small>Solo seasons finished</small>
                  </div>
                  <div>
                    <b>{n('titles')}</b>
                    <small>Titles</small>
                  </div>
                  <div>
                    <b>{n('series')}</b>
                    <small>Friend series</small>
                  </div>
                  <div>
                    <b>{n('seriesWins')}</b>
                    <small>Series won</small>
                  </div>
                </div>
                <p className="muted small">Counted from the day statistics were added; earlier games are not included.</p>
                <h3>Season badges</h3>
                {p.badges.length === 0 ? (
                  <p className="muted small">Play ranked this season: when it ends you keep a badge with the division you finished in.</p>
                ) : (
                  <div className="badges">
                    {[...p.badges].reverse().map((b) => (
                      <span key={b.season} className={`badge tier t-${b.tier.toLowerCase()}`} title={`${b.points} points at the end of season ${b.season}`}>
                        S{b.season} · {b.tier}
                      </span>
                    ))}
                  </div>
                )}
              </>
            )}

            {tab === 'account' && (
              <div className="profile-forms">
                <form
                  className="auth"
                  onSubmit={(e) => {
                    e.preventDefault()
                    void run(() => api.changeEmail(email, emailPw), 'Address saved. Open the link we mailed you to confirm it.').then(() => {
                      setEmail('')
                      setEmailPw('')
                    })
                  }}
                >
                  <h3>Email</h3>
                  <p>
                    {p.email ? (
                      <>
                        {p.email} <span className={p.verified ? 'win' : 'loss'}>{p.verified ? '✓ confirmed' : 'not confirmed'}</span>
                      </>
                    ) : (
                      <span className="loss">No address yet. Without one a forgotten password cannot be reset.</span>
                    )}
                  </p>
                  {p.email && !p.verified && (
                    <button type="button" disabled={busy} onClick={() => run(() => api.resendVerify(), 'Confirmation mail sent.')}>
                      Send the confirmation mail again
                    </button>
                  )}
                  <label>
                    {p.email ? 'New address' : 'Address'}
                    <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="email" maxLength={254} required />
                  </label>
                  <label>
                    Current password
                    <input type="password" value={emailPw} onChange={(e) => setEmailPw(e.target.value)} autoComplete="current-password" maxLength={128} required />
                  </label>
                  <button className="primary" disabled={busy}>
                    {p.email ? 'Change address' : 'Add address'}
                  </button>
                </form>
                <form
                  className="auth"
                  onSubmit={(e) => {
                    e.preventDefault()
                    if (next !== again) {
                      setMsg({
                        ok: false,
                        text: 'The two new passwords do not match.',
                      })
                      return
                    }
                    void run(() => api.changePassword(current, next), 'Password changed. Other devices were signed out.').then(() => {
                      setCurrent('')
                      setNext('')
                      setAgain('')
                    })
                  }}
                >
                  <h3>Password</h3>
                  <label>
                    Current password
                    <input type="password" value={current} onChange={(e) => setCurrent(e.target.value)} autoComplete="current-password" maxLength={128} required />
                  </label>
                  <label>
                    New password
                    <input type="password" value={next} onChange={(e) => setNext(e.target.value)} autoComplete="new-password" minLength={8} maxLength={128} required />
                  </label>
                  <label>
                    Confirm new password
                    <input type="password" value={again} onChange={(e) => setAgain(e.target.value)} autoComplete="new-password" minLength={8} maxLength={128} required />
                  </label>
                  <button className="primary" disabled={busy}>
                    Change password
                  </button>
                </form>
                <form
                  className="auth danger-zone"
                  onSubmit={async (e) => {
                    e.preventDefault()
                    setBusy(true)
                    setMsg(null)
                    try {
                      const r = await api.deleteAccount(delPw)
                      setDelPw('')
                      onDeleted?.(r.purgeAt)
                    } catch (err) {
                      setMsg({ ok: false, text: err instanceof ApiError ? err.message : "We can't reach the game right now. Try again in a moment." })
                    } finally {
                      setBusy(false)
                    }
                  }}
                >
                  <h3>Delete account</h3>
                  <p className="muted small">
                    Your account is hidden at once and erased after 30 days: email, password, statistics, achievements, match history, friends and chats. Until then your
                    username stays reserved and signing in brings everything back. Your name in other managers&apos; match history becomes “Former manager”, and feedback
                    you sent is kept without your name.
                  </p>
                  <label>
                    Current password
                    <input type="password" value={delPw} onChange={(e) => setDelPw(e.target.value)} autoComplete="current-password" maxLength={128} required />
                  </label>
                  <label className="check">
                    <input type="checkbox" checked={delSure} onChange={(e) => setDelSure(e.target.checked)} /> I understand that after 30 days this cannot be undone.
                  </label>
                  <button className="danger" disabled={busy || !delSure || !delPw}>
                    Delete my account
                  </button>
                </form>
              </div>
            )}
          </>
        )}
        {msg && <p className={msg.ok ? 'form-info' : 'form-error'}>{msg.text}</p>}
        <button onClick={onClose}>Close</button>
      </div>
    </div>
  )
}
