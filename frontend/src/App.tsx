import { useCallback, useEffect, useState } from 'react'
import { api, ApiError } from './api'
import type { Me, Meta, RankedView, SeriesView } from './api'
import Auth from './Auth'
import Feedback from './Feedback'
import Game from './Game'
import Home from './Home'
import Series from './Series'
import { setLive, watch } from './live'
import { resetPlan } from './plan'
import { loadPhotos } from './photos'
import type { Photos } from './photos'

// An invite link looks like /?add=<username>. The name is validated here and
// again by the server; it is only ever used as a friend-request target.
function readInvite(): string | null {
  const v = new URLSearchParams(location.search).get('add')
  return v && /^[A-Za-z0-9_]{3,20}$/.test(v) ? v : null
}

// Links in our mails look like /#verify=<token> or /#reset=<token>. The token
// sits in the fragment, which the browser never sends to any server, and is
// removed from the address bar as soon as it has been read.
function readMailLink(): { kind: 'verify' | 'reset'; token: string } | null {
  const m = /^#(verify|reset)=([A-Za-z0-9_-]{20,80})$/.exec(location.hash)
  if (!m) return null
  history.replaceState(null, '', location.pathname + location.search)
  return { kind: m[1] as 'verify' | 'reset', token: m[2] }
}
const mailLink = readMailLink()

// A challenge link looks like /?c=<username>: today's daily challenge, against them.
function readChallenge(): string | null {
  const v = new URLSearchParams(location.search).get('c')
  return v && /^[A-Za-z0-9_]{3,20}$/.test(v) ? v : null
}

// The app has two addresses: "/" (the game, signed in) and "/login". Anything
// else does not exist and says so, instead of quietly showing the home page.
const ROUTES = ['/', '/login']
const knownRoute = ROUTES.includes(location.pathname.replace(/\/+$/, '') || '/')

function NotFound() {
  return (
    <div className="center">
      <h1>Kickoff Tactics</h1>
      <p className="notfound">404</p>
      <p>Offside. There is no page at this address.</p>
      <a className="button primary" href="/">
        Back to the game
      </a>
    </div>
  )
}

function CookieNotice() {
  const KEY = 'autofootball-cookie-notice'
  const [show, setShow] = useState(() => {
    try {
      return localStorage.getItem(KEY) !== '1'
    } catch {
      return true
    }
  })
  if (!show) return null
  return (
    <div className="cookie-bar" role="region" aria-label="Cookies">
      <span>
        We use <b>one cookie</b>, to keep you signed in. No ads, no tracking cookies: visits are only counted anonymously. Player photos come from Wikimedia.
      </span>
      <button
        className="primary"
        onClick={() => {
          try {
            localStorage.setItem(KEY, '1')
          } catch {
            /* private mode: the notice simply shows again next time */
          }
          setShow(false)
        }}
      >
        Got it
      </button>
    </div>
  )
}

type View = { at: 'home' } | { at: 'game'; fresh: boolean; daily?: boolean }

