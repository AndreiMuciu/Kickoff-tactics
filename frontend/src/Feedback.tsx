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
// straight to the people who make the game.
export default function Feedback({ screen }: { screen: string }) {
  const [open, setOpen] = useState(false)
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
