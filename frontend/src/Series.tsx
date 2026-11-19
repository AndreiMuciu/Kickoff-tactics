import { useEffect, useRef, useState } from 'react'
import { api, ApiError } from './api'
import type { Action, MatchData, Meta, SeriesView } from './api'
import ClubBoard, { Shootout, StatsTable } from './ClubBoard'
import { watch } from './live'
import MatchView from './MatchView'
import type { Photos } from './photos'

interface Props {
  id: string
  kind: 'series' | 'lobby' // a duel with a friend, or a ranked lobby of eight
  meta: Meta
  photos: Photos
  onClose: () => void
  notify: (msg: string) => void
}

const emptyStats = () => ({ shots: 0, xg: 0, passes: 0, tackles: 0, poss: 0 })
const newMatch = (): MatchData => ({ half: 1, roster: [], frames: [], events: [], score: [0, 0], stats: [emptyStats(), emptyStats()], names: ['', ''] })

// Series is a live season against a friend. Everything runs on the server's
// clock: the transfer window, the match and the break. This screen is sent the
// state once a second, shows the phase the server reports and plays match frames at the
// moment the server says they belong to, so both managers stay in step.
export default function Series({ id, kind, meta, photos, onClose, notify }: Props) {
  const lobby = kind === 'lobby'
  // Both kinds of live game speak the same protocol on different routes.
  const net = lobby
    ? { action: api.lobbyAction, order: api.lobbyOrder, leave: api.lobbyLeave }
    : { action: api.seriesAction, order: api.seriesOrder, leave: api.seriesLeave }
  const [view, setView] = useState<SeriesView | null>(null)
  const [busy, setBusy] = useState(false)
  const [, redraw] = useState(0)
  // One replay object per round; its arrays grow in place while the match is on.
  const match = useRef<{ round: number; data: MatchData }>({ round: 0, data: newMatch() })
  const sync = useRef({ offset: 0, ready: false, kickoff: 0, half: 30000, brk: 20000, pen: 0 })
  // Time of my last transfer move: a poll sent before it carries an older club
  // and must not overwrite the fresher answer (a sold player would reappear).
  const lastMove = useRef(0)

  useEffect(() => {
    const path = `${lobby ? '/api/lobby' : '/api/series'}?id=${encodeURIComponent(id)}`
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
        if (v.round !== match.current.round) match.current = { round: v.round, data: newMatch() }
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
  }, [id])

  const serverNow = () => Date.now() + sync.current.offset
  // Maps server time to a frame position: first half, a pause, then the second half.
  const clock = () => {
    const { kickoff, half, brk } = sync.current
    const e = serverNow() - kickoff
    const per = meta.halfTicks
    if (e <= 0) return 0
    if (e < half) return (e / half) * per
    if (e < half + brk) return per - 0.01
    if (e < 2 * half + brk) return per + ((e - half - brk) / half) * per
    // After the final whistle a shootout, if there is one, runs at the same pace.
    const pen = sync.current.pen
    return Math.min(2 * per + Math.max(0, pen) - 0.01, 2 * per + ((e - 2 * half - brk) / half) * per)
  }

  if (!view || !view.my)
    return (
      <div className="center">
        <h1>Kickoff Tactics</h1>
        <p className="muted">{lobby ? 'Joining the lobby…' : 'Joining the live series…'}</p>
      </div>
    )

  const you = view.role
  const left = Math.max(0, Math.ceil((view.phaseEndsAt - serverNow()) / 1000))
  const inMatch = view.phase === 'countdown' || view.phase === 'h1' || view.phase === 'halftime' || view.phase === 'h2' || view.phase === 'shootout' || view.phase === 'result'
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
        <div className="chip budget">
          💰 <b>{view.my.budget}</b>
        </div>
        {view.status === 'live' && (
          <button className="restart" onClick={leave} title={lobby ? 'Give up: you finish last of those still in' : 'Give up: your friend wins the series'}>
            Forfeit
          </button>
        )}
      </header>

      <ClubBoard
        meta={meta}
        photos={photos}
        view={view.my}
        canAct={view.phase === 'shop'}
        busy={busy}
        onAction={act}
        oppTitle={lobby ? (view.opponent ? view.warmup ? `Warm-up: ${view.opponent} · no patience at stake` : `This round: ${view.opponent}${view.ghost ? ' (ghost squad)' : ''}` : 'Opponents are drawn when the window closes') : view.results.length ? `${view.opponent}'s last line-up` : `${view.opponent} is building a squad`}
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
                <div key={st.name} className={`stand ${st.you ? 'you' : ''} ${st.alive ? '' : 'out'} ${st.facing ? 'facing' : ''}`}>
                  <span>
                    {st.alive ? '' : `#${st.place} `}
                    {st.name}
                    {st.you ? ' (you)' : ''}
                    {st.facing ? ' ⚔' : ''}
                  </span>
                  <b>{st.alive ? st.patience : 'out'}</b>
                </div>
              ))}
              <p className="muted">
                {view.warmupRounds
                  ? `The first ${view.warmupRounds} rounds are warm-ups against a small club: you earn budget and experience, a defeat costs nothing. `
                  : ''}
                After that you meet the other managers: lose a match and your owner loses patience. At zero you are out; the last one standing wins.
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

      {inMatch && data.roster.length > 0 && (
        <div className="overlay">
          <div className="live-wrap">
            <MatchView key={view.round} data={data} meta={meta} photos={photos} startScore={[0, 0]} onDone={() => {}} clock={clock} total={2 * meta.halfTicks + view.penTicks} />
            {view.phase === 'countdown' && (
              <div className="live-cover">
                <div>
                  <p className="muted">
                    Round {view.round} · {view.names[0]} vs {view.names[1]} · you are <b className={`t${you}`}>{view.names[you]}</b>
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
                  <h2 className={view.matchWinner === you ? 'win' : 'loss'}>
                    {view.matchWinner === you ? 'You won the round' : 'You lost the round'} · {view.score[0]} : {view.score[1]}
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

      {lobby && !!view.place && (
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
            <button className="primary big" onClick={onClose}>
              Back to home
            </button>
          </div>
        </div>
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
