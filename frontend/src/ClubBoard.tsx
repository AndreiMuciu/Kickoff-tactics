import { useMemo, useState } from 'react'
import type { Action, ItemDef, MatchData, Meta, PlayerDef, RunView, SynergyView, Unit } from './api'
import { Face } from './Face'
import { AugmentList, AugmentPicker, Market } from './Market'
import { planStatus, usePlan } from './plan'
import Planner from './Planner'
import { shortName } from './photos'
import type { Photos } from './photos'

export const ROLE: Record<string, { icon: string; label: string }> = {
  striker: { icon: '⚽', label: 'Striker' },
  winger: { icon: '⚡', label: 'Winger' },
  playmaker: { icon: '🎯', label: 'Playmaker' },
  anchor: { icon: '🛡', label: 'Anchor' },
  defender: { icon: '🧱', label: 'Defender' },
  gk: { icon: '🧤', label: 'Goalkeeper' },
}
const KIND_LABEL: Record<string, string> = { style: 'Styles', role: 'Positions' }
const STAT_NAMES = ['Pace', 'Shooting', 'Passing', 'Dribbling', 'Defending', 'Physical', 'Stamina']
const STAR_MUL = [0, 1, 1.3, 1.65]
const LINE_NAMES = ['Defence', 'Midfield', 'Attack']
// Income explains, during the transfer window, what the club is paid after the
// match and what saving is worth right now.
function Income({ budget, meta, interestMax, lossCost }: { budget: number; meta: Meta; interestMax?: number; lossCost?: [number, number] }) {
  // A page left open across a server update still holds the older settings,
  // without these three: fall back to the rule's values rather than show NaN.
  const base = meta.baseIncome ?? 5
  const step = meta.interestStep ?? 10
  const max = interestMax ?? meta.interestMax ?? 5
  const interest = Math.min(Math.floor(Math.max(budget, 0) / step), max)
  const cap = step * max
  const missing = (interest + 1) * step - budget
  return (
    <details className="income">
      <summary>
        💰 Income after the match: {base} + <b>{interest}</b> interest
        {interest < max ? ` · ${missing} more in the bank for +${interest + 1}` : ' · interest is at its maximum'}
      </summary>
      <ul>
        <li>
          <b>Every match:</b> +{base}.
        </li>
        <li>
          <b>Interest:</b> +1 for every {step} 💰 you still hold when the match ends, up to +{max} at {cap} 💰. Money you
          spend before kick-off earns nothing.
        </li>
        <li>
          <b>Win:</b> +1.
        </li>
        <li>
          <b>Streak:</b> +1 after 2 results in a row, +2 after 4. Wins or defeats both count.
        </li>
        {lossCost && lossCost[1] > 0 && (
          <li>
            <b>Next defeat:</b> costs {lossCost[0]} of the owner's patience when lost by one goal, up to {lossCost[1]} by three or more.
            Defeats get more expensive as the season goes on.
          </li>
        )}
      </ul>
    </details>
  )
}

// What standing on a line means in the match: a short tag on the pitch and a
// full sentence in the legend under it.
const LINE_HINTS = [
  { tag: 'holds the back', text: 'stays in your own half, passes the ball on instead of carrying it forward and does not press high. With nobody ahead of him, or under All-Out Attack, he goes forward too.' },
  { tag: 'links the play', text: 'follows the ball both ways and passes a little more often.' },
  { tag: 'stays high', text: 'first to arrive in the box, last to drop back.' },
]
// PlayerCard is the piece on the pitch and the bench: the player's face fills
// the card, with name, position and stars over it.
function PlayerCard({ unit, def, photo, selected, enemy, item }: { unit: Unit; def: PlayerDef; photo?: string; selected?: boolean; enemy?: boolean; item?: ItemDef }) {
  return (
    <div
      className={`pcard cost${def.cost} ${selected ? 'selected' : ''} ${enemy ? 'enemy' : ''} ${unit.ban ? 'banned' : ''}`}
      title={unit.ban ? `Suspended for ${unit.ban} more match${unit.ban > 1 ? 'es' : ''}: he does not play, even if left on the pitch.` : undefined}
    >
      <Face def={def} photo={photo} aspect={0.8} tight={2.1} />
      {!!unit.ban && <span className="pcard-ban">🟥 {unit.ban}</span>}
      {item && (
        <span className={`pcard-item ${unit.ban ? 'low' : ''}`} title={`${item.name}: ${item.desc}`}>
          {item.icon}
        </span>
      )}
      <span className="pcard-stars">{'★'.repeat(unit.stars)}</span>
      <span className="pcard-name">
        {ROLE[def.role].icon} {shortName(def.name)}
      </span>
    </div>
  )
}

