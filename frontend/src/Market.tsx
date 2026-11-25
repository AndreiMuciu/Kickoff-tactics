import { useEffect, useRef, useState } from 'react'
import type { Action, AugmentDef, Meta, PlayerDef, RunView } from './api'
import { Face } from './Face'
import type { Photos } from './photos'

const ROLE_ICON: Record<string, string> = { striker: '⚽', winger: '⚡', playmaker: '🎯', anchor: '🛡', defender: '🧱', gk: '🧤' }
const ROLE_LABEL: Record<string, string> = { striker: 'Striker', winger: 'Winger', playmaker: 'Playmaker', anchor: 'Anchor', defender: 'Defender', gk: 'Goalkeeper' }
const KIND_ICON: Record<string, string> = { economy: '💼', team: '🏃', crest: '🛡' }

export type MarketView = Pick<RunView, 'budget' | 'level' | 'maxLevel' | 'xp' | 'xpNeed' | 'shop' | 'units' | 'locked' | 'rerollCost'>

interface MarketProps {
  meta: Meta
  photos: Photos
  view: MarketView
  canAct: boolean
  busy: boolean
  onAction: (a: Action) => void
  planned?: Set<string>
  onHover?: (def: string) => void
  onDropSell?: (e: React.DragEvent) => void
  primary?: React.ReactNode
  compact?: boolean // the small market shown under a live match
}

// Market is the transfer market: five players on sale, the refresh, the lock
// and the stadium upgrade, with the budget right next to them.
export function Market({ meta, photos, view, canAct, busy, onAction, planned, onHover, onDropSell, primary, compact }: MarketProps) {
  const defs: Record<string, PlayerDef> = Object.fromEntries(meta.players.map((p) => [p.id, p]))
  const reroll = view.rerollCost ?? meta.rerollCost
  const bench = view.units.filter((u) => u.line < 0).length
  // The main button (Kick off, or the live timer) floats in the corner while
  // its place under the market is scrolled out of sight.
  const slot = useRef<HTMLSpanElement>(null)
  const [away, setAway] = useState(false)
  useEffect(() => {
    if (compact || !slot.current) return
    const io = new IntersectionObserver(([e]) => setAway(!e.isIntersecting))
    io.observe(slot.current)
    return () => io.disconnect()
  }, [compact])
  return (
    <div className={`market ${compact ? 'compact' : ''}`}>
      <div className="shop-cards" onDragOver={(e) => canAct && onDropSell && e.preventDefault()} onDrop={onDropSell}>
        {view.shop.map((id, i) => {
          const d = defs[id]
          if (!d)
            return (
              <div key={i} className="card empty">
                signed
              </div>
            )
          // What you already hold of this player, at any star level.
          const mine = view.units.filter((u) => u.def === id)
          const ones = mine.filter((u) => u.stars === 1).length
          const twos = mine.filter((u) => u.stars === 2).length
          const best = mine.reduce((m, u) => Math.max(m, u.stars), 0)
          // Buying completes a merge when two 1★ copies are waiting; with two 2★
          // on top of that, the same purchase chains straight into a 3★.
          const merges = ones === 2 ? (twos === 2 ? 3 : 2) : 0
          const inPlan = planned?.has(id)
          return (
            <button
              key={i}
              title={[inPlan ? 'In your team plan.' : '', merges ? `Buying him makes a ${'★'.repeat(merges)} player` : best ? `You own him at ${'★'.repeat(best)}` : ''].filter(Boolean).join(' ') || undefined}
              className={`card cost${d.cost} ${inPlan ? 'planned' : ''} ${best > 0 ? 'owned' : ''} ${merges === 2 ? 'completes' : ''} ${merges === 3 ? 'completes three' : ''}`}
              disabled={!canAct || busy || view.budget < d.cost}
              onClick={() => onAction({ type: 'buy', slot: i, def: id })}
              onMouseEnter={() => onHover?.(id)}
            >
              <Face def={d} photo={photos[id]} aspect={compact ? 1.25 : 0.95} tight={2.3} />
              <span className="card-cost">{d.cost} 💰</span>
              {best > 0 && (
                <span className={`card-own ${merges ? 'up' : ''}`}>
                  {'★'.repeat(merges || best)}
                  {merges > 0 && ' ↑'}
                </span>
              )}
              <span className="card-body">
                <b>{d.name}</b>
                <span className="card-top">
                  {ROLE_ICON[d.role]} {ROLE_LABEL[d.role]}
                  {!compact && d.style && ` · ${meta.synergies.find((s) => s.id === d.style)?.name}`}
                </span>
                {!compact && <span className="card-trait">{d.trait}</span>}
              </span>
            </button>
          )
        })}
      </div>
      <div className="shop-actions">
        <div className="wallet" title="Your budget">
          💰 <b>{view.budget}</b>
        </div>
        <button disabled={!canAct || busy || view.budget < reroll} onClick={() => onAction({ type: 'reroll' })}>
          🔄 New players ({reroll === 0 ? 'free' : `${reroll} 💰`})
        </button>
        <button
          className={`lock ${view.locked ? 'on' : ''}`}
          disabled={!canAct || busy}
          onClick={() => onAction({ type: 'lock' })}
          title={view.locked ? 'Locked: these players are still here in the next transfer window. Click to unlock.' : 'Keep these players for the next transfer window instead of a new draw.'}
        >
          {view.locked ? '🔒 Locked' : '🔓 Lock'}
        </button>
        <button
          disabled={!canAct || busy || view.level >= view.maxLevel || view.budget < meta.xpCost}
          onClick={() => onAction({ type: 'levelup' })}
          title="Each stadium level lets you field one more outfield player and brings better players to the market."
        >
          🏟 {compact ? `lvl ${view.level}` : `Stadium lvl ${view.level}`}
          {view.level < view.maxLevel ? ` · ${view.xp}/${view.xpNeed} (${meta.xpCost} 💰)` : ' · max'}
        </button>
        {compact && <span className="muted bench-count">Bench {bench}/{meta.benchSize}</span>}
        {primary && (
          <span ref={slot} className="primary-slot">
            {primary}
          </span>
        )}
      </div>
      {away && primary && <div className="primary-float">{primary}</div>}
    </div>
  )
}