export default function App() {
  const [meta, setMeta] = useState<Meta | null>(null)
  const [photos, setPhotos] = useState<Photos>({})
  const [me, setMe] = useState<Me | null | undefined>(undefined) // undefined: still loading
  const [down, setDown] = useState(false)
  const [view, setView] = useState<View>({ at: 'home' })
  const [toast, setToast] = useState<string | null>(null)
  const [invite, setInvite] = useState<string | null>(readInvite)
  const [challenger] = useState<string | null>(readChallenge)
  const [live, setLive2] = useState<SeriesView | null>(null) // pending live-series invite
  const [liveId, setLiveId] = useState<string | null>(null) // series being played
  const [ranked, setRanked] = useState<RankedView | null>(null)
  const [lobbyId, setLobbyId] = useState<string | null>(null) // ranked lobby being played
  const [spectate, setSpectate] = useState(false) // opened only to watch a lobby you are out of
  const [resetToken, setResetToken] = useState<string | null>(mailLink?.kind === 'reset' ? mailLink.token : null)

  const notify = useCallback((msg: string) => {
    setToast(msg)
    window.setTimeout(() => setToast(null), 3200)
  }, [])

  const loadMe = useCallback(async () => {
    try {
      setMe(await api.me())
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) setMe(null)
      else setDown(true)
    }
  }, [])

  useEffect(() => {
    api
      .meta()
      .then((m) => {
        setMeta(m)
        void loadPhotos(m.players).then(setPhotos)
      })
      .catch(() => setDown(true))
    void loadMe()
  }, [loadMe])

  // A "confirm your address" link works whether or not you are signed in.
  useEffect(() => {
    if (mailLink?.kind !== 'verify') return
    api
      .verify(mailLink.token)
      .then(() => {
        notify('Email address confirmed.')
        void loadMe()
      })
      .catch((e) => notify(e instanceof ApiError ? e.message : 'Could not confirm the address.'))
  }, [notify, loadMe])

  // Keep the address honest: signed out means /login, signed in means /.
  // Someone signed in who opens /login is sent to the game, not shown a form.
  useEffect(() => {
    if (!knownRoute || me === undefined) return
    const want = me === null || resetToken ? '/login' : '/'
    if (location.pathname !== want) history.replaceState(null, '', want + location.search)
  }, [me, resetToken])

  // Once signed in, an invite link turns into a friend request, exactly once.
  useEffect(() => {
    if (!me || me.guest || !invite) return
    const name = invite
    setInvite(null)
    history.replaceState(null, '', challenger ? `/?c=${challenger}` : '/')
    if (name.toLowerCase() === me.name.toLowerCase()) return
    api
      .friendRequest(name)
      .then((r) => notify(r.status === 'accepted' ? `You and ${name} are now friends.` : `Invite sent to ${name}.`))
      .catch((e) => notify(e instanceof ApiError ? e.message : 'Could not send the invite.'))
  }, [me, invite, notify])

  // A guest plays alone: no live connection, lobbies or series to watch.
  const signedIn = !!me && !me.guest
  // One WebSocket while signed in; everything below is pushed through it,
  // with polling as the fallback while it is down.
  useEffect(() => {
    setLive(signedIn)
    resetPlan()
    return () => setLive(false)
  }, [signedIn])

  // Ranked: points, queue and "your lobby is ready".
  useEffect(() => {
    if (!signedIn || liveId || lobbyId) return
    return watch<RankedView>('/api/ranked', {
      every: 2000,
      onData: ({ data: v }) => {
        setRanked(v)
        if (v.lobbyId) setLobbyId(v.lobbyId)
      },
    })
  }, [signedIn, liveId, lobbyId])

  // Live series: invites and kick-offs.
  useEffect(() => {
    if (!signedIn || liveId || lobbyId) return
    return watch<SeriesView>('/api/series', {
      every: 2500,
      onData: ({ data: v }) => {
        if (v.status === 'live' && v.id) {
          setLive2(null)
          setLiveId(v.id)
        } else setLive2(v.status === 'invited' ? v : null)
      },
    })
  }, [signedIn, liveId, lobbyId])

  if (!knownRoute) return <NotFound />
  if (down)
    return (
      <div className="center">
        <h1>Kickoff Tactics</h1>
        <p>We can't reach the game right now. Please try again in a moment.</p>
        <button className="primary" onClick={() => location.reload()}>
          Try again
        </button>
      </div>
    )
  if (!meta || me === undefined)
    return (
      <div className="center">
        <h1>Kickoff Tactics</h1>
        <p className="muted">Loading…</p>
      </div>
    )
  // A reset link wins over an existing session: the password is about to change.
  if (me === null || resetToken)
    return (
      <>
        <Auth
          invitedBy={invite}
          challenger={challenger}
          onGuest={() => void loadMe()}
          resetToken={resetToken}
          onResetDone={() => {
            setResetToken(null)
            setMe(null)
          }}
          onDone={(restored) => {
            if (restored) notify('Welcome back! Your account is no longer scheduled for deletion.')
            void loadMe()
          }}
        />
        <CookieNotice />
        {toast && <div className="toast">{toast}</div>}
      </>
    )

  const goHome = () => {
    setView({ at: 'home' })
    void loadMe()
  }

  if (lobbyId)
    return (
      <>
        <Series
          id={lobbyId}
          kind="lobby"
          spectate={spectate}
          meta={meta}
          photos={photos}
          notify={notify}
          onClose={() => {
            setLobbyId(null)
            setSpectate(false)
            setRanked(null)
            setView({ at: 'home' })
            void loadMe()
          }}
        />
        <Feedback screen="lobby" />
        {toast && <div className="toast">{toast}</div>}
      </>
    )

  if (liveId)
    return (
      <>
        <Series
          id={liveId}
          kind="series"
          meta={meta}
          photos={photos}
          notify={notify}
          onClose={() => {
            setLiveId(null)
            setView({ at: 'home' })
            void loadMe()
          }}
        />
        <Feedback screen="series" />
        {toast && <div className="toast">{toast}</div>}
      </>
    )

  return (
    <>
      {view.at === 'game' ? (
        <Game key={`${view.fresh}-${!!view.daily}`} meta={meta} photos={photos} fresh={view.fresh} daily={!!view.daily} me={me} onHome={goHome} />
      ) : (
        <Home
          me={me}
          challenger={challenger}
          onDaily={() => setView({ at: 'game', fresh: false, daily: true })}
          ranked={ranked}
          onParty={(f) =>
            f()
              .then(() => api.ranked())
              .then(setRanked)
              .catch((e) => notify(e instanceof ApiError ? e.message : 'Could not reach the lobby.'))
          }
          onWatchLobby={(id) => {
            setSpectate(true)
            setLobbyId(id)
          }}
          onRanked={(mode) =>
            (mode ? api.rankedJoin(mode) : api.rankedCancel())
              .then(() => api.ranked())
              .then(setRanked)
              .catch((e) => notify(e instanceof ApiError ? e.message : 'Could not reach matchmaking.'))
          }
          notify={notify}
          onProfileChanged={loadMe}
          onPlay={(fresh) => {
            // A solo season ends the search for a lobby.
            if (ranked?.queued)
              api
                .rankedCancel()
                .then(() => notify('Lobby search cancelled: you are playing your season.'))
                .catch(() => {})
            setView({ at: 'game', fresh })
          }}
          onLogout={async () => {
            await api.logout().catch(() => {})
            setMe(null)
          }}
          onLive={(name) =>
            api
              .seriesInvite(name)
              .then(() => notify(`Invite to a live series sent to ${name}. It expires in a minute.`))
              .catch((e) => notify(e instanceof ApiError ? e.message : 'Could not send the invite.'))
          }
        />
      )}
      {live && !liveId && (
        <div className="live-bar">
          {live.role === 1 ? (
            <>
              <span>
                🔴 <b>{live.opponent}</b> invites you to a live series
              </span>
              <button className="primary" onClick={() => api.seriesRespond(true).then((r) => r.id && setLiveId(r.id)).catch((e) => notify(e instanceof ApiError ? e.message : 'Could not accept.'))}>
                Accept
              </button>
              <button onClick={() => api.seriesRespond(false).then(() => setLive2(null)).catch(() => {})}>Decline</button>
            </>
          ) : (
            <>
              <span>
                🔴 Waiting for <b>{live.opponent}</b> to accept…
              </span>
              <button onClick={() => api.seriesCancel().then(() => setLive2(null)).catch(() => {})}>Cancel</button>
            </>
          )}
        </div>
      )}
      <CookieNotice />
      {toast && <div className="toast">{toast}</div>}
      <Feedback
        screen={view.at === 'game' ? 'solo' : 'home'}
        ask={view.at === 'home' && !!me?.askFeedback}
        onAsked={() => setMe((m) => (m ? { ...m, askFeedback: false } : m))}
      />
    </>
  )
}
