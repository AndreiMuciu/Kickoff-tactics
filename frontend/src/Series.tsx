import { useEffect, useRef, useState } from 'react'
import { api, ApiError } from './api'
import type { Action, MatchData, Meta, SeriesView } from './api'
import ClubBoard, { Shootout, StatsTable } from './ClubBoard'
import { watch } from './live'
import { MatchMarket } from './Market'
import MatchView from './MatchView'
import type { Photos } from './photos'

interface Props {
  id: string
  kind: 'series' | 'lobby' // a duel with a friend, or a ranked lobby of eight
  meta: Meta
  photos: Photos
  onClose: () => void
  notify: (msg: string) => void
  spectate?: boolean // opened from home only to watch a lobby you are out of
}

const emptyStats = () => ({ shots: 0, xg: 0, passes: 0, tackles: 0, poss: 0 })
// A penalty round streams the kicks alone: its frames start where a full
// match would end, so the replay is told it begins after the second half.
const newMatch = (pens = false): MatchData => ({ half: pens ? 3 : 1, roster: [], frames: [], events: [], score: [0, 0], stats: [emptyStats(), emptyStats()], names: ['', ''] })

// Series is a live season against a friend. Everything runs on the server's
// clock: the transfer window, the match and the break. This screen is sent the
// state once a second, shows the phase the server reports and plays match frames at the
// moment the server says they belong to, so both managers stay in step.
export default function Series({ id, kind, meta, photos, onClose, notify, spectate }: Props) {
  const lobby = kind === 'lobby'
  // Both kinds of live game speak the same protocol on different routes.
  const net = lobby
    ? { action: api.lobbyAction, order: api.lobbyOrder, leave: api.lobbyLeave }
    : { action: api.seriesAction, order: api.seriesOrder, leave: api.seriesLeave }
  const [view, setView] = useState<SeriesView | null>(null)
  const [busy, setBusy] = useState(false)
  const [, redraw] = useState(0)
  // One replay object per round; its arrays grow in place while the match is on.
  const match = useRef<{ round: number; key: string; data: MatchData }>({ round: 0, key: '', data: newMatch() })
  const sync = useRef({ offset: 0, ready: false, kickoff: 0, half: 30000, brk: 20000, pen: 0, penRound: false })
  // Lobby: the seat whose match (and board) is being watched; -1 is your own.
  const [watching, setWatching] = useState(-1)
  // Out of the lobby but staying to watch the others.
  const [stay, setStay] = useState(!!spectate)
  // Time of my last transfer move: a poll sent before it carries an older club
  // and must not overwrite the fresher answer (a sold player would reappear).
  const lastMove = useRef(0)

  useEffect(() => {
    const path = `${lobby ? '/api/lobby' : '/api/series'}?id=${encodeURIComponent(id)}${watching >= 0 ? `&watch=${watching}` : ''}`
    match.current = { round: 0, key: '', data: newMatch() }
    const stop = watch<SeriesView>(path, {
      every: 1000,
      // Where this screen is in the frame stream; the server continues from there.
      game: () => ({ round: match.current.round, from: match.current.round ? match.current.data.frames.length : 0 }),
      onData: ({ data: v, sentAt, pushed }) => {
        // Polled: server time at the middle of the round trip. Pushed: on arrival.
        const offset = v.serverNow - (pushed ? sentAt : (sentAt + Date.now()) / 2)
        const sy = sync.current
        sy.offset = sy.ready ? sy.offset * 0.7 + offset * 0.3 : offset
        sy.ready = true
        sy.kickoff = v.kickoffAt
        sy.half = v.halfMs
        sy.brk = v.breakMs
        sy.pen = v.penTicks
        sy.penRound = !!v.penRound
        const key = `${v.round}:${v.watching ?? -1}`
        if (key !== match.current.key) match.current = { round: v.round, key, data: newMatch(!!v.penRound) }
        const d = match.current.data
        if (v.roster && d.roster.length === 0) {
          d.roster.push(...v.roster)
          d.names = v.names
        }
        if (v.roster && v.from === d.frames.length) {
          d.frames.push(...v.frames)
          d.events!.push(...v.events)
        }
        // An answer from before my last transfer move carries an older club. A
        // pushed one may have been computed just before it, so allow a moment.
        const stale = (pushed ? sentAt - 400 : sentAt) < lastMove.current
        setView((old) => (old?.my && v.my && stale ? { ...v, my: old.my } : v))
      },
      onError: (status) => {
        if (status === 404) {
          notify('That live game is no longer available.')
          onClose()
        }
      },
    })
    const t = window.setInterval(() => redraw((n) => n + 1), 250) // timers
    return () => {
      stop()
      window.clearInterval(t)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id, watching])

  const serverNow = () => Date.now() + sync.current.offset
  // Maps server time to a frame position: first half, a pause, then the second half.
  const clock = () => {
    const { kickoff, half, brk, penRound, pen } = sync.current
    const e = serverNow() - kickoff
    const per = meta.halfTicks
    if (e <= 0) return 0
    // A penalty round is the shootout alone, at the pace of a match.
    if (penRound) return Math.min(Math.max(0, pen) - 0.01, (e / half) * per)
    if (e < half) return (e / half) * per
    if (e < half + brk) return per - 0.01
    if (e < 2 * half + brk) return per + ((e - half - brk) / half) * per
    // After the final whistle a shootout, if there is one, runs at the same pace.
    return Math.min(2 * per + Math.max(0, pen) - 0.01, 2 * per + ((e - 2 * half - brk) / half) * per)
  }

  // Out and watching: follow a manager still in, the leader first, and move
  // on when the one being watched goes out.
  const outNow = lobby && !!view && view.status === 'live' && !view.alive && !!view.place
  const watchedAlive = !!view?.standings?.find((st) => st.seat === watching)?.alive
  const leader = view?.standings?.find((st) => st.alive && !st.you)?.seat
  useEffect(() => {
    if (outNow && stay && !watchedAlive && leader !== undefined) setWatching(leader)
  }, [outNow, stay, watchedAlive, leader])

  if (!view || !view.my)
    return (
      <div className="center">
        <h1>Kickoff Tactics</h1>
        <p className="muted">{lobby ? 'Joining the lobby…' : 'Joining the live series…'}</p>
      </div>
    )

  const spectating = lobby && (view.watching ?? -1) >= 0
  const you = view.role // the side of the club whose match is shown
  const left = Math.max(0, Math.ceil((view.phaseEndsAt - serverNow()) / 1000))
  const inMatch = view.phase === 'countdown' || view.phase === 'h1' || view.phase === 'halftime' || view.phase === 'h2' || view.phase === 'shootout' || view.phase === 'result'
  // The market stays open during the match; only your own seat can buy.
  const marketOpen = view.status === 'live' && (!lobby || !!view.alive) && view.phase !== 'done' && view.phase !== 'waiting'
  const me = view.standings?.find((s) => s.you)
  const out = lobby && view.status === 'live' && !view.alive && !!view.place
  const data = match.current.data

  const act = async (a: Action) => {
    if (busy) return
    setBusy(true)
    try {
      const my = await net.action(id, a)
      lastMove.current = Date.now()
      setView((v) => (v ? { ...v, my } : v))
    } catch (e) {
      notify(e instanceof ApiError ? e.message : "We can't reach the game right now. Try again in a moment.")
    } finally {
      setBusy(false)
    }
  }
  const order = (t: string) =>
    net
      .order(id, t)
      .then((r) => setView((v) => (v ? { ...v, myOrder: r.myOrder } : v)))
      .catch((e) => notify(e instanceof ApiError ? e.message : 'Could not send the team talk.'))
  const leave = () =>
    net
      .leave(id)
      // In a lobby the next poll shows the finishing position; a series just ends.
      .then(() => (lobby ? undefined : onClose()))
      .catch((e) => notify(e instanceof ApiError ? e.message : 'Could not leave.'))

  const lastResult = view.results[view.results.length - 1]
  const patienceBar = (i: number) => (
    <div className={`chip patience t${i}`} title="The owner's patience. At zero the series is lost.">
      <span>{view.names[i]}{i === you ? ' (you)' : ''}</span>
      <div className="bar">
        <i style={{ width: `${(view.patience[i] / view.maxPatience) * 100}%` }} />
      </div>
      <b>{view.patience[i]}</b>
    </div>
  )

  return (
    <div className="app">
      <header className="top">
        <h1>Kickoff Tactics</h1>
        <div className="chip live-tag">{lobby ? (view.ranked ? '● RANKED LOBBY' : '● LOBBY') : '● LIVE SERIES'}</div>
        <div className="chip">
          Round <b>{view.round}</b>
        </div>
        {lobby && (
          <div className="chip">
            Still in{' '}
            <b>
              {view.standings?.filter((s) => s.alive).length ?? 0}/{view.standings?.length ?? 8}
            </b>
          </div>
        )}
        {view.phase === 'shop' && (
          <div className={`chip clock-chip ${left <= 10 ? 'hurry' : ''}`} title="When it reaches zero, whatever is on your pitch plays the match.">
            ⏱ Transfers <b className="timer">{left}s</b>
          </div>
        )}
        {lobby ? (
          <div className="chip patience" title="The owner's patience. At zero you are out of the lobby.">
            <span>Owner's patience</span>
            <div className="bar">
              <i style={{ width: `${((view.standings?.find((s) => s.you)?.patience ?? 0) / view.maxPatience) * 100}%` }} />
            </div>
            <b>{view.standings?.find((s) => s.you)?.patience ?? 0}</b>
          </div>
        ) : (
          <>
            {patienceBar(0)}
            <div className="chip">
              <b>
                {view.wins[0]} – {view.wins[1]}
              </b>
            </div>
            {patienceBar(1)}
          </>
        )}
        {view.status === 'live' && (
          <button className="restart" onClick={leave} title={lobby ? 'Give up: you finish last of those still in' : 'Give up: your friend wins the series'}>
            Forfeit
          </button>
        )}
      </header>

      <ClubBoard
        meta={meta}
        photos={photos}
        view={spectating && view.watched ? { ...view.my, opponent: { name: view.watched.name, units: view.watched.units }, oppSynergies: view.watched.synergies } : view.my}
        oppAugments={spectating ? view.watched?.augments : undefined}
        canAct={view.phase === 'shop' && (!lobby || !!view.alive)}
        busy={busy}
        onAction={act}
        oppTitle={
          spectating && view.watched
            ? `👁 Watching ${view.watched.name} · stadium lvl ${view.watched.level} · 💰 ${view.watched.budget} · patience ${view.watched.patience}`
            : lobby
              ? view.phase === 'shop'
                ? view.penRound
                  ? `Penalty round · opponents are drawn when the window closes`
                  : 'Opponents are drawn when the window closes'
                : view.penRound
                  ? `Penalty round: ${view.opponent} · winner +${meta.penaltyPrize ?? 2} 💰, no patience at stake`
                  : view.warmup
                    ? `Warm-up: ${view.opponent} · no patience at stake`
                    : `This round: ${view.opponent}${view.ghost ? ' (ghost squad)' : ''}`
              : view.results.length
                ? `${view.opponent}'s last line-up`
                : `${view.opponent} is building a squad`
        }
        primary={
          <div className={`window ${left <= 10 ? 'hurry' : ''}`}>
            {view.phase === 'shop' ? (
              <>
                Transfer window closes in <b className="timer">{left}s</b>
              </>
            ) : (
              'Transfer window closed'
            )}
          </div>
        }
        side={
          lobby ? (
            <div className="panel">
              <h3>Lobby</h3>
              {view.standings?.map((st) => (
                <button
                  key={st.seat}
                  className={`stand ${st.you ? 'you' : ''} ${st.alive ? '' : 'out'} ${st.facing ? 'facing' : ''} ${(view.watching ?? -1) === st.seat ? 'watched' : ''}`}
                  title={st.you ? 'Back to your own club and match' : `Watch ${st.name}: their board now, their match when it is played`}
                  onClick={() => setWatching(st.you ? -1 : st.seat)}
                >
                  <span>
                    {(view.watching ?? -1) === st.seat ? '👁 ' : ''}
                    {st.alive ? '' : `#${st.place} `}
                    {st.name}
                    {st.you ? ' (you)' : ''}
                    {st.facing ? ' ⚔' : ''}
                    {!!st.watchers && (
                      <small className="watchers" title={`${st.watchers} manager${st.watchers > 1 ? 's are' : ' is'} watching`}>
                        {' '}
                        👁{st.watchers}
                      </small>
                    )}
                  </span>
                  <b>{st.alive ? st.patience : 'out'}</b>
                </button>
              ))}
              <p className="muted">Click a manager to watch their board and their match. Click yourself to come back.</p>
              {!!view.watchingMe?.length && <p className="spectated">👁 Watching you: {view.watchingMe.join(', ')}</p>}
              {view.penRound ? (
                <p className="muted">
                  <b>Penalty round.</b> Your keeper and the best shooters on your pitch take on another manager: five kicks each, then sudden death. The winner gets +
                  {meta.penaltyPrize ?? 2} 💰; nobody loses patience.
                </p>
              ) : (
                me?.alive &&
                !!view.my.lossCost?.[1] && (
                  <p className="muted">
                    A defeat this round costs <b>{view.my.lossCost[0]}</b> patience by one goal, up to <b>{view.my.lossCost[1]}</b> by three or more. It grows every round.
                  </p>
                )
              )}
              <p className="muted">
                {view.warmupRounds ? `The first ${view.warmupRounds} rounds are penalty rounds between the managers, with nothing at stake. ` : ''}
                After that you play full matches: lose one and your owner loses patience. At zero you are out; the last one standing wins.
              </p>
            </div>
          ) : (
            <div className="panel">
              <h3>Series</h3>
              {view.results.length === 0 && <p className="muted">First match after this transfer window.</p>}
              {view.results.map((r) => (
                <p key={r.round} className={r.winner === you ? 'win' : 'loss'}>
                  {r.round}. {view.names[0]}{' '}
                  <b>
                    {r.score[0]}–{r.score[1]}
                  </b>{' '}
                  {view.names[1]}
                  {r.pens ? ' (pens)' : ''}
                </p>
              ))}
              <p className="muted">Whatever is on your pitch when the timer ends plays the match.</p>
            </div>
          )
        }
      />

      {inMatch && data.roster.length > 0 && (!out || stay) && (
        <div className="overlay">
          <div className="live-wrap">
            {lobby && (
              <div className="watch-bar">
                <span className="muted">
                  {spectating ? `👁 Watching ${view.names[0]} vs ${view.names[1]}` : view.penRound ? '🥅 Penalty round' : 'Your match'}
                  {!!view.spectators?.length && <b className="spectated"> · 👁 spectators: {view.spectators.join(', ')}</b>}
                </span>
                <select value={view.watching ?? -1} onChange={(e) => setWatching(Number(e.target.value))}>
                  {view.alive && <option value={-1}>My match</option>}
                  {view.standings
                    ?.filter((st) => st.alive && !st.you)
                    .map((st) => (
                      <option key={st.seat} value={st.seat}>
                        Watch {st.name}
                      </option>
                    ))}
                </select>
              </div>
            )}
            <MatchView key={match.current.key} data={data} meta={meta} photos={photos} startScore={[0, 0]} onDone={() => {}} clock={clock} total={view.penRound ? view.penTicks : 2 * meta.halfTicks + view.penTicks} />
            {marketOpen && (
              <MatchMarket meta={meta} photos={photos} view={view.my} canAct={!busy} busy={busy} onAction={act} offer={view.my.augmentOffer} />
            )}
            {view.phase === 'countdown' && (
              <div className="live-cover">
                <div>
                  <p className="muted">
                    {view.penRound ? 'Penalty round' : `Round ${view.round}`} · {view.names[0]} vs {view.names[1]}
                    {spectating ? ' · spectating' : <> · you are <b className={`t${you}`}>{view.names[you]}</b></>}
                  </p>
                  <div className="count">{Math.max(1, left)}</div>
                </div>
              </div>
            )}
            {view.phase === 'halftime' && (
              <div className="live-cover">
                <div className="dialog">
                  <h2>
                    Half-time · {view.halfScore?.[0]} : {view.halfScore?.[1]}
                  </h2>
                  {view.stats && <StatsTable data={{ ...data, stats: view.stats }} />}
                  {spectating ? (
                    <p className="muted">The managers are giving their team talks. Second half in {left}s.</p>
                  ) : (
                    <>
                      <h3>
                        Team talk · <span className="timer">{left}s</span>
                      </h3>
                      <div className="tactics">
                        {meta.tactics.map((t) => (
                          <button key={t.id} className={`tactic ${view.myOrder === t.id ? 'chosen' : ''}`} onClick={() => order(t.id)}>
                            <b>{t.name}</b>
                            <small>{t.desc}</small>
                            <small className="gk-note">🧤 {t.keeper}</small>
                          </button>
                        ))}
                      </div>
                      <p className="muted">You can change your mind until the timer ends. {view.opponent} cannot see your choice.</p>
                    </>
                  )}
                </div>
              </div>
            )}
            {view.phase === 'shootout' && view.penTicks === 0 && (
              <div className="live-cover">
                <div className="dialog">
                  <h2>
                    Full time · {view.score?.[0]} : {view.score?.[1]}
                  </h2>
                  <p className="muted">Other matches are settled on penalties. Results in {left}s.</p>
                </div>
              </div>
            )}
            {view.phase === 'result' && view.score && (
              <div className="live-cover">
                <div className="dialog">
                  <h2 className={spectating ? '' : view.matchWinner === you ? 'win' : 'loss'}>
                    {spectating
                      ? `${view.names[view.matchWinner ?? 0]} won`
                      : view.penRound
                        ? view.matchWinner === you
                          ? `You won the penalty round · +${meta.penaltyPrize ?? 2} 💰`
                          : 'You lost the penalty round'
                        : view.matchWinner === you
                          ? 'You won the round'
                          : 'You lost the round'}
                    {!view.penRound && ` · ${view.score[0]} : ${view.score[1]}`}
                  </h2>
                  <Shootout data={{ ...data, pens: view.pens }} />
                  {view.stats && <StatsTable data={{ ...data, stats: view.stats }} />}
                  <p>
                    Owner's patience: {view.names[0]} <b>{view.patience[0]}</b> · {view.names[1]} <b>{view.patience[1]}</b>
                  </p>
                  <p className="muted">{lobby ? `Next round in ${left}s` : view.winner === undefined ? `Next transfer window in ${left}s` : `Series decided. Final screen in ${left}s`}</p>
                </div>
              </div>
            )}
          </div>
        </div>
      )}

      {lobby && !!view.place && !(stay && view.status === 'live') && (
        <div className="overlay">
          <div className="dialog">
            <h2 className={view.place <= Math.ceil((view.standings?.length ?? 8) / 2) ? 'win' : 'loss'}>{view.place === 1 ? '🏆 You won the lobby' : `You finished #${view.place} of ${view.standings?.length ?? 8}`}</h2>
            {view.ranked ? (
              <p className="lp">
                <b className={(view.lpChange ?? 0) >= 0 ? 'win' : 'loss'}>
                  {(view.lpChange ?? 0) >= 0 ? '+' : ''}
                  {view.lpChange} points
                </b>{' '}
                · rating {view.rating} · {view.tier}
              </p>
            ) : (
              <p className="muted">No points at stake in this lobby: your rating did not change.</p>
            )}
            {view.status === 'done' && view.place !== 1 && view.standings?.find((st) => st.place === 1) && (
              <p className="winner-line">
                🏆 Lobby winner: <b>{view.standings.find((st) => st.place === 1)!.name}</b>
              </p>
            )}
            {view.status === 'live' && (
              <p className="muted">
                Still playing: {view.standings?.filter((st) => st.alive).map((st) => st.name).join(', ')}. Keep watching to see who wins; this
                list updates when the lobby ends.
              </p>
            )}
            <div className="final-stand">
              {view.standings?.map((st) => (
                <div key={st.name} className={`stand ${st.you ? 'you' : ''}`}>
                  <span>
                    {st.alive ? 'still in' : `#${st.place}`} · {st.name}
                  </span>
                  <b>{st.bot ? 'bot' : ''}</b>
                </div>
              ))}
            </div>
            <div className="row center-row">
              {view.status === 'live' && (
                <button className="big" onClick={() => setStay(true)} title="Stay and watch the managers still in">
                  👁 Keep watching
                </button>
              )}
              <button className="primary big" onClick={onClose}>
                Back to home
              </button>
            </div>
          </div>
        </div>
      )}
      {out && stay && (watching < 0 || !view.standings?.find((s) => s.seat === watching)?.alive) && (
        <div className="toast">You are out. Pick a manager in the lobby list to watch their match.</div>
      )}

      {!lobby && view.status === 'done' && (
        <div className="overlay">
          <div className="dialog">
            <h2 className={view.winner === you ? 'win' : 'loss'}>{view.winner === you ? '🏆 You won the series' : `${view.opponent} won the series`}</h2>
            <p>
              Matches: {view.names[0]} <b>{view.wins[0]}</b> – <b>{view.wins[1]}</b> {view.names[1]}
              {lastResult && ` · last match ${lastResult.score[0]}–${lastResult.score[1]}`}
            </p>
            <p>
              Owner's patience left: {view.names[0]} <b>{view.patience[0]}</b> · {view.names[1]} <b>{view.patience[1]}</b>
            </p>
            <button className="primary big" onClick={onClose}>
              Back to home
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