// The kit bag: items not handed to a player yet. Pick one, then click a
// player (or drag it onto him) to hand it over.
function KitBag({ meta, items, picked, canAct, onPick }: { meta: Meta; items: string[]; picked: number | null; canAct: boolean; onPick: (i: number | null) => void }) {
  const defs = Object.fromEntries((meta.items ?? []).map((it) => [it.id, it]))
  return (
    <div className="panel kitbag">
      <h3>Kit bag</h3>
      {items.length === 0 && <p className="muted">Empty. A new item arrives every {meta.itemEvery ?? 3} rounds.</p>}
      {items.map((id, i) => (
        <button
          key={i}
          className={`kit-item ${picked === i ? 'on' : ''}`}
          disabled={!canAct}
          draggable={canAct}
          onDragStart={(e) => e.dataTransfer.setData('text/plain', `item:${i}`)}
          onClick={() => onPick(picked === i ? null : i)}
          title={defs[id]?.desc}
        >
          <b>
            {defs[id]?.icon} {defs[id]?.name ?? id}
          </b>
          <small>{defs[id]?.desc}</small>
        </button>
      ))}
      {picked !== null && <p className="muted small">Now click the player who should wear it.</p>}
      {items.length > 0 && picked === null && <p className="muted small">One item per player. Selling a player gives his item back.</p>}
    </div>
  )
}

function SynergyList({ list, meta, title }: { list: SynergyView[]; meta: Meta; title: string }) {
  const defs = useMemo(() => Object.fromEntries(meta.synergies.map((s) => [s.id, s])), [meta])
  const sorted = [...list].sort((a, b) => b.active - a.active || b.count - a.count)
  return (
    <div className="panel">
      <h3>{title}</h3>
      {sorted.length === 0 && <p className="muted">Nobody on the pitch yet.</p>}
      {(['style', 'role'] as const).map((kind) => {
        const rows = sorted.filter((s) => defs[s.id]?.kind === kind)
        if (rows.length === 0) return null
        return (
          <div key={kind} className="syn-group">
            <h5>{KIND_LABEL[kind]}</h5>
            {rows.map((s) => {
              const d = defs[s.id]
              const next = d.tiers.find((t) => t > s.count)
              return (
                <div key={s.id} className={`syn ${s.active >= 0 ? 'active' : ''}`} title={d.desc.map((x, i) => `${d.tiers[i]}: ${x}`).join('\n')}>
                  <div className="syn-head">
                    <b>{d.name}</b>
                    <span>
                      {s.count}/{next ?? d.tiers[d.tiers.length - 1]}
                    </span>
                  </div>
                  <small>{s.active >= 0 ? d.desc[s.active] : `At ${d.tiers[0]}: ${d.desc[0]}`}</small>
                </div>
              )
            })}
          </div>
        )
      })}
    </div>
  )
}

function PlayerSheet({ stars, def, meta, photo }: { stars: number; def: PlayerDef | null; meta: Meta; photo?: string }) {
  if (!def)
    return (
      <div className="panel sheet">
        <h3>Player card</h3>
        <p className="muted">Hover a player to see the details.</p>
      </div>
    )
  const style = meta.synergies.find((s) => s.id === def.style)
  const mul = STAR_MUL[stars]
  return (
    <div className="panel sheet">
      <div className={`sheet-face cost${def.cost}`}>
        <Face key={def.id} def={def} photo={photo} aspect={1.6} tight={3.4} />
        <div className="sheet-title">
          <b>{def.name}</b>
          <span className="stars">{'★'.repeat(stars)}</span>
        </div>
      </div>
      <p className="tags">
        {def.cost > 0 && <span className={`tag cost${def.cost}`}>{def.cost} 💰</span>}
        <span className="tag">
          {ROLE[def.role].icon} {ROLE[def.role].label}
        </span>
        {style && <span className="tag">{style.name}</span>}
      </p>
      <div className="trait">
        <b>
          {def.trait}
          {def.active && <em>Moment</em>}
        </b>
        <small>{def.traitDesc}</small>
      </div>
      {def.role === 'gk' ? (
        <div className="stat">
          <span>Reflexes</span>
          <div className="bar">
            <i style={{ width: `${Math.min(100, (def.ref ?? 0) + (stars - 1) * 8)}%` }} />
          </div>
          <b>{(def.ref ?? 0) + (stars - 1) * 8}</b>
        </div>
      ) : (
        def.stats.map((v, i) => {
          const val = Math.round(v * mul)
          return (
            <div key={i} className="stat">
              <span>{STAT_NAMES[i]}</span>
              <div className="bar">
                <i style={{ width: `${Math.min(100, val)}%` }} />
              </div>
              <b>{val}</b>
            </div>
          )
        })
      )}
    </div>
  )
}

