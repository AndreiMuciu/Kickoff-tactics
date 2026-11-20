import { useCallback, useEffect, useState } from 'react'
import { api, ApiError } from './api'
import type { Action, HalfData, Meta, RunView } from './api'
import ClubBoard, { Shootout, StatsTable } from './ClubBoard'
import { MatchMarket } from './Market'
import MatchView from './MatchView'
import type { Photos } from './photos'

const RESULT_TEXT: Record<string, { title: string; text: string }> = {
  champion: { title: '🏆 Champions!', text: 'The owner is building you a statue outside the stadium.' },
  europe: { title: '🎟 European football', text: 'A solid season. The owner extends your contract.' },
  survived: { title: '😮‍💨 You lasted the season', text: 'Not pretty, but you were still in the dugout at the end.' },
  sacked: { title: '📦 Sacked', text: "The owner's patience ran out. Time to clear your locker." },
}

type MatchStage =
  | { stage: 'h1'; h1: HalfData }
  | { stage: 'halftime'; h1: HalfData }
  | { stage: 'h2'; h1: HalfData; h2: HalfData }
  | { stage: 'result'; h1: HalfData; h2: HalfData }

interface Props {
  meta: Meta
  photos: Photos
  fresh: boolean
  onHome: () => void
}

// The first load may create a season (POST). React's development mode mounts
// a screen twice, and two creations racing each other left the page showing
// one season while the server kept the other: the market on screen was not
// the one being bought from. Both mounts share this single request instead.
let loading: Promise<RunView> | null = null
function loadRun(fresh: boolean): Promise<RunView> {
  loading ??= (async () => {
    if (fresh) return api.newRun()
    try {
      return await api.run()
    } catch (e) {
      if (!(e instanceof ApiError && e.status === 404)) throw e
      return api.newRun()
    }
  })().finally(() => {
    loading = null
  })
  return loading
}

