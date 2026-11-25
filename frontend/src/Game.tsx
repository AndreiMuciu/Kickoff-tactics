import { Fragment, useCallback, useEffect, useState } from 'react'
import { api, ApiError } from './api'
import type { Action, CupView, HalfData, HistoryEntry, Me, Meta, RunView } from './api'
import { challengeLink, friendLink, shareCard } from './share'
import type { Card } from './share'
import ClubBoard, { Shootout, StatsTable } from './ClubBoard'
import { MatchMarket } from './Market'
import MatchView from './MatchView'
import type { Photos } from './photos'
import { CUPS, EPILOGUE, INTRO, PLEDGES, RIVALS, STAGES, cupEnding, cupStage, reaction, stars } from './story'
import { CupOverview, LeagueTable, Results, Ties, ord } from './League'

type MatchStage =
  | { stage: 'h1'; h1: HalfData }
  | { stage: 'halftime'; h1: HalfData }
  | { stage: 'h2'; h1: HalfData; h2: HalfData }
  | { stage: 'result'; h1: HalfData; h2: HalfData }

interface Props {
  meta: Meta
  photos: Photos
  fresh: boolean
  daily: boolean // today's daily challenge instead of the season
  me: Me
  onHome: () => void
}

// The first load may create a season (POST). React's development mode mounts
// a screen twice, and two creations racing each other left the page showing
// one season while the server kept the other: the market on screen was not
// the one being bought from. Both mounts share this single request instead.
let loading: Promise<RunView> | null = null
function loadRun(fresh: boolean, daily: boolean): Promise<RunView> {
  loading ??= (async () => {
    if (fresh) return api.newRun(daily)
    try {
      return await api.run(daily)
    } catch (e) {
      if (!(e instanceof ApiError && e.status === 404)) throw e
      return api.newRun(daily)
    }
  })().finally(() => {
    loading = null
  })
  return loading
}

// What a result after the league means, for match m (1..6).
function europeLine(r: HistoryEntry, m: number, cup?: CupView): string {
  if (cup?.playoff) {
    if (!r.won && !r.draw) return 'A defeat. The owner has seen enough.'
    const pts = r.won ? '+3 points.' : '+1 point.'
    if (m < 3) return `${pts} Still alive.`
    return `${pts} ${cup.out === 'promoted' ? 'First place: the club goes straight back up!' : 'Not first: the club stays down.'}`
  }
  if (m > 3) {
    if (!r.won) return 'Out of Europe. The adventure ends here.'
    return m === 6 ? '🏆 The trophy is yours!' : m === 5 ? 'Through to the final!' : 'Through to the semi-finals!'
  }
  const pts = r.won ? '+3 points in the group.' : r.draw ? '+1 point in the group.' : 'No points in the group.'
  if (m < 3) return pts
  return `${pts} ${cup?.out === 'group' ? 'Not enough: the group stage is as far as you go.' : 'You are through to the quarter-finals!'}`
}

const cupStageKey = (m: number) => (m <= 4 ? 'quarter' : m === 5 ? 'semi' : 'final')

const DAILY_RULES = "Today's challenge: every manager gets this same club, the same market and the same rivals. No Europe and no play-off: after nine matchdays, the most points wins the day."

const PLACE: Record<string, string> = { champion: 'Champions', sacked: 'Sacked' }

// card is what the share picture shows of a finished season.
function card(run: RunView, meta: Meta): Card {
  const lg = run.league
  const pos = lg ? lg.position : 0
  const cupLine = run.cup?.out ? cupEnding(run.cup.id, run.cup.out).title : undefined
  const best = [...run.units]
    .map((u) => ({ u, p: meta.players.find((p) => p.id === u.def) }))
    .filter((x) => x.p)
    .sort((a, b) => b.u.stars - a.u.stars || b.p!.cost - a.p!.cost)
    .slice(0, 3)
    .map((x) => `${x.p!.name}${x.u.stars > 1 ? ' ' + '★'.repeat(x.u.stars) : ''}`)
  const day = run.daily ? new Date(`${run.daily}T12:00:00Z`).toLocaleDateString('en-GB', { day: 'numeric', month: 'short' }) : ''
  return {
    kicker: run.daily ? `Daily challenge · ${day}` : 'Solo season',
    title: run.result === 'sacked' && !run.cup ? 'Sacked' : PLACE[run.result] ?? (pos ? `${ord(pos)} place` : 'Season over'),
    line: cupLine,
    stats: [
      ['points', String(lg?.points ?? 0)],
      ['W-D-L', `${run.wins}-${run.draws ?? 0}-${run.losses}`],
      ...(pos ? ([['position', ord(pos)]] as [string, string][]) : []),
    ],
    players: best,
    footer: run.daily ? 'Beat my score at' : 'Build your club at',
  }
}

