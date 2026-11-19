import { useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { ApiError } from './api'
import type { Meta, PlanView } from './api'
import type { BoardView } from './ClubBoard'
import { planStatus, savePlan } from './plan'

interface Props {
  meta: Meta
  plan: PlanView
  view: BoardView
  synergies: ReactNode // the plan's combinations, drawn by the board
  onClose: () => void
}

const ROLE_LABEL: Record<string, string> = { striker: 'Striker', winger: 'Winger', playmaker: 'Playmaker', anchor: 'Anchor', defender: 'Defender', gk: 'Goalkeeper' }

/** The team planner: choose the players you are building towards. */
export default function Planner({ meta, plan, view, synergies, onClose }: Props) {
  const [filter, setFilter] = useState('all')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const defs = useMemo(() => Object.fromEntries(meta.players.map((d) => [d.id, d])), [meta])
  const styles = meta.synergies.filter((s) => s.kind === 'style')
  const styleName = (id: string) => styles.find((s) => s.id === id)?.name ?? ''

  const picked = new Set(plan.players)
  const outfield = plan.players.filter((id) => defs[id]?.role !== 'gk').length
  const keepers = plan.players.length - outfield
  const status = planStatus(meta, plan, view)
  const owned = new Set(view.units.map((u) => u.def))

  const save = async (players: string[]) => {
    if (busy) return
    setBusy(true)
    setError(null)
    try {
      await savePlan(players)
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'The game server is not responding.')
    } finally {
      setBusy(false)
    }
  }
  const toggle = (id: string) => save(picked.has(id) ? plan.players.filter((x) => x !== id) : [...plan.players, id])

  const shown = meta.players
    .filter((d) => d.cost > 0) // the academy keeper is nobody's plan
    .filter((d) => (filter === 'all' ? true : filter === 'gk' ? d.role === 'gk' : d.role !== 'gk' && d.style === filter))
  const costs = [...new Set(shown.map((d) => d.cost))].sort((a, b) => a - b)

  return (
    <div className="overlay" onClick={onClose}>
      <div className="dialog planner" onClick={(e) => e.stopPropagation()}>
        <h2>Team plan</h2>
        <p className="muted">
          Choose the team you are building towards. It is saved on your account and used in every game: planned players are marked in the market, and
          the board tells you what is still missing. The plan never buys anything by itself.
        </p>
        <div className="planner-cols">
          <div className="planner-pick">
            <div className="small-tabs">
              <button className={filter === 'all' ? 'on' : ''} onClick={() => setFilter('all')}>
                All
              </button>
              {styles.map((s) => (
                <button key={s.id} className={filter === s.id ? 'on' : ''} onClick={() => setFilter(s.id)}>
                  {s.name}
                </button>
              ))}
              <button className={filter === 'gk' ? 'on' : ''} onClick={() => setFilter('gk')}>
                Keepers
              </button>
            </div>
            {costs.map((c) => (
              <div key={c} className="planner-cost">
                <h5>{c} 💰</h5>
                <div className="planner-grid">
                  {shown
                    .filter((d) => d.cost === c)
                    .map((d) => {
                      const full = !picked.has(d.id) && (d.role === 'gk' ? keepers >= 1 : outfield >= plan.maxOutfield)
                      return (
                        <button
                          key={d.id}
                          className={`plan-pick cost${d.cost} ${picked.has(d.id) ? 'picked' : ''}`}
                          disabled={busy || full}
                          title={`${d.trait}: ${d.traitDesc}${full ? '\nThe plan is full: remove someone first.' : ''}`}
                          onClick={() => toggle(d.id)}
                        >
                          <b>{d.name}</b>
                          <small>
                            {ROLE_LABEL[d.role] ?? d.role}
                            {styleName(d.style) ? ` · ${styleName(d.style)}` : ''}
                          </small>
                        </button>
                      )
                    })}
                </div>
              </div>
            ))}
          </div>
          <div className="planner-side">
            <div className="panel">
              <h3>
                Your plan · {outfield}/{plan.maxOutfield} outfield · {keepers}/1 keeper
              </h3>
              {plan.players.length === 0 && <p className="muted">Nobody yet. Click players on the left.</p>}
              <div className="planner-chips">
                {plan.players.map((id) => (
                  <button key={id} className={`chip cost${defs[id]?.cost ?? 1} ${owned.has(id) ? 'signed' : ''}`} disabled={busy} title="Remove from the plan" onClick={() => toggle(id)}>
                    {owned.has(id) ? '✓ ' : ''}
                    {defs[id]?.name ?? id} ✕
                  </button>
                ))}
              </div>
              {plan.players.length > 0 && (
                <button disabled={busy} onClick={() => save([])}>
                  Clear the plan
                </button>
              )}
            </div>
            {plan.players.length > 0 && synergies}
            <div className="panel">
              <h3>Advice</h3>
              <ul className="planner-advice">
                {status.advice.map((a) => (
                  <li key={a}>{a}</li>
                ))}
              </ul>
            </div>
          </div>
        </div>
        {error && <p className="error">{error}</p>}
        <button className="primary" onClick={onClose}>
          Done
        </button>
      </div>
    </div>
  )
}
