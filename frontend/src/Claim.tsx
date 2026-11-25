import { useState } from 'react'
import { api, ApiError } from './api'

// A guest keeps everything by choosing a name, an email and a password: the
// same account, now with a name, so the season, the daily result and the
// achievements stay.
export default function Claim({ onClose, onDone, onLogin }: { onClose: () => void; onDone: () => void; onLogin: () => void }) {
  const [username, setUsername] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (password !== confirm) {
      setError('The two passwords do not match.')
      return
    }
    setBusy(true)
    setError(null)
    try {
      await api.claim(username, email, password)
      onDone()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "We can't reach the game right now. Try again in a moment.")
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="overlay" onClick={onClose}>
      <div className="dialog claim" role="dialog" aria-modal="true" aria-labelledby="claim-title" onClick={(e) => e.stopPropagation()}>
        <h2 id="claim-title">Create your account</h2>
        <p className="muted">Your season, your daily result and your achievements stay with you. With an account you can also add friends, play live series and lobbies of eight, and sign in from any device.</p>
        <form className="auth" onSubmit={submit}>
          <label>
            Username
            <input value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" maxLength={20} required pattern="[A-Za-z0-9_]{3,20}" title="3-20 letters, digits or underscores" />
          </label>
          <label>
            Email
            <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="email" maxLength={254} required />
          </label>
          <label>
            Password
            <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" minLength={8} maxLength={128} required />
          </label>
          <label>
            Confirm password
            <input type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)} autoComplete="new-password" minLength={8} maxLength={128} required />
          </label>
          <small className="muted">
            At least 8 characters. We email you a link to confirm your address. See the <a href="/privacy">privacy policy</a>.
          </small>
          {error && <p className="form-error">{error}</p>}
          <button className="primary" disabled={busy}>
            Create account
          </button>
          <button type="button" className="link" onClick={onLogin} title="This guest season is left behind">
            I already have an account: log in
          </button>
        </form>
      </div>
    </div>
  )
}
