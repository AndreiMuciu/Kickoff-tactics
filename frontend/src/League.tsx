import type { CupTie, CupView, LeagueResult, LeagueView } from './api'
import { STAGES } from './story'

const ord = (n: number) => `${n}${n % 10 === 1 && n !== 11 ? 'st' : n % 10 === 2 && n !== 12 ? 'nd' : n % 10 === 3 && n !== 13 ? 'rd' : 'th'}`
export { ord }

// The solo league table: champion, European places and relegation marked. A
// European group (`group`) marks only the places that go through.
export function LeagueTable({ league, compact, group, keyless }: { league: LeagueView; compact?: boolean; group?: boolean; keyless?: boolean }) {
  const zone = (pos: number) =>
    group ? (pos <= league.europe ? 'europe' : '') : pos === 1 ? 'champ' : pos <= league.europe ? 'europe' : pos >= league.relegate ? 'drop' : ''

  return (
    <div className={`league ${compact ? 'compact' : ''}`}>
      <table>
        <thead>
          <tr>
            <th>#</th>
            <th className="club">Club</th>
            <th title="Played">P</th>
            {!compact && (
              <>
                <th title="Won">W</th>
                <th title="Drawn">D</th>
                <th title="Lost">L</th>
              </>
            )}
            <th title="Goal difference">GD</th>
            <th title="Points">Pts</th>
          </tr>
        </thead>
        <tbody>
          {league.table.map((t, i) => (
            <tr key={t.name} className={`${zone(i + 1)} ${t.you ? 'you' : ''}`}>
              <td>{i + 1}</td>
              <td className="club" title={t.name}>{t.name}</td>
              <td>{t.p}</td>
              {!compact && (
                <>
                  <td>{t.w}</td>
                  <td>{t.d}</td>
                  <td>{t.l}</td>
                </>
              )}
              <td>{t.gf - t.ga > 0 ? `+${t.gf - t.ga}` : t.gf - t.ga}</td>
              <td>
                <b>{t.pts}</b>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {keyless ? null : group ? (
        <p className="league-key">
          <i className="europe" /> {league.europe === 1 ? 'Only first place goes back up' : `Top ${league.europe} go through to the quarter-finals`}
        </p>
      ) : (
        <p className="league-key">
          <i className="champ" /> Champions: European Crown <i className="europe" /> 2–3: Shield, {league.europe}: Plate <i className="drop" /> Relegation ({league.relegate}–{league.table.length})
        </p>
      )}
    </div>
  )
}

// The other results of a matchday, told like a vidiprinter.
export function Results({ results, you }: { results: LeagueResult[]; you?: boolean }) {
  const shown = you ? results : results.filter((r) => r.home !== 'Your Club' && r.away !== 'Your Club')
  if (shown.length === 0) return null
  return (
    <div className="league-results">
      {shown.map((r, i) => (
        <p key={i}>
          <span className={r.hg > r.ag ? 'won' : ''}>{r.home}</span> <b>
            {r.hg}–{r.ag}
          </b>{' '}
          <span className={r.ag > r.hg ? 'won' : ''}>{r.away}</span>
        </p>
      ))}
    </div>
  )
}

// Knockout ties, stage by stage; the winner in bold, "pens" when it took a shootout.
export function Ties({ ties }: { ties: CupTie[] }) {
  const stages = ['quarter', 'semi', 'final'].filter((st) => ties.some((t) => t.stage === st))
  return (
    <div className="cup-ties">
      {stages.map((st) => (
        <div key={st}>
          <h4>{STAGES[st]}</h4>
          {ties
            .filter((t) => t.stage === st)
            .map((t, i) => (
              <p key={i} className={t.home === 'Your Club' || t.away === 'Your Club' ? 'you' : ''}>
                <span className={t.winner === t.home ? 'won' : ''}>{t.home}</span> <b>{t.winner ? `${t.hg}–${t.ag}` : 'v'}</b>{' '}
                <span className={t.winner === t.away ? 'won' : ''}>{t.away}</span>
                {t.pens && <small> (pens)</small>}
              </p>
            ))}
        </div>
      ))}
    </div>
  )
}

// Every group of a European cup and the bracket so far.
export function CupOverview({ cup }: { cup: CupView }) {
  return (
    <div className="cup-overview">
      <div className="cup-groups">
        {cup.groups.map((g, i) => (
          <div key={g.letter} className={i === cup.mine ? 'mine' : ''}>
            <h4>
              Group {g.letter}
              {i === cup.mine ? ' · yours' : ''}
            </h4>
            <LeagueTable league={{ table: g.table, last: g.last, played: 0, position: 0, points: 0, europe: 2, relegate: 0 }} compact group keyless />
          </div>
        ))}
      </div>
      {cup.ties.length > 0 && <Ties ties={cup.ties} />}
      {cup.winner && <p className="cup-winner">🏆 Winner: {cup.winner}</p>}
    </div>
  )
}