export default function Game({ meta, photos, fresh, onHome }: Props) {
  const [run, setRun] = useState<RunView | null>(null)
  const [busy, setBusy] = useState(false)
  const [toast, setToast] = useState<string | null>(null)
  const [match, setMatch] = useState<MatchStage | null>(null)

  const fail = useCallback((e: unknown) => {
    setToast(e instanceof ApiError ? e.message : "We can't reach the game right now. Try again in a moment.")
    window.setTimeout(() => setToast(null), 3200)
  }, [])

  useEffect(() => {
    let alive = true
    ;(async () => {
      try {
        const r = await loadRun(fresh)
        if (!alive) return
        setRun(r)
        if (r.phase === 'halftime') {
          const h1 = await api.match()
          if (alive) setMatch({ stage: 'halftime', h1 })
        }
      } catch (e) {
        if (alive) fail(e)
      }
    })()
    return () => {
      alive = false
    }
    // `fresh` is fixed for the lifetime of this screen.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const guard = async <T,>(f: () => Promise<T>): Promise<T | undefined> => {
    if (busy) return
    setBusy(true)
    try {
      return await f()
    } catch (e) {
      fail(e)
    } finally {
      setBusy(false)
    }
  }
  const act = (a: Action) =>
    guard(async () => {
      try {
        setRun(await api.action(a))
      } catch (e) {
        // A refused action may mean this screen is out of date: show what the server has.
        api.run().then(setRun).catch(() => {})
        throw e
      }
    })
  const newRun = () =>
    guard(async () => {
      setMatch(null)
      setRun(await api.newRun())
    })
  const kickoff = () => guard(async () => setMatch({ stage: 'h1', h1: await api.startMatch() }))
  const second = (tactic: string) =>
    guard(async () => {
      if (!match) return
      const h2 = await api.secondHalf(tactic)
      setMatch({ stage: 'h2', h1: match.h1, h2 })
    })

  if (!run)
    return (
      <div className="center">
        <h1>Kickoff Tactics</h1>
        <p className="muted">{toast ?? 'Loading your club…'}</p>
        <button onClick={onHome}>← Home</button>
      </div>
    )

  const shopPhase = run.phase === 'shop' && !match
  const last = run.history[run.history.length - 1]

  return (
    <div className="app">
      <header className="top">
        <button className="restart" onClick={onHome}>
          ← Home
        </button>
        <h1>Kickoff Tactics</h1>
        <div className="chip">
          Fixture{' '}
          <b>
            {Math.min(run.round, run.maxRounds)}/{run.maxRounds}
          </b>
        </div>
        <div className="chip patience" title="The owner's patience. At zero you are sacked.">
          <span>Owner's patience</span>
          <div className="bar">
            <i style={{ width: `${(run.patience / run.maxPatience) * 100}%` }} />
          </div>
          <b>{run.patience}</b>
        </div>
        <div className="chip">
          Record{' '}
          <b>
            {run.wins}W {run.losses}L
          </b>
        </div>
        {Math.abs(run.streak) >= 2 && (
          <div className="chip">
            {run.streak > 0 ? '🔥' : '🧊'} streak {Math.abs(run.streak)}
          </div>
        )}
        {shopPhase && (
          <button className="restart" disabled={busy} onClick={newRun} title="Abandon this season and start another">
            ↺ New season
          </button>
        )}
      </header>

      <ClubBoard
        meta={meta}
        photos={photos}
        view={run}
        canAct={shopPhase}
        busy={busy}
        onAction={act}
        oppTitle={`Scouting: ${run.opponent.name}`}
        primary={
          <button className="primary" disabled={!shopPhase || busy} onClick={kickoff}>
            Kick off ▶
          </button>
        }
        side={
          <div className="panel">
            <h3>Season</h3>
            {run.history.length === 0 && <p className="muted">Your first match awaits.</p>}
            {run.history.map((h) => (
              <p key={h.round} className={h.won ? 'win' : 'loss'}>
                {h.round}. {h.opponent}{' '}
                <b>
                  {h.gf}–{h.ga}
                </b>
                {h.pens ? ' (pens)' : ''}
              </p>
            ))}
          </div>
        }
      />

      {match && (
        <div className="overlay">
          {match.stage === 'h1' && <MatchView key="h1" data={match.h1} meta={meta} photos={photos} startScore={[0, 0]} onDone={() => setMatch({ stage: 'halftime', h1: match.h1 })} />}
          {/* The market stays open in the first half and at the break. After the
              break the season has already moved on to the next round, whose
              market and budget would give the result away. */}
          {(match.stage === 'h1' || match.stage === 'halftime') && (
            <div className="solo-market">
              <MatchMarket
                meta={meta}
                photos={photos}
                view={run}
                canAct={!busy}
                busy={busy}
                onAction={act}
                note="Signings made now play from the next match. The market closes when the second half starts and reopens after the final whistle."
              />
            </div>
          )}
          {match.stage === 'halftime' && (
            <div className="dialog">
              <h2>
                Half-time · {match.h1.score[0]} : {match.h1.score[1]}
              </h2>
              <StatsTable data={match.h1} />
              <h3>What do you tell them in the dressing room?</h3>
              <div className="tactics">
                {meta.tactics.map((t) => (
                  <button key={t.id} className="tactic" disabled={busy} onClick={() => second(t.id)}>
                    <b>{t.name}</b>
                    <small>{t.desc}</small>
                    <small className="gk-note">🧤 {t.keeper}</small>
                  </button>
                ))}
              </div>
            </div>
          )}
          {match.stage === 'h2' && (
            <MatchView key="h2" data={match.h2} meta={meta} photos={photos} startScore={match.h1.score} onDone={() => setMatch({ stage: 'result', h1: match.h1, h2: match.h2 })} />
          )}
          {match.stage === 'result' && match.h2.result && (
            <div className="dialog">
              <h2 className={match.h2.result.won ? 'win' : 'loss'}>
                {match.h2.result.won ? 'Victory!' : 'Defeat'} · {match.h2.score[0]} : {match.h2.score[1]}
              </h2>
              <Shootout data={match.h2} />
              <StatsTable data={match.h2} />
              <p>
                {match.h2.result.won ? 'The owner is smiling.' : `The owner's patience drops by ${match.h2.result.damage}.`}
                {match.h2.result.income > 0 && ` Budget received: +${match.h2.result.income} 💰.`}
              </p>
              <button
                className="primary"
                onClick={() => {
                  setRun(match.h2.run)
                  setMatch(null)
                  // The result carries the club as the match ended; read it again
                  // in case anything changes server-side meanwhile.
                  api.run().then(setRun).catch(() => {})
                }}
              >
                Continue
              </button>
            </div>
          )}
        </div>
      )}

      {run.phase === 'ended' && !match && (
        <div className="overlay">
          <div className="dialog">
            <h2>{RESULT_TEXT[run.result]?.title}</h2>
            <p>{RESULT_TEXT[run.result]?.text}</p>
            <p>
              Record: {run.wins} wins, {run.losses} defeats.
              {last && ` Last match: ${last.gf}–${last.ga} against ${last.opponent}.`}
            </p>
            <div className="row center-row">
              <button onClick={onHome}>← Home</button>
              <button className="primary big" disabled={busy} onClick={newRun}>
                New season
              </button>
            </div>
          </div>
        </div>
      )}
      {toast && <div className="toast">{toast}</div>}
    </div>
  )
}
