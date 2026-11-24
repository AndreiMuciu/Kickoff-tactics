import { useCallback, useEffect, useRef, useState } from 'react'
import { api, ApiError } from './api'
import type { ChatMessage, FriendList, Me, Ranking, RankedView } from './api'
import { watch } from './live'
import { QUOTES } from './quotes'
import Profile from './Profile'
import Tutorial from './Tutorial'
import Daily from './Daily'
import Claim from './Claim'
import { openConsent } from './Consent'
import { ord } from './League'
import { CUPS, cupEnding } from './story'

const RESULT: Record<string, string> = { champion: '🏆 Champions', europe: '🎟 European place', survived: '😮‍💨 Mid-table', relegated: '⬇️ Relegated', sacked: '📦 Sacked' }

interface Props {
  me: Me
  challenger: string | null // opened from a daily-challenge link
  onDaily: () => void
  ranked: RankedView | null
  onRanked: (mode: 'ranked' | 'normal' | null) => void // null leaves the queue
  onParty: (f: () => Promise<unknown>) => void // runs a friends-lobby call, then refreshes
  onPlay: (fresh: boolean) => void
  onLive: (name: string) => void
  onWatchLobby: (id: string) => void // go back to a lobby you are out of, to watch it end
  onLogout: () => void
  onProfileChanged: () => void
  notify: (msg: string) => void
}