export default function Game({ meta, photos, fresh, daily, me, onHome }: Props) {
  const [run, setRun] = useState<RunView | null>(null)
  const [busy, setBusy] = useState(false)
  const [toast, setToast] = useState<string | null>(null)
  const [match, setMatch] = useState<MatchStage | null>(null)
  // The opening chapter is shown once per season, before the first fixture.
  const [introSeen, setIntroSeen] = useState(false)
  const [cupIntroSeen, setCupIntroSeen] = useState(false)

  const fail = useCallback((e: unknown) => {
    setToast(e instanceof ApiError ? e.message : "We can't reach the game right now. Try again in a moment.")
    window.setTimeout(() => setToast(null), 3200)
  }, [])

  useEffect(() => {
    let alive = true
    ;(async () => {
      try {
        const r = await loadRun(fresh, daily)
        if (!alive) return
        setRun(r)
        if (r.phase === 'halftime') {
          const h1 = await api.match(daily)
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
        setRun(await api.action(a, daily))
      } catch (e) {
        // A refused action may mean this screen is out of date: show what the server has.
        api.run(daily).then(setRun).catch(() => {})
        throw e
      }
    })
  const newRun = () =>
    guard(async () => {
      setMatch(null)
      setIntroSeen(false)
      setCupIntroSeen(false)
      setRun(await api.newRun(daily))
    })
  const kickoff = () => guard(async () => setMatch({ stage: 'h1', h1: await api.startMatch(daily) }))
  const second = (tactic: string) =>
    guard(async () => {
      if (!match) return
      const h2 = await api.secondHalf(tactic, daily)
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
  const share = () =>
    guard(async () => {
      const pts = run.league?.points ?? 0
      const text = run.daily
        ? `I made ${pts} pts in today's Kickoff Tactics challenge. Can you beat me?`
        : `My Kickoff Tactics season: ${card(run, meta).title}, ${pts} pts.`
      const link = run.daily ? challengeLink(me.name) : me.guest ? location.origin : friendLink(me.name)
      const r = await shareCard(card(run, meta), text, link)
      if (r === 'saved') {
        setToast('Picture saved and text with link copied: paste it anywhere.')
        window.setTimeout(() => setToast(null), 3200)
      }
    })
  const last = run.history[run.history.length - 1]

  return (
    <div className="app">
      <header className="top game-top">
        <button className="restart" onClick={onHome}>
          ← Home
        </button>
        <h1>Kickoff Tactics</h1>
        {run.cup ? (
          <div className={`chip cup cup-${run.cup.id}`} title={run.cup.playoff ? 'The play-off after relegation' : 'Your European run'}>
            <span className="cup-stars">{stars(run.cup.stars)}</span> {run.cup.name} · <b>{cupStage(run.cup.match, run.cup.playoff)}</b>
          </div>
        ) : (
          <div className="chip">
            Matchday{' '}
            <b>
              {Math.min(run.round, run.maxRounds)}/{run.maxRounds}
            </b>
          </div>
        )}
        {run.daily && (
          <div className="chip daily-chip" title="Today's daily challenge: the same start for everyone">
            📅 Daily
          </div>
        )}
        {run.league && (
          <div className="chip" title="League points and position">
            <b>{run.league.points}</b> pts · <b>{run.league.played ? ord(run.league.position) : '–'}</b>
          </div>
        )}
        {(!run.cup || run.cup.playoff) && (
          <div className="chip patience" title="The owner's patience. At zero you are sacked.">
            <span>Owner's patience</span>
            <div className="bar">
              <i style={{ width: `${(run.patience / run.maxPatience) * 100}%` }} />
            </div>
            <b>{run.patience}</b>
          </div>
        )}
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
                  {run.cup ? cupStage(run.cup.match, run.cup.playoff) : `Matchday ${run.round}`}: {run.opponent.name}
                </h3>
                {RIVALS[run.opponent.name] && <p className="muted">{RIVALS[run.opponent.name]}</p>}
              </div>
            )}
            {run.cup && (
              <>
                <h3>
                  {stars(run.cup.stars)} {run.cup.name}
                  {run.cup.playoff ? '' : `: group ${run.cup.groups[run.cup.mine]?.letter}`}
                </h3>
                <LeagueTable league={run.cup.group} compact group />
                {!run.cup.playoff && (
                  <details className="cup-all">
                    <summary>All groups and the knockout bracket</summary>
                    <CupOverview cup={run.cup} />
                  </details>
                )}
              </>
            )}
            {run.league && (
              <>
                <h3>{run.cup ? 'Final league table' : 'League table'}</h3>
                <LeagueTable league={run.league} compact />
              </>
            )}
            <h3>Your results</h3>
            {run.history.length === 0 && <p className="muted">Your first match awaits.</p>}
            {run.history.map((h) => (
              <Fragment key={h.round}>
                {h.round === run.maxRounds + 1 && <h3>Europe</h3>}
                <p className={h.won ? 'win' : h.draw ? 'draw' : 'loss'}>
                  {h.round > run.maxRounds ? `E${h.round - run.maxRounds}` : h.round}. {h.opponent}{' '}
                  <b>
                    {h.gf}–{h.ga}
                  </b>
                  {h.pens ? ' (pens)' : ''}
                  {h.note && <small className="story-note"> · {h.note}</small>}
                </p>
              </Fragment>
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
              {match.h2.result.round > run.maxRounds ? (
                <p>
                  {europeLine(match.h2.result, match.h2.result.round - run.maxRounds, match.h2.run.cup)}
                  {match.h2.result.income > 0 && ` Budget received: +${match.h2.result.income} 💰.`}
                </p>
              ) : (
                <>
                  <p className="owner-says">{reaction(match.h2.result.won, match.h2.result.gf, match.h2.result.ga, match.h2.result.round)}</p>
                  <p>
                    {match.h2.result.won ? '+3 points. The owner is smiling.' : match.h2.result.draw ? '+1 point. No harm to the owner’s patience.' : `No points, and the owner's patience drops by ${match.h2.result.damage}.`}
                    {match.h2.result.income > 0 && ` Budget received: +${match.h2.result.income} 💰.`}
                  </p>
                </>
              )}
              {match.h2.result.note && <p className="story-note">📰 {match.h2.result.note}</p>}
              {match.h2.result.round > run.maxRounds && match.h2.result.round - run.maxRounds <= 3 && match.h2.run.cup && (
                <div className="matchday">
                  <div>
                    <h3>{match.h2.run.cup.playoff ? 'The other match' : `Matchday ${match.h2.result.round - run.maxRounds} around Europe`}</h3>
                    {match.h2.run.cup.groups.map((g) =>
                      g.last.some((r) => r.home !== 'Your Club' && r.away !== 'Your Club') ? (
                        <div key={g.letter}>
                          {!match.h2.run.cup?.playoff && <h4>Group {g.letter}</h4>}
                          <Results results={g.last} />
                        </div>
                      ) : null,
                    )}
                  </div>
                  <div>
                    <h3>Your group</h3>
                    <LeagueTable league={match.h2.run.cup.group} compact group />
                  </div>
                </div>
              )}
              {match.h2.result.round - run.maxRounds > 3 && match.h2.run.cup && !match.h2.run.cup.playoff && (
                <div className="matchday">
                  <div>
                    <h3>{STAGES[cupStageKey(match.h2.result.round - run.maxRounds)]}</h3>
                    <Ties ties={match.h2.run.cup.ties.filter((t) => t.stage === cupStageKey(match.h2.result!.round - run.maxRounds))} />
                  </div>
                  {match.h2.run.cup.ties.some((t) => t.stage !== cupStageKey(match.h2.result!.round - run.maxRounds) && !t.winner) && (
                    <div>
                      <h3>Next</h3>
                      <Ties ties={match.h2.run.cup.ties.filter((t) => !t.winner)} />
                    </div>
                  )}
                </div>
              )}
              {match.h2.result.round <= run.maxRounds && match.h2.run.league && match.h2.run.league.last.length > 0 && (
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
                  api.run(daily).then(setRun).catch(() => {})
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
            {run.cup?.out ? (
              <>
                <small className="story-kicker">
                  {stars(run.cup.stars)} {run.cup.name}
                </small>
                <h2>{cupEnding(run.cup.id, run.cup.out).title}</h2>
                <p>{cupEnding(run.cup.id, run.cup.out).text}</p>
                <p className="muted">League: {EPILOGUE[run.cup.playoff ? 'relegated' : run.result]?.title}</p>
              </>
            ) : run.daily ? (
              <>
                <small className="story-kicker">📅 Daily challenge</small>
                <h2>{run.result === 'sacked' ? '📦 Sacked' : `${card(run, meta).title}!`}</h2>
                <p>Today's challenge is done. The table of the day is on the home page; a new challenge starts at midnight UTC. Send a friend the link: they get the same start as you.</p>
              </>
            ) : (
              <>
                <h2>{EPILOGUE[run.result]?.title}</h2>
                <p>{EPILOGUE[run.result]?.text}</p>
              </>
            )}
            <p>
              {run.league && (run.result !== 'sacked' || run.cup) && (
                <>
                  Final position: <b>{ord(run.league.position)}</b> with <b>{run.league.points}</b> points.{' '}
                </>
              )}
              Record: {run.wins} wins, {run.draws} draws, {run.losses} defeats.
              {last && ` Last match: ${last.gf}–${last.ga} against ${last.opponent}.`}
            </p>
            {run.cup && !run.cup.playoff && <CupOverview cup={run.cup} />}
            {run.league && <LeagueTable league={run.league} />}
            <div className="row center-row">
              <button onClick={onHome}>← Home</button>
              <button disabled={busy} onClick={share} title="A picture of your result and a link to the game">
                📤 Share
              </button>
              {!run.daily && (
                <button className="primary big" disabled={busy} onClick={newRun}>
                  New season
                </button>
              )}
            </div>
          </div>
        </div>
      )}
      {shopPhase && run.round === 1 && run.history.length === 0 && !introSeen && (
        <div className="overlay story-overlay">
          <div className="dialog story">
            <small className="story-kicker">Chapter 1</small>
            <h2>{INTRO.title}</h2>
            {(run.daily ? [...INTRO.text.slice(0, -1), DAILY_RULES] : INTRO.text).map((t, i) => (
              <p key={i}>{t}</p>
            ))}
            <button className="primary big" onClick={() => setIntroSeen(true)}>
              Take the job
            </button>
          </div>
        </div>
      )}
      {shopPhase && run.cup && run.round === run.maxRounds + 1 && !cupIntroSeen && (
        <div className="overlay story-overlay">
          <div className={`dialog story cup-${run.cup.id}`}>
            <small className="story-kicker">
              {run.cup.playoff ? 'Last chance' : 'Chapter 2'}
              {run.cup.stars > 0 && <span className="cup-stars"> · {stars(run.cup.stars)}</span>}
            </small>
            <h2>{run.cup.name}</h2>
            <p>{CUPS[run.cup.id]?.intro}</p>
            <p>{CUPS[run.cup.id]?.rules}</p>
            {!run.cup.playoff && (
              <>
                <h3>The draw</h3>
                <CupOverview cup={run.cup} />
              </>
            )}
            <button className="primary big" onClick={() => setCupIntroSeen(true)}>
              {run.cup.playoff ? 'One last chance' : 'To Europe ✈'}
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