export function StatsTable({ data }: { data: MatchData }) {
  const [a, b] = data.stats
  const poss = a.poss + b.poss || 1
  const rows: [string, string | number, string | number][] = [
    ['Possession', `${Math.round((a.poss / poss) * 100)}%`, `${Math.round((b.poss / poss) * 100)}%`],
    ['Shots', a.shots, b.shots],
    ['xG', a.xg.toFixed(2), b.xg.toFixed(2)],
    ['Passes', a.passes, b.passes],
    ['Tackles', a.tackles, b.tackles],
  ]
  return (
    <table className="stats">
      <thead>
        <tr>
          <th className="t0">{data.names[0]}</th>
          <th />
          <th className="t1">{data.names[1]}</th>
        </tr>
      </thead>
      <tbody>
        {rows.map(([n, x, y]) => (
          <tr key={n}>
            <td>{x}</td>
            <td className="muted">{n}</td>
            <td>{y}</td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}

export function Shootout({ data }: { data: MatchData }) {
  if (!data.pens) return null
  return (
    <div className="pens">
      <p>Penalty shootout</p>
      {[0, 1].map((t) => (
        <div key={t} className={`t${t}`}>
          {data.names[t]}:{' '}
          {data
            .pens!.filter((p) => p.team === t)
            .map((p, i) => (
              <span key={i}>{p.scored ? '⚽' : '❌'}</span>
            ))}
        </div>
      ))}
    </div>
  )
}

// BoardView is what the board needs to draw a club: a season and a live
// series both provide it.
export type BoardView = Pick<
  RunView,
  'budget' | 'level' | 'maxLevel' | 'xp' | 'xpNeed' | 'slots' | 'shop' | 'shopOdds' | 'units' | 'synergies' | 'opponent' | 'oppSynergies' | 'locked' | 'rerollCost' | 'interestMax' | 'augments' | 'augmentOffer' | 'lossCost' | 'items'
>

interface BoardProps {
  meta: Meta
  photos: Photos
  view: BoardView
  canAct: boolean // transfer window open
  busy: boolean
  onAction: (a: Action) => void
  oppTitle: string
  primary?: React.ReactNode // main button or timer, right of the market actions
  side?: React.ReactNode // extra panel under the opponent's bonuses
  windowLeft?: number // seconds left in a timed transfer window (live games only)
  oppAugments?: string[] // spectating: the watched club's augments
}

// ClubBoard is the management screen: pitch, bench and transfer market. It
// only reports intents through onAction; the server decides what happens.
export default function ClubBoard({ meta, photos, view, canAct, busy, onAction, oppTitle, primary, side, oppAugments, windowLeft }: BoardProps) {
  const [selected, setSelected] = useState<string | null>(null)
  const [picked, setPicked] = useState<number | null>(null) // an item from the kit bag, about to be handed over
  const items = useMemo(() => Object.fromEntries((meta.items ?? []).map((it) => [it.id, it])), [meta])
  const itemOf = (u: Unit) => (u.item ? items[u.item] : undefined)
  const [hover, setHover] = useState<{ def: string; stars: number } | null>(null)
  const [planning, setPlanning] = useState(false)
  const plan = usePlan()
  const planned = new Set(plan?.players)
  const status = plan ? planStatus(meta, plan, view) : null
  const defs = useMemo(() => Object.fromEntries(meta.players.map((p) => [p.id, p])), [meta])

  const GK = meta.gkLine
  const bench = view.units.filter((u) => u.line < 0)
  const board = view.units.filter((u) => u.line >= 0 && u.line < GK)
  const keeper = view.units.find((u) => u.line === GK)
  const oppKeeper = view.opponent.units.find((u) => u.line === GK)
  const sel = (canAct && view.units.find((u) => u.uid === selected)) || null

  const clickCell = (line: number, lane: number, occupant?: Unit) => {
    if (!canAct) return
    if (picked !== null) {
      if (occupant) onAction({ type: 'equip', slot: picked, uid: occupant.uid })
      setPicked(null)
      return
    }
    if (sel) {
      if (occupant?.uid === sel.uid) return setSelected(null)
      onAction({ type: 'move', uid: sel.uid, line, lane })
      setSelected(null)
    } else if (occupant) setSelected(occupant.uid)
  }
  const drop = (e: React.DragEvent, line: number, lane: number) => {
    e.preventDefault()
    const uid = e.dataTransfer.getData('text/plain')
    if (uid.startsWith('item:')) {
      const occupant = view.units.find((u) => u.line === line && u.lane === lane)
      if (occupant) onAction({ type: 'equip', slot: Number(uid.slice(5)), uid: occupant.uid })
      setPicked(null)
      return
    }
    if (uid && view.units.some((u) => u.uid === uid)) onAction({ type: 'move', uid, line, lane })
    setSelected(null)
  }
  const cell = (line: number, lane: number, occupant: Unit | undefined, cls: string) => (
    <div
      key={`${line}-${lane}`}
      className={`cell ${cls} ${sel || (canAct && picked !== null) ? 'target' : ''}`}
      onClick={() => clickCell(line, lane, occupant)}
      onDragOver={(e) => canAct && e.preventDefault()}
      onDrop={(e) => canAct && drop(e, line, lane)}
    >
      {occupant && defs[occupant.def] && (
        <div draggable={canAct} onDragStart={(e) => e.dataTransfer.setData('text/plain', occupant.uid)} onMouseEnter={() => setHover({ def: occupant.def, stars: occupant.stars })}>
          <PlayerCard unit={occupant} def={defs[occupant.def]} photo={photos[occupant.def]} selected={occupant.uid === sel?.uid} item={itemOf(occupant)} />
        </div>
      )}
    </div>
  )
  const sellValue = (u: Unit) => {
    const c = defs[u.def].cost
    if (c === 0) return 0
    return u.stars === 2 ? c * 3 - 1 : u.stars === 3 ? c * 9 - 1 : c
  }
  // Dragging a player onto the Sell box or onto the market sells him.
  const dropSell = (e: React.DragEvent) => {
    e.preventDefault()
    const u = view.units.find((x) => x.uid === e.dataTransfer.getData('text/plain'))
    if (canAct && u && u.line !== GK) onAction({ type: 'sell', uid: u.uid })
    setSelected(null)
  }
  const shown = hover ?? (sel ? { def: sel.def, stars: sel.stars } : null)
  const canSell = !!sel && sel.line !== GK

  return (
    <main className="layout">
      <section className="side">
        <SynergyList list={view.synergies} meta={meta} title="Your team" />
        {(meta.augments?.length ?? 0) > 0 && <AugmentList meta={meta} list={view.augments} />}
        {(meta.items?.length ?? 0) > 0 && <KitBag meta={meta} items={view.items ?? []} picked={canAct ? picked : null} canAct={canAct && !busy} onPick={(i) => (setPicked(i), setSelected(null))} />}
        <PlayerSheet stars={shown?.stars ?? 1} def={shown ? defs[shown.def] : null} meta={meta} photo={shown ? photos[shown.def] : undefined} />
      </section>

      <section className="center-col">
        {canAct && <AugmentPicker meta={meta} offer={view.augmentOffer ?? []} canAct={canAct} busy={busy} onAction={onAction} modal owned={view.augments} secondsLeft={windowLeft} />}
        {plan && status && (
          <p className="plan-strip">
            <button onClick={() => setPlanning(true)}>📋 Team plan</button>
            <span className="muted">
              {plan.players.length === 0
                ? 'No plan yet: choose the team you are building towards and the market will mark those players.'
                : `${status.signed.length}/${plan.players.length} signed${
                    status.onSale.length ? ` · in the market now: ${status.onSale.map((id) => defs[id]?.name ?? id).join(', ')}` : ''
                  }${status.missing.length ? ` · missing: ${status.missing.map((id) => defs[id]?.name ?? id).join(', ')}` : ' · complete'}`}
            </span>
          </p>
        )}
        <div className="pitch-head">
          <span className="t0">
            Your club · stadium level {view.level}: {board.length}/{view.slots} outfield + keeper
          </span>
          <span className="t1">{oppTitle}</span>
        </div>
        <div className="pitch">
          <div className="gk-col">{cell(GK, 0, keeper, 'gk-cell')}</div>
          <div className="half mine">
            {[0, 1, 2, 3, 4].map((lane) => [0, 1, 2].map((line) => cell(line, lane, board.find((u) => u.line === line && u.lane === lane), `line${line}`)))}
          </div>
          <div className="half theirs">
            {[0, 1, 2, 3, 4].map((row) =>
              [2, 1, 0].map((line) => {
                const u = view.opponent.units.find((x) => x.line === line && x.lane === 4 - row)
                return (
                  <div key={`${line}-${row}`} className="cell enemy-cell">
                    {u && defs[u.def] && (
                      <div onMouseEnter={() => setHover({ def: u.def, stars: u.stars })}>
                        <PlayerCard unit={u} def={defs[u.def]} photo={photos[u.def]} enemy item={itemOf(u)} />
                      </div>
                    )}
                  </div>
                )
              }),
            )}
          </div>
          <div className="gk-col">
            <div className="cell enemy-cell gk-cell">
              {oppKeeper && defs[oppKeeper.def] && (
                <div onMouseEnter={() => setHover({ def: oppKeeper.def, stars: oppKeeper.stars })}>
                  <PlayerCard unit={oppKeeper} def={defs[oppKeeper.def]} photo={photos[oppKeeper.def]} enemy item={itemOf(oppKeeper)} />
                </div>
              )}
            </div>
          </div>
          <div className="line-labels">
            <span>Goal</span>
            {LINE_NAMES.map((n, i) => (
              <span key={n}>
                {n}
                <small>{LINE_HINTS[i].tag}</small>
              </span>
            ))}
            {[...LINE_NAMES].reverse().map((n) => (
              <span key={`o${n}`}>{n}</span>
            ))}
            <span>Goal</span>
          </div>
        </div>
        <details className="line-help">
          <summary>Where you place a player decides how he plays</summary>
          <ul>
            {LINE_NAMES.map((n, i) => (
              <li key={n}>
                <b>{n}:</b> {LINE_HINTS[i].text}
              </li>
            ))}
          </ul>
        </details>

        <div className="bench-row">
          <div className="bench">{Array.from({ length: meta.benchSize }, (_, i) => cell(-1, i, bench.find((u) => u.lane === i), 'bench-cell'))}</div>
          <button
            className="sell"
            onDragOver={(e) => canAct && e.preventDefault()}
            onDrop={dropSell}
            aria-disabled={!canSell || busy}
            title={sel?.line === GK ? 'Swap another keeper into goal before selling this one.' : undefined}
            onClick={() => {
              if (sel && canSell && !busy) {
                onAction({ type: 'sell', uid: sel.uid })
                setSelected(null)
              }
            }}
          >
            {sel && sel.line !== GK ? `Sell +${sellValue(sel)} 💰` : canAct ? 'Sell: select a player or drag him here' : 'Sell'}
          </button>
          {sel?.item && (
            <button
              className="sell unequip"
              disabled={busy}
              onClick={() => {
                onAction({ type: 'unequip', uid: sel.uid })
                setSelected(null)
              }}
            >
              Take off {itemOf(sel)?.icon} {itemOf(sel)?.name}
            </button>
          )}
        </div>

        <div className="shop">
          <Market
            meta={meta}
            photos={photos}
            view={view}
            canAct={canAct}
            busy={busy}
            onAction={onAction}
            planned={planned}
            onHover={(id) => setHover({ def: id, stars: 1 })}
            onDropSell={dropSell}
            primary={primary}
          />
          <Income budget={view.budget} meta={meta} interestMax={view.interestMax} lossCost={view.lossCost} />
          {planning && plan && (
            <Planner
              meta={meta}
              plan={plan}
              view={view}
              synergies={<SynergyList list={plan.synergies} meta={meta} title="What the plan gives you" />}
              onClose={() => setPlanning(false)}
            />
          )}
          <p className="muted odds">Market odds: {view.shopOdds.map((o, i) => `${i + 1}💰 ${o}%`).join(' · ')}</p>
        </div>
      </section>

      <section className="side">
        <SynergyList list={view.oppSynergies} meta={meta} title={view.opponent.name || 'Opponent'} />
        {oppAugments && <AugmentList meta={meta} list={oppAugments} title={`${view.opponent.name}'s augments`} />}
        {side}
      </section>
    </main>
  )
}
