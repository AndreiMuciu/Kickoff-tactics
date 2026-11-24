import { useState } from 'react'
import { api, ApiError } from './api'

type Mode = 'login' | 'signup' | 'forgot' | 'reset'

interface Props {
  invitedBy: string | null
  challenger: string | null // opened from a daily-challenge link
  onGuest: () => void // "Play now": a guest account, no form
  resetToken: string | null // set when the page was opened from a "reset your password" mail
  onResetDone: () => void
  onDone: (restored?: boolean) => void // restored: signing in cancelled a pending account deletion
}

export default function Auth({ invitedBy, challenger, onGuest, resetToken, onResetDone, onDone }: Props) {
  const [mode, setMode] = useState<Mode>(resetToken ? 'reset' : invitedBy ? 'signup' : 'login')
  const [username, setUsername] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [info, setInfo] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const go = (m: Mode) => {
    setMode(m)
    setConfirm('')
    setError(null)
    setInfo(null)
  }

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    // A new password is typed twice, so a typo cannot lock the manager out.
    if ((mode === 'signup' || mode === 'reset') && password !== confirm) {
      setError('The two passwords do not match.')
      return
    }
    setBusy(true)
    setError(null)
    setInfo(null)
    try {
      if (mode === 'forgot') {
        await api.forgot(email)
        setInfo('If that address has an account, a reset link is on its way. It works for 30 minutes.')
      } else if (mode === 'reset') {
        const r = await api.reset(resetToken ?? '', password)
        setPassword('')
        setConfirm('')
        setUsername(r.name)
        onResetDone()
        setMode('login')
        setInfo('Password changed. Log in with the new one.')
      } else {
        const r = mode === 'signup' ? await api.signup(username, email, password) : await api.login(username, password)
        setPassword('')
        setConfirm('')
        onDone('restored' in r && !!r.restored)
      }
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "We can't reach the game right now. Try again in a moment.")
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="center intro">
      <h1>Kickoff Tactics</h1>
      <p className="lead">Sign players, set them up on the pitch and watch them play. An auto-battler, with football.</p>
      {challenger ? (
        <p className="invite-note">
          ⚔️ <b>{challenger}</b> challenges you to today's daily challenge. Same start for both of you: can you score more points?
        </p>
      ) : (
        invitedBy && (
          <p className="invite-note">
            <b>{invitedBy}</b> invited you. Create an account and you will be connected.
          </p>
        )
      )}
      {mode !== 'reset' && (
        <div className="play-now">
          <button
            className="primary big"
            disabled={busy}
            onClick={async () => {
              setBusy(true)
              setError(null)
              try {
                await api.guest()
                onGuest()
              } catch (err) {
                setError(err instanceof ApiError ? err.message : "We can't reach the game right now. Try again in a moment.")
              } finally {
                setBusy(false)
              }
            }}
          >
            {challenger ? 'Accept the challenge ▶' : 'Play now ▶'}
          </button>
          <small className="muted">No account needed. Create one later to keep your progress and play with friends.</small>
          <span className="or">or</span>
        </div>
      )}
      <form className="auth" onSubmit={submit}>
        {(mode === 'login' || mode === 'signup') && (
          <div className="tabs">
            <button type="button" className={mode === 'login' ? 'on' : ''} onClick={() => go('login')}>
              Log in
            </button>
            <button type="button" className={mode === 'signup' ? 'on' : ''} onClick={() => go('signup')}>
              Sign up
            </button>
          </div>
        )}
        {mode === 'forgot' && <h3>Forgot your password?</h3>}
        {mode === 'reset' && <h3>Choose a new password</h3>}
        {(mode === 'login' || mode === 'signup') && (
          <label>
            Username
            <input value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" maxLength={20} required pattern="[A-Za-z0-9_]{3,20}" title="3-20 letters, digits or underscores" />
          </label>
        )}
        {(mode === 'signup' || mode === 'forgot') && (
          <label>
            Email
            <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="email" maxLength={254} required />
          </label>
        )}
        {mode !== 'forgot' && (
          <label>
            {mode === 'reset' ? 'New password' : 'Password'}
            <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete={mode === 'login' ? 'current-password' : 'new-password'} minLength={8} maxLength={128} required />
          </label>
        )}
        {(mode === 'signup' || mode === 'reset') && (
          <label>
            Confirm password
            <input type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)} autoComplete="new-password" minLength={8} maxLength={128} required />
          </label>
        )}
        {mode === 'signup' && <small className="muted">At least 8 characters. We email you a link to confirm your address.</small>}
        {mode === 'forgot' && <small className="muted">Enter the address of your account and we mail you a link to choose a new password.</small>}
        {error && <p className="form-error">{error}</p>}
        {info && <p className="form-info">{info}</p>}
        <button className="primary" disabled={busy}>
          {mode === 'signup' ? 'Create account' : mode === 'forgot' ? 'Send reset link' : mode === 'reset' ? 'Save new password' : 'Log in'}
        </button>
        {mode === 'login' && (
          <button type="button" className="link" onClick={() => go('forgot')}>
            Forgot password?
          </button>
        )}
        {(mode === 'forgot' || mode === 'reset') && (
          <button
            type="button"
            className="link"
            onClick={() => {
              if (mode === 'reset') onResetDone()
              go('login')
            }}
          >
            ← Back to log in
          </button>
        )}
      </form>
    </div>
  )
}
