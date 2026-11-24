import { useEffect, useState } from 'react'
import { api } from './api'
import type { DailyView, Me } from './api'
import { ord } from './League'
import { challengeLink, shareLink } from './share'

// The daily challenge on the home page: one attempt a day, the same start for
// every manager, and a table of the day. A challenge link (/?c=name) shows
// how the friend who sent it did.

const gd = (n: number) => (n > 0 ? `+${n}` : String(n))

function left(ms: number): string {
  const m = Math.max(0, Math.floor(ms / 60000))
  return m >= 60 ? `${Math.floor(m / 60)}h ${m % 60}m` : `${m}m`
}

export default function Daily({ me, challenger, onPlay, notify }: { me: Me; challenger: string | null; onPlay: () => void; notify: (m: string) => void }) {
  const [d, setD] = useState<DailyView | null>(null)
  useEffect(() => {
    api
      .daily(challenger)
      .then(setD)
      .catch(() => {})
  }, [challenger, me.name]) // a guest who creates an account shows up under the new name

  const challenge = async () => {
    const mine = d?.mine
    const text = mine
      ? `I made ${mine.points} pts (${ord(mine.place)} place) in today's Kickoff Tactics challenge. Can you beat me?`
      : `Play today's Kickoff Tactics challenge with me: same start for everyone, one attempt.`
    const r = await shareLink(text, challengeLink(me.name))
    if (r === 'copied') notify('Challenge link copied. Send it to a friend.')
    if (r === 'failed') notify(`Send this link: ${challengeLink(me.name)}`)
  }

  if (!d) return null
  const rival = d.rival
  return (
    <section className="panel daily">
      <h3>
        📅 Daily challenge <small className="muted">new one in {left(d.endsAt - Date.now())}</small>
      </h3>
      <p className="muted">Everyone gets the same start today: the same first market, the same rivals, the same league. Nine matchdays, one attempt. Most points wins the day.</p>
      {rival ? (
        <p className="rival-line">
          ⚔️ <b>{rival.name}</b> made <b>{rival.points} pts</b> ({gd(rival.gd)}), #{rival.rank} today.{' '}
          {d.mine ? (d.mine.points > rival.points || (d.mine.points === rival.points && d.mine.gd > rival.gd) ? 'You beat them! 🎉' : 'They are ahead of you.') : 'Beat them!'}
        </p>
      ) : (
        challenger &&
        challenger.toLowerCase() !== me.name.toLowerCase() && (
          <p className="rival-line">
            ⚔️ <b>{challenger}</b> challenged you. Their score shows here once they finish today's challenge.
          </p>
        )
      )}
      <div className="row daily-actions">
        {d.status === 'done' ? (
          <span className="daily-done">
            Done for today: <b>{d.mine ? `#${d.mine.rank}, ${d.mine.points} pts` : `${d.points ?? 0} pts`}</b>
          </span>
        ) : (
          <button className="primary big" onClick={onPlay}>
            {d.status === 'playing' ? `Continue · matchday ${d.round} ▶` : "Play today's challenge ▶"}
          </button>
        )}
        <button onClick={challenge} title="Send a friend a link to today's challenge">
          ⚔️ Challenge a friend
        </button>
      </div>
      {d.top.length > 0 && (
        <div className="daily-table">
          {d.top.map((r) => (
            <div key={r.rank} className={`stand ${r.you ? 'you' : ''}`}>
              <span>
                {r.rank}. {r.name}
              </span>
              <b>
                {r.points} pts <small className="muted">{gd(r.gd)}</small>
              </b>
            </div>
          ))}
          {d.mine && d.mine.rank > d.top.length && (
            <div className="stand you">
              <span>
                {d.mine.rank}. {d.mine.name}
              </span>
              <b>
                {d.mine.points} pts <small className="muted">{gd(d.mine.gd)}</small>
              </b>
            </div>
          )}
          <small className="muted">
            {d.players} {d.players === 1 ? 'manager has' : 'managers have'} finished today.
          </small>
        </div>
      )}
    </section>
  )
}
