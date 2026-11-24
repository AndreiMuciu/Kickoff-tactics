import type { LeagueResult, LeagueView } from './api'

const ord = (n: number) => `${n}${n % 10 === 1 && n !== 11 ? 'st' : n % 10 === 2 && n !== 12 ? 'nd' : n % 10 === 3 && n !== 13 ? 'rd' : 'th'}`
export { ord }

// The solo league table: champion, European places and relegation marked. A
// European group (`group`) marks only the places that go through.
export function LeagueTable({ league, compact, group }: { league: LeagueView; compact?: boolean; group?: boolean }) {
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
              <td className="club">{t.name}</td>
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
      {group ? (
        <p className="league-key">
          <i className="europe" /> Top {league.europe} go through to the semi-final
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