export default function Home({ me, challenger, onDaily, ranked, onRanked, onParty, onPlay, onLive, onWatchLobby, onLogout, onProfileChanged, notify }: Props) {
  const [profile, setProfile] = useState(false)
  const [claim, setClaim] = useState(false) // a guest creating an account
  // A friend whose profile is open (read-only).
  const [friendProfile, setFriendProfile] = useState<string | null>(null)
  const [list, setList] = useState<FriendList>({ friends: [], incoming: [], outgoing: [] })
  const [add, setAdd] = useState('')
  const [open, setOpen] = useState<string | null>(null)
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [draft, setDraft] = useState('')
  const [copied, setCopied] = useState(false)
  const logRef = useRef<HTMLDivElement>(null)
  // The full ranking of every manager, loaded when it is opened.
  const [board, setBoard] = useState<Ranking | null>(null)
  const [showTiers, setShowTiers] = useState(false)
  const [scope, setScope] = useState<'all' | 'friends'>('all')
  const openBoard = (sc: 'all' | 'friends') => {
    setScope(sc)
    api
      .ranking(sc)
      .then(setBoard)
      .catch(() => notify('Could not load the ranking.'))
  }
  // The short table on the home page: everybody, or just you and your friends.
  const [ladder, setLadder] = useState<'all' | 'friends'>('all')
  const [friendTop, setFriendTop] = useState<Ranking | null>(null)
  useEffect(() => {
    if (ladder !== 'friends') return
    return watch<Ranking>('/api/ranking?scope=friends', { every: 15000, onData: ({ data }) => setFriendTop(data) })
  }, [ladder])
  // While a lobby is being found, a different football quote every few seconds.
  const [quote, setQuote] = useState(() => Math.floor(Math.random() * QUOTES.length))
  const searching = !!ranked?.queued
  // Time spent searching: the server says how long, the clock ticks locally in between polls.
  const since = useRef(0)
  const [waited, setWaited] = useState(0)
  useEffect(() => {
    if (!ranked?.queued) return
    since.current = Date.now() - ranked.waitedMs
    setWaited(ranked.waitedMs)
  }, [ranked?.queued, ranked?.waitedMs])
  useEffect(() => {
    if (!searching) return
    const t = window.setInterval(() => setWaited(Date.now() - since.current), 1000)
    return () => window.clearInterval(t)
  }, [searching])
  const clock = `${Math.floor(waited / 60000)}:${String(Math.floor(waited / 1000) % 60).padStart(2, '0')}`
  useEffect(() => {
    if (!searching) return
    const t = window.setInterval(() => setQuote((q) => (q + 1 + Math.floor(Math.random() * (QUOTES.length - 1))) % QUOTES.length), 6000)
    return () => window.clearInterval(t)
  }, [searching])

  const fail = useCallback((e: unknown) => notify(e instanceof ApiError ? e.message : "We can't reach the game right now. Try again in a moment."), [notify])
  const refresh = useCallback(() => api.friends().then(setList).catch(() => {}), [])

  // Friends and the open conversation arrive over the live connection.
  useEffect(() => (me.guest ? undefined : watch<FriendList>('/api/friends', { every: 6000, onData: ({ data }) => setList(data) })), [me.guest])

  useEffect(() => {
    if (!open) return
    return watch<{ messages: ChatMessage[] }>(`/api/chat?with=${encodeURIComponent(open)}`, { every: 3000, onData: ({ data }) => setMessages(data.messages) })
  }, [open])

  useEffect(() => {
    logRef.current?.scrollTo(0, logRef.current.scrollHeight)
  }, [messages])

  const act = async (f: () => Promise<unknown>) => {
    try {
      await f()
      await refresh()
    } catch (e) {
      fail(e)
    }
  }
  const send = async (e: React.FormEvent) => {
    e.preventDefault()
    const text = draft.trim()
    if (!open || !text) return
    try {
      const m = await api.chatSend(open, text)
      setMessages((x) => [...x, m])
      setDraft('')
    } catch (err) {
      fail(err)
    }
  }
  const inviteLink = `${location.origin}/?add=${encodeURIComponent(me.name)}`
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(inviteLink)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1800)
    } catch {
      notify('Copy the link from the box.')
    }
  }
  const s = me.season
  const inSeason = s && s.phase !== 'ended'
  // The tutorial opens by itself once, for a manager who has not played yet.
  const tutorialKey = `autofootball-tutorial-${me.name}`
  const [tutorial, setTutorial] = useState(() => {
    try {
      return !!me.newPlayer && !localStorage.getItem(tutorialKey)
    } catch {
      return !!me.newPlayer
    }
  })
  const closeTutorial = useCallback(() => {
    setTutorial(false)
    try {
      localStorage.setItem(tutorialKey, '1')
    } catch {
      // private mode: it may open again next time, and can be skipped again
    }
  }, [tutorialKey])

  return (
    <div className="app">
      <header className="top">
        <h1>Kickoff Tactics</h1>
        {me.guest ? (
          <>
            <span className="chip" title="You are playing without an account">
              Guest
            </span>
            <button className="primary" onClick={() => setClaim(true)}>
              Create account
            </button>
            <button className="restart" onClick={() => setTutorial(true)} title="The rules in eight short steps">
              📖 How to play
            </button>
          </>
        ) : (
          <>
        <button className="chip chip-btn" onClick={() => setProfile(true)} title="Profile, statistics, email and password">
          Manager <b>{me.name}</b>
          {me.newAchievements > 0 && (
            <span className="ach-dot" title={`${me.newAchievements} new achievement${me.newAchievements === 1 ? '' : 's'}`}>
              🏅 {me.newAchievements}
            </span>
          )}{' '}
          ▾
        </button>
        <button className="restart" onClick={() => setTutorial(true)} title="The rules in eight short steps">
          📖 How to play
        </button>
        <button className="restart" onClick={onLogout}>
          Log out
        </button>
          </>
        )}
      </header>
      {me.guest && (
        <div className="notice">
          You are playing as a guest: your progress lives in this browser for about a week. Create an account to keep it for good, on any device, and to add friends and play lobbies of eight.{' '}
          <button className="link" onClick={() => setClaim(true)}>
            Create account →
          </button>
        </div>
      )}
      {claim && <Claim onClose={() => setClaim(false)} onDone={() => { setClaim(false); notify('Account created. Everything you played is kept.'); onProfileChanged() }} onLogin={onLogout} />}
      {!me.verified && !me.guest && (
        <div className="notice">
          {me.hasEmail ? 'Confirm your email address: open the link we mailed you.' : 'Add an email address, so a forgotten password can be reset.'}{' '}
          <button className="link" onClick={() => setProfile(true)}>
            Open profile →
          </button>
        </div>
      )}
      {profile && (
        <Profile
          onClose={() => setProfile(false)}
          onChanged={onProfileChanged}
          onDeleted={(at) => {
            notify(`Account deleted. It will be erased on ${new Date(at).toLocaleDateString()}; sign in before then to keep it.`)
            onLogout()
          }}
        />
      )}
      {tutorial && <Tutorial onClose={closeTutorial} />}
      {friendProfile && <Profile name={friendProfile} onClose={() => setFriendProfile(null)} onChanged={() => {}} />}
      <main className="home">
        <div className={`home-col ${challenger ? 'challenged' : ''}`}>
        <section className="panel hero">
          <h2>{inSeason ? 'Your season is waiting' : 'Start a season'}</h2>
          {s ? (
            <p>
              {s.phase === 'ended'
                ? `Last season: ${RESULT[s.result] ?? 'finished'}${s.cup && s.cupOut ? `, then ${cupEnding(s.cup, s.cupOut).title}` : ''}. `
                : s.cup === 'playoff'
                  ? 'Relegated: in the promotion play-off, one defeat from the sack. '
                  : s.cup
                    ? `In Europe: ${CUPS[s.cup]?.name ?? 'a European cup'}. `
                    : `Matchday ${s.round} of ${s.maxRounds}. `}
              {s.round > 1 || s.phase === 'ended' ? `${s.points} pts, ${ord(s.position)} in the league. ` : ''}
              Record {s.wins}W {s.draws ?? 0}D {s.losses}L{inSeason && (!s.cup || s.cup === 'playoff') ? `, owner's patience ${s.patience}.` : '.'}
              {s.phase === 'halftime' && ' You are at half-time.'}
            </p>
          ) : (
            <p>A league of ten clubs over nine matchdays: Europe for the top four, a play-off for the bottom two. Your first stadium fits one outfield player and a keeper.</p>
          )}
          <div className="hero-actions">
            {inSeason && (
              <button className="primary big" onClick={() => onPlay(false)}>
                Continue season ▶
              </button>
            )}
            <button className={inSeason ? 'big' : 'primary big'} title={inSeason ? 'Abandon this season and start a new one' : undefined} onClick={() => onPlay(true)}>
              {inSeason ? 'Abandon & start new' : 'New season ▶'}
            </button>
          </div>
          <ul className="how">
            <li>Each stadium level adds one outfield player. You always have a goalkeeper.</li>
            <li>Three identical players merge into one with an extra ★.</li>
            <li>Every player has a unique trait; shared styles and positions give team bonuses.</li>
            <li>At half-time you choose the team talk for the second half.</li>
            <li>Live series with a friend: you both start from scratch, sign players against a timer and meet every round until one owner loses patience.</li>
          </ul>
        </section>

        <Daily me={me} challenger={challenger} onPlay={onDaily} notify={notify} />
        </div>

        {me.guest ? (
          <section className="panel guest-box">
            <h3>Play with others</h3>
            <p>Lobbies of eight managers, ranked points and divisions, live series against friends, chat and a public profile need an account.</p>
            <p className="muted">Creating one keeps everything you played as a guest: the season, today's challenge and your achievements.</p>
            <button className="primary big" onClick={() => setClaim(true)}>
              Create account
            </button>
          </section>
        ) : (
          <>
        <section className="panel rankedbox">
          <h3>Lobby of eight</h3>
          {ranked ? (
            <>
              <div className="rating">
                <b>{ranked.rating}</b>
                <span className={`tier t-${ranked.tier.toLowerCase()}`}>{ranked.tier}</span>
                <small className="muted">
                  {ranked.games} {ranked.games === 1 ? 'lobby' : 'lobbies'} played
                </small>
                <button className="link" onClick={() => setShowTiers(true)}>
                  Divisions & points →
                </button>
              </div>
              {ranked.season && (
                <p className="season-line">
                  <b>Season {ranked.season.number}</b> · ends {new Date(ranked.season.endsAt).toLocaleDateString(undefined, { day: 'numeric', month: 'long' })} (
                  {Math.max(0, Math.ceil((ranked.season.endsAt - Date.now()) / 86400000))} days). At the end everyone keeps half their points, and ranked players get a badge
                  with the division they finished in.
                </p>
              )}
              <p className="muted">Eight managers, everyone starts from scratch with 100 owner's patience. The first two rounds are penalty shootouts between managers, with nothing at stake. After that you are paired with another survivor every round and a defeat costs patience, a little at first and more as the lobby goes on; the last one standing wins. Out early? You can stay and watch the others to the end. Everyone starts at 0 points. Ranked moves them (never below 0): low divisions pay a lot for a good finish and take little for a bad one. It looks for eight managers of your level first; after two minutes bots take the empty seats. Normal is the same game with nothing at stake; after a minute bots take the empty seats.</p>
              {ranked.watchLobby && (
                <div className="watch-lobby">
                  <span>
                    Your last lobby is still being played{ranked.watchPlace ? ` (you finished #${ranked.watchPlace})` : ''}.
                  </span>
                  <button onClick={() => onWatchLobby(ranked.watchLobby!)}>👁 Watch it to the end</button>
                </div>
              )}
              {ranked.queued ? (
                <div className="finding">
                  <div className="finding-head">
                    <b>
                      {ranked.mode === 'normal' ? 'Normal' : 'Ranked'} · found {ranked.inQueue} of 8 managers
                    </b>
                    <span className="waited" title="How long you have been searching">
                      ⏱ {clock}
                    </span>
                    <button onClick={() => onRanked(null)}>Cancel</button>
                  </div>
                  <div className="seats" aria-label={`${ranked.inQueue} of 8 found`}>
                    {Array.from({ length: 8 }, (_, i) => (
                      <i key={i} className={i < ranked.inQueue ? 'on' : ''} />
                    ))}
                  </div>
                  <small className="muted">
                    {ranked.mode === 'ranked' && ranked.waitMs > 0
                      ? `${ranked.spread < 0 ? 'Now accepting opponents of any level.' : `Looking within ±${ranked.spread} points of you; the range widens the longer you wait.`} Bots take the empty seats in ${Math.ceil(ranked.waitMs / 1000)}s.`
                      : ranked.waitMs > 0
                        ? `Still looking for people. Bots take the empty seats in ${Math.ceil(ranked.waitMs / 1000)}s.`
                        : 'Lobby is starting…'}
                  </small>
                  <blockquote key={quote}>
                    “{QUOTES[quote].text}”<cite>{QUOTES[quote].by}</cite>
                  </blockquote>
                </div>
              ) : (
                <div className="row">
                  <button className="primary big" onClick={() => onRanked('ranked')}>
                    Ranked ▶
                  </button>
                  <button className="big" onClick={() => onRanked('normal')} title="Same lobby of eight, no rating change.">
                    Normal ▶
                  </button>
                </div>
              )}
              <div className="party">
                <h4>Friends lobby</h4>
                {(ranked.invites ?? []).map((inv) => (
                  <div key={inv.id} className="friend pending">
                    <span>
                      <b>{inv.host}</b> invites you to a friends lobby
                    </span>
                    <span className="row">
                      <button className="primary" onClick={() => onParty(() => api.partyJoin(inv.id, true))}>
                        Join
                      </button>
                      <button onClick={() => onParty(() => api.partyJoin(inv.id, false))}>Decline</button>
                    </span>
                  </div>
                ))}
                {ranked.party ? (
                  <>
                    <p className="muted">
                      {ranked.party.isHost ? 'Your lobby.' : `${ranked.party.host}'s lobby.`} No bots and no points: anyone in the lobby can invite friends, the host starts it with whoever is in, two to eight managers.
                    </p>
                    {ranked.party.members.map((n) => (
                      <div key={n} className={`stand ${n === me.name ? 'you' : ''}`}>
                        <span>
                          {n}
                          {n === ranked.party!.host ? ' (host)' : ''}
                        </span>
                        <b>in</b>
                      </div>
                    ))}
                    {ranked.party.invited.map((n) => (
                      <div key={n} className="stand out">
                        <span>{n}</span>
                        <b>invited…</b>
                      </div>
                    ))}
                    {
                      <div className="invitees">
                        {list.friends
                          .filter((f) => !ranked.party!.members.includes(f.name) && !ranked.party!.invited.includes(f.name))
                          .map((f) => (
                            <button key={f.name} disabled={!f.online} title={f.online ? undefined : `${f.name} is offline.`} onClick={() => onParty(() => api.partyInvite(f.name))}>
                              ＋ {f.name}
                            </button>
                          ))}
                        {list.friends.length === 0 && <small className="muted">Add friends first, then invite them here.</small>}
                      </div>
                    }
                    <div className="row">
                      {ranked.party.isHost && (
                        <button className="primary" disabled={ranked.party.members.length < 2} title={ranked.party.members.length < 2 ? 'At least one friend has to join first.' : undefined} onClick={() => onParty(() => api.partyStart())}>
                          Start with {ranked.party.members.length} ▶
                        </button>
                      )}
                      <button onClick={() => onParty(() => api.partyLeave())}>{ranked.party.isHost ? 'Close lobby' : 'Leave lobby'}</button>
                    </div>
                  </>
                ) : (
                  <button disabled={ranked.queued} onClick={() => onParty(() => api.partyCreate())}>
                    Create a friends lobby
                  </button>
                )}
              </div>
              <div className="ladder">
                <h4>
                  Top managers
                  <button className="link" onClick={() => openBoard(ladder)}>
                    Full ranking →
                  </button>
                </h4>
                <div className="tabs small-tabs">
                  <button className={ladder === 'all' ? 'on' : ''} onClick={() => setLadder('all')}>
                    Everyone
                  </button>
                  <button className={ladder === 'friends' ? 'on' : ''} onClick={() => setLadder('friends')}>
                    Friends
                  </button>
                </div>
                {(ladder === 'all'
                  ? (ranked.top ?? []).map((t, i) => ({ rank: i + 1, name: t.name, points: t.rating, tier: t.tier }))
                  : (friendTop?.players ?? []).slice(0, 10)
                ).map((t) => (
                  <div key={t.name} className={`stand ${t.name === me.name ? 'you' : ''}`}>
                    <span>
                      {t.rank}. {t.name}
                    </span>
                    <b>
                      {t.points} <small className={`tier t-${t.tier.toLowerCase()}`}>{t.tier}</small>
                    </b>
                  </div>
                ))}
                {ladder === 'friends' && friendTop && friendTop.players.length < 2 && <small className="muted">Add friends to compare points with them.</small>}
              </div>
            </>
          ) : (
            <p className="muted">Loading…</p>
          )}
        </section>

        <section className="panel social">
          <h3>Friends</h3>
          <form
            className="row"
            onSubmit={(e) => {
              e.preventDefault()
              const name = add.trim()
              if (!name) return
              void act(async () => {
                const r = await api.friendRequest(name)
                notify(r.status === 'accepted' ? `You and ${name} are now friends.` : `Invite sent to ${name}.`)
                setAdd('')
              })
            }}
          >
            <input placeholder="Add a manager by username" value={add} onChange={(e) => setAdd(e.target.value)} maxLength={20} />
            <button>Invite</button>
          </form>
          <div className="row invite">
            <input readOnly value={inviteLink} onFocus={(e) => e.currentTarget.select()} aria-label="Invite link" />
            <button type="button" onClick={copy}>
              {copied ? 'Copied ✓' : 'Copy invite link'}
            </button>
          </div>

          {list.incoming.map((n) => (
            <div key={n} className="friend pending">
              <span>
                <b>{n}</b> wants to be friends
              </span>
              <span className="row">
                <button className="primary" onClick={() => act(() => api.friendRespond(n, true))}>
                  Accept
                </button>
                <button onClick={() => act(() => api.friendRespond(n, false))}>Decline</button>
              </span>
            </div>
          ))}
          {list.outgoing.map((n) => (
            <div key={n} className="friend">
              <span className="muted">Invite sent to {n}</span>
              <button onClick={() => act(() => api.friendRemove(n))}>Cancel</button>
            </div>
          ))}
          {list.friends.length === 0 && list.incoming.length === 0 && list.outgoing.length === 0 && <p className="muted">No friends yet. Invite someone by username or send them your link.</p>}
          {list.friends.map((f) => {
            // A live series starts both managers from scratch, so only presence matters.
            const blockLive = !f.online ? `${f.name} is offline.` : null
            return (
              <div key={f.name} className={`friend ${open === f.name ? 'open' : ''}`}>
                <span>
                  <i className={`dot ${f.online ? 'on' : ''}`} title={f.online ? 'Online' : 'Offline'} />{' '}
                  <button className="link friend-name" title={`Open ${f.name}'s profile`} onClick={() => setFriendProfile(f.name)}>
                    <b>{f.name}</b>
                  </button>
                  <small className={`presence ${f.online ? 'on' : ''}`}>{f.online ? 'online' : 'offline'}</small>
                </span>
                <span className="row">
                  <button onClick={() => setOpen(open === f.name ? null : f.name)}>💬 Chat</button>
                  <button className={blockLive ? 'off' : ''} title="A new season against this friend, live: you both start from scratch and meet every round." onClick={() => (blockLive ? notify(blockLive) : onLive(f.name))}>
                    🔴 Live series
                  </button>
                  <button
                    title="Remove friend"
                    onClick={() =>
                      act(async () => {
                        await api.friendRemove(f.name)
                        if (open === f.name) setOpen(null)
                      })
                    }
                  >
                    ✕
                  </button>
                </span>
              </div>
            )
          })}

          {open && (
            <div className="chat">
              <h4>Chat with {open}</h4>
              <div className="chat-log" ref={logRef}>
                {messages.length === 0 && <p className="muted">Say hello.</p>}
                {messages.map((m) => (
                  <p key={m.id} className={m.system ? 'sys' : m.mine ? 'mine' : 'theirs'}>
                    {m.text}
                  </p>
                ))}
              </div>
              <form className="row" onSubmit={send}>
                <input value={draft} onChange={(e) => setDraft(e.target.value)} maxLength={500} placeholder="Write a message" />
                <button className="primary">Send</button>
              </form>
            </div>
          )}
        </section>
          </>
        )}
      </main>
      {board && (
        <div className="overlay" onClick={() => setBoard(null)}>
          <div className="dialog board" onClick={(e) => e.stopPropagation()}>
            <h2>Ranking</h2>
            <div className="tabs">
              <button className={scope === 'all' ? 'on' : ''} onClick={() => openBoard('all')}>
                Everyone
              </button>
              <button className={scope === 'friends' ? 'on' : ''} onClick={() => openBoard('friends')}>
                Friends
              </button>
            </div>
            <p className="muted">
              {scope === 'friends' ? `You and ${board.total - 1} friend${board.total === 2 ? '' : 's'}` : `${board.total} manager${board.total === 1 ? '' : 's'} in the game`}
              {board.total > board.shown && ` · top ${board.shown} shown`}
              {board.myRank > 0 && (
                <>
                  {' '}
                  · you are <b>#{board.myRank}</b>
                </>
              )}
            </p>
            <div className="board-list">
              <div className="board-row head">
                <span>#</span>
                <span>Manager</span>
                <span>Points</span>
                <span>Tier</span>
                <span>Ranked lobbies</span>
              </div>
              {board.players.map((p) => (
                <div key={p.name} className={`board-row ${p.you ? 'you' : ''}`}>
                  <span>{p.rank}</span>
                  <span>
                    {p.name}
                    {p.you ? ' (you)' : ''}
                  </span>
                  <b>{p.points}</b>
                  <span className={`tier t-${p.tier.toLowerCase()}`}>{p.tier}</span>
                  <span>{p.games}</span>
                </div>
              ))}
            </div>
            <button className="primary" onClick={() => setBoard(null)}>
              Close
            </button>
          </div>
        </div>
      )}
      {showTiers && ranked && (
        <div className="overlay" onClick={() => setShowTiers(false)}>
          <div className="dialog board" onClick={(e) => e.stopPropagation()}>
            <h2>Divisions</h2>
            <p className="muted">
              You have <b>{ranked.rating}</b> points. Points gained or lost in a ranked lobby by finishing place; the table of the division you start the lobby in applies. Points never drop below 0.
            </p>
            <div className="board-list">
              <div className="tier-row head">
                <span>Division</span>
                <span>Points</span>
                {[1, 2, 3, 4, 5, 6, 7, 8].map((n) => (
                  <span key={n}>{n}.</span>
                ))}
              </div>
              {ranked.tiers.map((t) => (
                <div key={t.name} className={`tier-row ${t.name === ranked.tier ? 'you' : ''}`}>
                  <span className={`tier t-${t.name.toLowerCase()}`}>{t.name}</span>
                  <span>{t.max < 0 ? `${t.min}+` : `${t.min}–${t.max}`}</span>
                  {t.points.map((v, i) => (
                    <b key={i} className={v > 0 ? 'win' : v < 0 ? 'loss' : ''}>
                      {v > 0 ? `+${v}` : v}
                    </b>
                  ))}
                </div>
              ))}
            </div>
            <p className="muted">Matchmaking: first within ±100 points of you, 50 wider every 10 seconds, anyone after 2 minutes.</p>
            <button className="primary" onClick={() => setShowTiers(false)}>
              Close
            </button>
          </div>
        </div>
      )}
      <footer className="credits">
        Unofficial fan game, not affiliated with any player, club or league. Player photos:{' '}
        <a href="https://commons.wikimedia.org/" target="_blank" rel="noreferrer noopener">
          Wikimedia Commons
        </a>
        .{' '}
        <button className="link" onClick={openConsent}>
          Cookie settings
        </button>
      </footer>
    </div>
  )
}