export function augmentDefs(meta: Meta): Record<string, AugmentDef> {
  return Object.fromEntries((meta.augments ?? []).map((a) => [a.id, a]))
}

// AugmentPicker offers three augments; one must be chosen before the window
// closes, or the first is taken. As a modal it covers the board until the
// manager picks one, so the choice cannot be missed.
export function AugmentPicker({ meta, offer, canAct, busy, onAction, modal, owned, secondsLeft }: { meta: Meta; offer: string[]; canAct: boolean; busy: boolean; onAction: (a: Action) => void; modal?: boolean; owned?: string[]; secondsLeft?: number }) {
  const defs = augmentDefs(meta)
  if (offer.length === 0) return null
  const picker = (
    <div className={`augments-offer ${modal ? 'dialog' : ''}`} role={modal ? 'dialog' : undefined} aria-modal={modal || undefined} aria-label="Choose an augment">
      {modal ? (
        <>
          <div className="augment-head">
            <h2>✨ Choose an augment</h2>
            {secondsLeft !== undefined && (
              <span className={`augment-timer ${secondsLeft <= 10 ? 'urgent' : ''}`} title="Time left in the transfer window">
                ⏱ {secondsLeft}s
              </span>
            )}
          </div>
          <p className="muted">
            {secondsLeft !== undefined
              ? `It lasts the whole season. If you have not chosen when the timer runs out, the first one is taken.`
              : 'It lasts the whole season. Pick one to continue.'}
          </p>
        </>
      ) : (
        <p>
          <b>✨ Choose an augment</b> <span className="muted">· it lasts the whole season. If you do not choose, the first one is taken when the window closes.</span>
        </p>
      )}
      <div className="augment-cards">
        {offer.map((id, i) => {
          const a = defs[id]
          return (
            <button key={id} className={`augment ${a?.kind ?? ''}`} disabled={!canAct || busy} onClick={() => onAction({ type: 'augment', slot: i })}>
              <b>
                {KIND_ICON[a?.kind ?? ''] ?? '✨'} {a?.name ?? id}
              </b>
              <small>{a?.desc}</small>
            </button>
          )
        })}
      </div>
      {modal && owned && owned.length > 0 && <p className="muted augments-owned">You already have: {owned.map((id) => defs[id]?.name ?? id).join(', ')}.</p>}
    </div>
  )
  return modal ? <div className="overlay augment-overlay">{picker}</div> : picker
}

// AugmentList shows the augments a club already has.
export function AugmentList({ meta, list, title = 'Augments' }: { meta: Meta; list?: string[]; title?: string }) {
  const defs = augmentDefs(meta)
  const rounds = meta.augmentRounds ?? []
  return (
    <div className="panel">
      <h3>{title}</h3>
      {!list?.length && <p className="muted">None yet.{rounds.length ? ` Offered in rounds ${rounds.join(', ')}.` : ''}</p>}
      {list?.map((id) => (
        <div key={id} className="syn active" title={defs[id]?.desc}>
          <div className="syn-head">
            <b>
              {KIND_ICON[defs[id]?.kind ?? ''] ?? '✨'} {defs[id]?.name ?? id}
            </b>
          </div>
          <small>{defs[id]?.desc}</small>
        </div>
      ))}
    </div>
  )
}

// MatchMarket is the market during a live match: a drawer under the pitch.
// What is bought counts from the next match; the one being played is frozen.
export function MatchMarket(props: Omit<MarketProps, 'compact' | 'primary'> & { offer?: string[]; note?: string }) {
  const [open, setOpen] = useState(false)
  const { view, offer, note } = props
  return (
    <div className={`match-market ${open ? 'open' : ''}`}>
      <button className="match-market-toggle" onClick={() => setOpen(!open)}>
        🛒 Market · 💰 <b>{view.budget}</b> {offer?.length ? '· ✨ augment to choose' : ''} {open ? '▾' : '▴'}
      </button>
      {open && (
        <div className="match-market-body">
          <p className="muted">{note ?? 'Signings made now play from the next match: the line-up on the pitch is already set.'}</p>
          {offer && offer.length > 0 && <AugmentPicker meta={props.meta} offer={offer} canAct={props.canAct} busy={props.busy} onAction={props.onAction} />}
          <Market {...props} compact />
        </div>
      )}
    </div>
  )
}
