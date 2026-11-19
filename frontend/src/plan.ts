// The team plan: the players a manager wants to end up with. It is stored on
// the account, loaded once per sign-in and shared by every screen with a board.
import { useEffect, useSyncExternalStore } from 'react'
import { api } from './api'
import type { Meta, PlanView } from './api'
import type { BoardView } from './ClubBoard'

let plan: PlanView | null = null
let loading = false
const listeners = new Set<() => void>()
const emit = () => listeners.forEach((f) => f())

function load() {
  if (plan || loading) return
  loading = true
  api
    .plan()
    .then((p) => {
      plan = p
    })
    .catch(() => {
      /* without a plan the board simply shows none */
    })
    .finally(() => {
      loading = false
      emit()
    })
}

/** Forget the plan: another manager may sign in on this page. */
export function resetPlan() {
  plan = null
  emit()
}

export async function savePlan(players: string[]) {
  plan = await api.savePlan(players)
  emit()
}

export function usePlan(): PlanView | null {
  const p = useSyncExternalStore(
    (f) => {
      listeners.add(f)
      return () => listeners.delete(f)
    },
    () => plan,
  )
  useEffect(load, [p])
  return p
}

export interface PlanStatus {
  signed: string[] // planned players already at the club
  missing: string[]
  onSale: string[] // missing players in the market right now
  advice: string[]
}

/** Compares the plan with the club as it is and says what to do next. */
export function planStatus(meta: Meta, p: PlanView, view: BoardView): PlanStatus {
  const defs = Object.fromEntries(meta.players.map((d) => [d.id, d]))
  const names = (ids: string[]) => ids.map((id) => defs[id]?.name ?? id).join(', ')
  const owned = new Set(view.units.map((u) => u.def))
  const signed = p.players.filter((id) => owned.has(id))
  const missing = p.players.filter((id) => !owned.has(id))
  const onSale = missing.filter((id) => view.shop.includes(id))
  const advice: string[] = []
  if (p.players.length === 0) {
    advice.push('Pick the players you want to end up with. From then on they are marked in the market and you see what is still missing.')
    return { signed, missing, onSale, advice }
  }
  if (onSale.length) {
    const cost = onSale.reduce((n, id) => n + (defs[id]?.cost ?? 0), 0)
    advice.push(`In the market right now: ${names(onSale)} (${cost} 💰 together${cost > view.budget ? `, you have ${view.budget}` : ''}).`)
  }
  // A planned player he already owns is also worth buying again: three copies merge.
  const again = signed.filter((id) => view.shop.includes(id) && view.units.some((u) => u.def === id && u.stars < 3))
  if (again.length) advice.push(`Another copy of ${names(again)} is in the market: three identical players merge into one with an extra ★.`)
  if (missing.length === 0) advice.push('Every planned player is at the club. Now collect copies for ★★ and place them well.')
  const outfield = p.players.filter((id) => defs[id]?.role !== 'gk').length
  if (outfield > view.level) {
    const n = outfield - view.level
    advice.push(`The plan fields ${outfield} outfield players and your stadium fields ${view.level}: ${n} more level${n > 1 ? 's' : ''} to go.`)
  }
  // Players whose price the market does not offer yet at this stadium level.
  const tooEarly = missing.filter((id) => (view.shopOdds[(defs[id]?.cost ?? 1) - 1] ?? 0) === 0)
  if (tooEarly.length) advice.push(`Not in the market at stadium level ${view.level} yet: ${names(tooEarly)}. Their price only appears at a higher level.`)
  const syn = Object.fromEntries(meta.synergies.map((s) => [s.id, s]))
  for (const s of p.synergies) {
    const d = syn[s.id]
    const next = d?.tiers.find((t) => t > s.count)
    if (d && next !== undefined && next - s.count === 1) {
      advice.push(`One more ${d.name} player in the plan reaches ${d.name} (${next}): ${d.desc[d.tiers.indexOf(next)]}.`)
    }
  }
  return { signed, missing, onSale, advice }
}
