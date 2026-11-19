import { useCallback, useEffect, useState } from 'react'
import { api, ApiError } from './api'
import type { ProfileView } from './api'

const pct = (a: number, b: number) => (b > 0 ? `${Math.round((a / b) * 100)}%` : '–')

export default function Profile({ onClose, onChanged }: { onClose: () => void; onChanged: () => void }) {
  const [p, setP] = useState<ProfileView | null>(null)
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null)
  const [busy, setBusy] = useState(false)
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [again, setAgain] = useState('')
  const [email, setEmail] = useState('')
  const [emailPw, setEmailPw] = useState('')

  const load = useCallback(() => api.profile().then(setP).catch(() => setMsg({ ok: false, text: 'Could not load your profile.' })), [])
  useEffect(() => {
    void load()
  }, [load])

  const run = async (f: () => Promise<unknown>, done: string) => {
    setBusy(true)
    setMsg(null)
    try {
      await f()
      setMsg({ ok: true, text: done })
      await load()
      onChanged()
    } catch (e) {
      setMsg({ ok: false, text: e instanceof ApiError ? e.message : "We can't reach the game right now. Try again in a moment." })
    } finally {
      setBusy(false)
    }
  }
  const s = p?.stats ?? {}
  const n = (k: string) => s[k] ?? 0

  return (
    <div className="overlay" onClick={onClose}>
      <div className="dialog profile" onClick={(e) => e.stopPropagation()}>
        <h2>{p ? p.name : 'Profile'}</h2>
        {p && (
          <>
            <p className="muted">
              Manager since {new Date(p.since).toLocaleDateString()} · <b>{p.points}</b> points <span className={`tier t-${p.tier.toLowerCase()}`}>{p.tier}</span>
            </p>
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
                    setMsg({ ok: false, text: 'The two new passwords do not match.' })
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
            </div>
          </>
        )}
        {msg && <p className={msg.ok ? 'form-info' : 'form-error'}>{msg.text}</p>}
        <button onClick={onClose}>Close</button>
      </div>
    </div>
  )
}
