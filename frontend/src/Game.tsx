import { useCallback, useEffect, useState } from 'react'
import { api, ApiError } from './api'
import type { Action, HalfData, Meta, RunView } from './api'
import ClubBoard, { Shootout, StatsTable } from './ClubBoard'
import { MatchMarket } from './Market'
import MatchView from './MatchView'
import type { Photos } from './photos'
import { EPILOGUE, INTRO, PLEDGES, RIVALS, reaction } from './story'
import { LeagueTable, Results, ord } from './League'

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
  // The opening chapter is shown once per season, before the first fixture.
  const [introSeen, setIntroSeen] = useState(false)

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
      setIntroSeen(false)
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
          Matchday{' '}
          <b>
            {Math.min(run.round, run.maxRounds)}/{run.maxRounds}
          </b>
        </div>
        {run.league && (
          <div className="chip" title="League points and position">
            <b>{run.league.points}</b> pts · <b>{run.league.played ? ord(run.league.position) : '–'}</b>
          </div>
        )}
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
            {run.wins}W {run.draws}D {run.losses}L
          </b>
        </div>
        {run.pledge && PLEDGES[run.pledge] && (
          <div className="chip pledge" title="Judged by the next match">
            {PLEDGES[run.pledge]}
          </div>
        )}
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
            {run.phase === 'shop' && (
              <div className="rival">
                <h3>
                  Matchday {run.round}: {run.opponent.name}
                </h3>
                {RIVALS[run.opponent.name] && <p className="muted">{RIVALS[run.opponent.name]}</p>}
              </div>
            )}
            {run.league && (
              <>
                <h3>League table</h3>
                <LeagueTable league={run.league} compact />
              </>
            )}
            <h3>Your results</h3>
            {run.history.length === 0 && <p className="muted">Your first match awaits.</p>}
            {run.history.map((h) => (
              <p key={h.round} className={h.won ? 'win' : h.draw ? 'draw' : 'loss'}>
                {h.round}. {h.opponent}{' '}
                <b>
                  {h.gf}–{h.ga}
                </b>
                {h.pens ? ' (pens)' : ''}
                {h.note && <small className="story-note"> · {h.note}</small>}
              </p>
            ))}
          </div>
        }
      />

      {match && (
        <div className="overlay">
          {match.stage === 'h1' && <MatchView key="h1" data={match.h1} meta={meta} photos={photos} startScore={[0, 0]} streaks={[run.streak, 0]} onDone={() => setMatch({ stage: 'halftime', h1: match.h1 })} />}
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
            <MatchView key="h2" data={match.h2} meta={meta} photos={photos} startScore={match.h1.score} streaks={[run.streak, 0]} onDone={() => setMatch({ stage: 'result', h1: match.h1, h2: match.h2 })} />
          )}
          {match.stage === 'result' && match.h2.result && (
            <div className="dialog">
              <h2 className={match.h2.result.won ? 'win' : match.h2.result.draw ? 'draw' : 'loss'}>
                {match.h2.result.won ? 'Victory!' : match.h2.result.draw ? 'Draw' : 'Defeat'} · {match.h2.score[0]} : {match.h2.score[1]}
              </h2>
              <Shootout data={match.h2} />
              <StatsTable data={match.h2} />
              <p className="owner-says">{reaction(match.h2.result.won, match.h2.result.gf, match.h2.result.ga, match.h2.result.round)}</p>
              <p>
                {match.h2.result.won ? '+3 points. The owner is smiling.' : match.h2.result.draw ? '+1 point. No harm to the owner’s patience.' : `No points, and the owner's patience drops by ${match.h2.result.damage}.`}
                {match.h2.result.income > 0 && ` Budget received: +${match.h2.result.income} 💰.`}
              </p>
              {match.h2.result.note && <p className="story-note">📰 {match.h2.result.note}</p>}
              {match.h2.run.league && match.h2.run.league.last.length > 0 && (
                <div className="matchday">
                  <div>
                    <h3>Matchday {match.h2.run.league.played}: other results</h3>
                    <Results results={match.h2.run.league.last} />
                  </div>
                  <div>
                    <h3>Table</h3>
                    <LeagueTable league={match.h2.run.league} compact />
                  </div>
                </div>
              )}
              {!!match.h2.unlocked?.length && (
                <p className="ach-unlocked">🏅 Achievement unlocked: {match.h2.unlocked.map((a) => `${a.icon} ${a.name} (${a.tier})`).join(', ')}</p>
              )}
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
            <h2>{EPILOGUE[run.result]?.title}</h2>
            <p>{EPILOGUE[run.result]?.text}</p>
            <p>
              {run.league && run.result !== 'sacked' && (
                <>
                  Final position: <b>{ord(run.league.position)}</b> with <b>{run.league.points}</b> points.{' '}
                </>
              )}
              Record: {run.wins} wins, {run.draws} draws, {run.losses} defeats.
              {last && ` Last match: ${last.gf}–${last.ga} against ${last.opponent}.`}
            </p>
            {run.league && <LeagueTable league={run.league} />}
            <div className="row center-row">
              <button onClick={onHome}>← Home</button>
              <button className="primary big" disabled={busy} onClick={newRun}>
                New season
              </button>
            </div>
          </div>
        </div>
      )}
      {shopPhase && run.round === 1 && run.history.length === 0 && !introSeen && (
        <div className="overlay story-overlay">
          <div className="dialog story">
            <small className="story-kicker">Chapter 1</small>
            <h2>{INTRO.title}</h2>
            {INTRO.text.map((t, i) => (
              <p key={i}>{t}</p>
            ))}
            <button className="primary big" onClick={() => setIntroSeen(true)}>
              Take the job
            </button>
          </div>
        </div>
      )}
      {shopPhase && run.event && (
        <div className="overlay story-overlay">
          <div className="dialog story">
            <small className="story-kicker">Before fixture {run.round}</small>
            <h2>{run.event.title}</h2>
            <p>{run.event.text}</p>
            <div className="story-choices">
              {run.event.choices.map((c, i) => {
                const short = (c.cost ?? 0) > run.budget
                return (
                  <button key={i} className="tactic" disabled={busy || short} title={short ? 'Not enough budget' : undefined} onClick={() => act({ type: 'story', slot: i })}>
                    <b>{c.label}</b>
                    <small>{c.effect}</small>
                  </button>
                )
              })}
            </div>
          </div>
        </div>
      )}
      {toast && <div className="toast">{toast}</div>}
    </div>
  )
}
