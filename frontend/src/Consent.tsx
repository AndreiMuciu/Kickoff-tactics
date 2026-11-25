import { useEffect, useState } from 'react'

// Cookie choice: "Essential only" keeps just the sign-in cookie; "Accept all"
// also loads Cloudflare Web Analytics (anonymous visit counts). The beacon is
// loaded here, only after "Accept all", so Cloudflare's automatic injection
// must stay off (Web Analytics: manual JS snippet setup). The token is public:
// it ends up in the page anyway. VITE_CF_ANALYTICS_TOKEN overrides it.

type Choice = 'essential' | 'all'
const KEY = 'autofootball-consent-v1'
const TOKEN = (import.meta.env.VITE_CF_ANALYTICS_TOKEN as string | undefined) || 'd733d0c280db48a685bfa9910a201e39'

function read(): Choice | null {
  try {
    const v = localStorage.getItem(KEY)
    return v === 'essential' || v === 'all' ? v : null
  } catch {
    return null
  }
}

let loaded = false
function loadAnalytics() {
  if (loaded || !TOKEN) return
  loaded = true
  const s = document.createElement('script')
  s.defer = true
  s.src = 'https://static.cloudflareinsights.com/beacon.min.js'
  s.dataset.cfBeacon = JSON.stringify({ token: TOKEN, spa: true })
  document.head.appendChild(s)
}

// openConsent shows the banner again, from a "Cookie settings" link.
export const openConsent = () => window.dispatchEvent(new Event('consent:open'))

export default function Consent() {
  const [choice, setChoice] = useState<Choice | null>(read)
  const [open, setOpen] = useState(() => read() === null)

  useEffect(() => {
    if (choice === 'all') loadAnalytics()
  }, [choice])
  useEffect(() => {
    const show = () => setOpen(true)
    window.addEventListener('consent:open', show)
    return () => window.removeEventListener('consent:open', show)
  }, [])

  const pick = (c: Choice) => {
    try {
      localStorage.setItem(KEY, c)
    } catch {
      /* private mode: asked again next time */
    }
    setOpen(false)
    // A loaded beacon cannot be taken back out of the page: start it fresh.
    if (c === 'essential' && loaded) location.reload()
    setChoice(c)
  }

  if (!open) return null
  return (
    <div className="cookie-bar" role="region" aria-label="Cookies">
      <span>
        We use <b>one essential cookie</b> to keep you signed in. With <b>Accept all</b> we also count visits anonymously with Cloudflare Web Analytics: no ads, no tracking across sites. Player photos come from Wikimedia. <a href="/privacy">Privacy policy</a>
      </span>
      <span className="cookie-actions">
        <button onClick={() => pick('essential')}>Essential only</button>
        <button className="primary" onClick={() => pick('all')}>
          Accept all
        </button>
      </span>
    </div>
  )
}
