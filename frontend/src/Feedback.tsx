import { useState } from 'react'
import { api, ApiError } from './api'

type Kind = 'idea' | 'bug' | 'other'
const KINDS: [Kind, string][] = [
  ['idea', '💡 Idea'],
  ['bug', '🐞 Bug'],
  ['other', '💬 Other'],
]
const MAX = 1500

// A button on every screen that opens a short form: ideas and bug reports go
// straight to the people who make the game. Once a week (`ask`, decided by the
// server) the game also asks, politely, whether there is anything to say.
export default function Feedback({ screen, ask, onAsked }: { screen: string; ask?: boolean; onAsked?: () => void }) {
  const [open, setOpen] = useState(false)
  const answer = (write: boolean) => {
    void api.feedbackLater().catch(() => {})
    onAsked?.()
    if (write) setOpen(true)
  }
  const [kind, setKind] = useState<Kind>('idea')
  const [text, setText] = useState('')
  const [busy, setBusy] = useState(false)
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null)

  const close = () => {
    setOpen(false)
    if (msg?.ok) {
      setMsg(null)
      setText('')
    }
  }
  const send = async (e: React.FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setMsg(null)
    try {
      const r = await api.feedback(kind, text.trim(), screen)
      const got = r.unlocked.map((a) => `${a.icon} ${a.name}`).join(', ')
      setMsg({ ok: true, text: `Thank you! We read every message.${got ? ` Achievement unlocked: ${got}.` : ''}` })
      setText('')
    } catch (err) {
      setMsg({ ok: false, text: err instanceof ApiError ? err.message : "We can't reach the game right now. Try again in a moment." })
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      {ask && !open && (
        <div className="overlay feedback-overlay">
          <div className="dialog feedback ask">
            <h2>💬 Got a minute?</h2>
            <p>You have been playing for a while. Is there anything you would like to tell us about the game: something you love, something that annoys you, an idea?</p>
            <div className="row center-row">
              <button onClick={() => answer(false)}>Nothing this week</button>
              <button className="primary" onClick={() => answer(true)}>
                Yes, I have something
              </button>
            </div>
            <p className="muted small">We ask at most once a week. The 💬 Feedback button is always in the corner.</p>
          </div>
        </div>
      )}
      <button className="feedback-fab" onClick={() => setOpen(true)} title="Send us an idea or report a bug">
        💬 Feedback
      </button>
      {open && (
        <div className="overlay feedback-overlay" onClick={close}>
          <form className="dialog feedback" onClick={(e) => e.stopPropagation()} onSubmit={send}>
            <h2>Help us make the game better</h2>
            <p className="muted">An idea, something that annoys you, a bug: tell us. Your username comes with the message so we can follow up.</p>
            <div className="tabs" role="radiogroup" aria-label="Kind of feedback">
              {KINDS.map(([id, label]) => (
                <button type="button" key={id} role="radio" aria-checked={kind === id} className={kind === id ? 'on' : ''} onClick={() => setKind(id)}>
                  {label}
                </button>
              ))}
            </div>
            <label className="feedback-text">
              <textarea
                value={text}
                onChange={(e) => setText(e.target.value)}
                maxLength={MAX}
                rows={6}
                required
                minLength={10}
                placeholder={kind === 'bug' ? 'What happened, and what did you expect instead?' : kind === 'idea' ? 'What would make the game better?' : 'Your message'}
              />
              <small className="muted">
                {text.length}/{MAX}
              </small>
            </label>
            {msg && <p className={msg.ok ? 'form-info' : 'form-error'}>{msg.text}</p>}
            <div className="row center-row">
              <button type="button" onClick={close}>
                Close
              </button>
              <button className="primary" disabled={busy || text.trim().length < 10}>
                Send
              </button>
            </div>
          </form>
        </div>
      )}
    </>
  )
}
