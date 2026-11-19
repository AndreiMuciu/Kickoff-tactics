import { useEffect, useRef, useState } from 'react'
import type { GameEvent, MatchData, Meta } from './api'
import { cropRect, initials } from './photos'
import type { Photos } from './photos'

const W = 1000
const H = 600
const R = 23
const TEAM = ['#38bdf8', '#fb923c']
const TEAM_DARK = ['#0c4a6e', '#7c2d12']

interface Props {
  data: MatchData
  meta: Meta
  photos: Photos
  startScore: [number, number]
  onDone: () => void
  // Live matches: the position comes from the shared server clock instead of
  // local playback, so there is no speed control and no skipping. `data.frames`
  // and `data.events` then grow in place while the match is on.
  clock?: () => number
  total?: number
}

interface Floater {
  x: number
  y: number
  text: string
  color: string
  born: number
}

function drawPitch(ctx: CanvasRenderingContext2D) {
  for (let i = 0; i < 10; i++) {
    ctx.fillStyle = i % 2 ? '#1f7a3d' : '#23863f'
    ctx.fillRect(i * 100, 0, 100, H)
  }
  ctx.strokeStyle = 'rgba(255,255,255,.75)'
  ctx.lineWidth = 3
  ctx.strokeRect(6, 6, W - 12, H - 12)
  ctx.beginPath()
  ctx.moveTo(W / 2, 6)
  ctx.lineTo(W / 2, H - 6)
  ctx.stroke()
  ctx.beginPath()
  ctx.arc(W / 2, H / 2, 70, 0, Math.PI * 2)
  ctx.stroke()
  for (const side of [0, 1]) {
    const x = side ? W - 6 : 6
    const d = side ? -1 : 1
    ctx.strokeRect(side ? x - 150 : x, H / 2 - 160, 150, 320)
    ctx.strokeRect(side ? x - 55 : x, H / 2 - 75, 55, 150)
    ctx.fillStyle = 'rgba(255,255,255,.9)'
    ctx.fillRect(side ? x : x - 6, H / 2 - 45, 6, 90)
    ctx.beginPath()
    ctx.arc(x + d * 110, H / 2, 4, 0, Math.PI * 2)
    ctx.fill()
  }
}

export default function MatchView({ data, meta, photos, startScore, onDone, clock, total }: Props) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const speedRef = useRef(1)
  const doneRef = useRef(onDone)
  doneRef.current = onDone
  const [speed, setSpeed] = useState(1)
  const [score, setScore] = useState<[number, number]>(startScore)
  const [minute, setMinute] = useState(0)
  const [feed, setFeed] = useState<{ id: number; text: string; team: number }[]>([])
  const [banner, setBanner] = useState<string | null>(null)
  const [pens, setPens] = useState<[number, number] | null>(null) // shootout tally, once it starts
  const totalRef = useRef(total)
  totalRef.current = total
  const skipRef = useRef(false)

  useEffect(() => {
    speedRef.current = speed
  }, [speed])

  useEffect(() => {
    const canvas = canvasRef.current!
    const ctx = canvas.getContext('2d')!
    // Render at device resolution so the pitch stays sharp on HiDPI screens.
    const dpr = Math.min(2, window.devicePixelRatio || 1)
    canvas.width = W * dpr
    canvas.height = H * dpr
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)

    const frames = data.frames
    const events = (data.events ??= [])
    const roster = data.roster
    const traits = Object.fromEntries(meta.players.map((p) => [p.id, p.trait]))
    // Portraits are only drawn, never read back, so no CORS mode is needed.
    const faces = roster.map((r) => {
      const src = photos[r.def]
      if (!src) return null
      const img = new Image()
      img.referrerPolicy = 'no-referrer'
      img.src = src
      return img
    })
    const startTick = (data.half - 1) * meta.halfTicks
    const fullTicks = meta.halfTicks * 2
    let pos = 0
    let evI = 0
    let last = performance.now()
    let raf = 0
    let finished = false
    let feedId = 0
    let lastXG = 0
    let flash = 0
    const sc: [number, number] = [startScore[0], startScore[1]]
    const pk: [number, number] = [0, 0]
    const booked = new Set<number>() // players on a yellow card
    const flashBanner = (text: string, ms: number) => {
      setBanner(text)
      window.clearTimeout(bannerTimer)
      bannerTimer = window.setTimeout(() => setBanner(null), ms / speedRef.current)
    }
    const floaters: Floater[] = []
    const trail: { x: number; y: number }[] = []
    let bannerTimer: number | undefined

    const name = (i: number) => roster[i]?.name ?? '?'
    const pushFeed = (text: string, team: number) => setFeed((f) => [{ id: feedId++, text, team }, ...f].slice(0, 9))
    const floatAt = (i: number, text: string, color: string, frame: number[]) =>
      floaters.push({ x: frame[3 + i * 3], y: frame[4 + i * 3] - 30, text, color, born: performance.now() })

    const handle = (e: GameEvent, frame: number[]) => {
      const xg = lastXG ? ` (xG ${lastXG.toFixed(2)})` : ''
      switch (e.type) {
        case 'shot':
          lastXG = e.xg ?? 0
          break
        case 'goal':
          sc[e.team]++
          setScore([sc[0], sc[1]])
          pushFeed(`⚽ GOAL! ${name(e.p)}${xg}`, e.team)
          flashBanner(`GOAL! ${name(e.p)}`, 1600)
          flash = 1
          break
        case 'foul':
          pushFeed(`Foul by ${name(e.p)} on ${name(e.q)}`, e.team)
          floatAt(e.p, 'Foul!', '#fde047', frame)
          break
        case 'yellow':
          booked.add(e.p)
          pushFeed(`🟨 ${name(e.p)} is booked`, e.team)
          break
        case 'red':
        case 'red2':
          // e.q carries the length of the suspension.
          pushFeed(`🟥 ${name(e.p)} is sent off${e.type === 'red2' ? ' (second yellow)' : ''}: suspended ${e.q} match${e.q > 1 ? 'es' : ''}`, e.team)
          flashBanner(`RED CARD! ${name(e.p)}`, 1600)
          break
        case 'freekick':
          pushFeed(`🎯 Free kick: ${name(e.p)} goes for goal`, e.team)
          flashBanner('FREE KICK', 1000)
          break
        case 'penalty':
          lastXG = 0
          pushFeed(`⚠ PENALTY! ${name(e.p)} steps up`, e.team)
          flashBanner('PENALTY!', 1300)
          break
        case 'shootout':
          setPens([0, 0])
          pushFeed('Penalty shootout', 0)
          flashBanner('PENALTY SHOOTOUT', 1500)
          break
        case 'pengoal':
          pk[e.team]++
          setPens([pk[0], pk[1]])
          pushFeed(`⚽ ${name(e.p)} scores`, e.team)
          floatAt(e.p, 'Scores!', '#4ade80', frame)
          break
        case 'penmiss':
          pushFeed(`❌ ${name(e.p)} misses`, e.team)
          floatAt(e.p, 'No goal!', '#f87171', frame)
          break
        case 'save':
          pushFeed(`🧤 ${name(e.p)} saves from ${name(e.q)}${xg}`, e.team)
          floatAt(e.p, 'Save!', '#fde68a', frame)
          break
        case 'miss':
          pushFeed(`❌ ${name(e.p)} shoots wide${xg}`, e.team)
          break
        case 'tackle':
          pushFeed(`🛡 ${name(e.p)} wins the ball from ${name(e.q)}`, e.team)
          floatAt(e.p, 'Tackle!', '#fff', frame)
          break
        case 'dribble':
          floatAt(e.p, 'Skill!', '#fff', frame)
          break
        case 'intercept':
          floatAt(e.p, 'Intercepted', '#e2e8f0', frame)
          break
        case 'ability': {
          const ab = traits[roster[e.p]?.def] ?? 'Moment'
          pushFeed(`✨ ${name(e.p)}: ${ab}`, e.team)
          floatAt(e.p, `✨ ${ab}`, '#fde047', frame)
          break
        }
      }
    }

    const draw = (a: number[], b: number[], t: number) => {
      const L = (i: number) => a[i] + (b[i] - a[i]) * t
      drawPitch(ctx)
      if (flash > 0) {
        ctx.fillStyle = `rgba(255,255,255,${flash * 0.35})`
        ctx.fillRect(0, 0, W, H)
        flash = Math.max(0, flash - 0.03)
      }
      const owner = a[2]
      for (const r of roster) {
        const x = L(3 + r.idx * 3)
        const y = L(4 + r.idx * 3)
        const flags = a[5 + r.idx * 3]
        if (flags & 8) continue // sent off
        const face = faces[r.idx]
        ctx.save()
        ctx.globalAlpha = flags & 1 ? 0.45 : 1
        ctx.fillStyle = 'rgba(0,0,0,.28)'
        ctx.beginPath()
        ctx.ellipse(x, y + R - 2, R - 2, 7, 0, 0, Math.PI * 2)
        ctx.fill()
        if (flags & 2) {
          ctx.strokeStyle = '#a7f3d0'
          ctx.lineWidth = 3
          ctx.beginPath()
          ctx.arc(x, y, R + 7, 0, Math.PI * 2)
          ctx.stroke()
        }
        if (flags & 4) {
          ctx.shadowColor = '#fde047'
          ctx.shadowBlur = 20
        }
        ctx.fillStyle = r.gk ? TEAM_DARK[r.team] : TEAM[r.team]
        ctx.beginPath()
        ctx.arc(x, y, R, 0, Math.PI * 2)
        ctx.fill()
        ctx.shadowBlur = 0
        if (face && face.complete && face.naturalWidth > 0) {
          ctx.save()
          ctx.beginPath()
          ctx.arc(x, y, R - 3, 0, Math.PI * 2)
          ctx.clip()
          // Crop a square centred on the face, using the same focus data as the cards.
          const c = cropRect(r.def, face.naturalWidth, face.naturalHeight, 1, 1.7)
          ctx.drawImage(face, c.x, c.y, c.w, c.h, x - R + 3, y - R + 3, 2 * R - 6, 2 * R - 6)
          ctx.restore()
        } else {
          ctx.fillStyle = r.gk ? '#fff' : '#0b1220'
          ctx.font = 'bold 14px system-ui, sans-serif'
          ctx.textAlign = 'center'
          ctx.textBaseline = 'middle'
          ctx.fillText(initials(r.name), x, y + 1)
        }
        // Team-coloured ring; the ball carrier gets a white one.
        ctx.strokeStyle = r.idx === owner ? '#fff' : r.gk ? TEAM_DARK[r.team] : TEAM[r.team]
        ctx.lineWidth = r.idx === owner ? 4 : 3
        ctx.beginPath()
        ctx.arc(x, y, R - 1, 0, Math.PI * 2)
        ctx.stroke()
        if (booked.has(r.idx)) {
          // a small yellow card next to a booked player
          ctx.fillStyle = '#fde047'
          ctx.strokeStyle = 'rgba(0,0,0,.6)'
          ctx.lineWidth = 1
          ctx.fillRect(x + R - 6, y - R - 2, 8, 11)
          ctx.strokeRect(x + R - 6, y - R - 2, 8, 11)
        }
        if (r.stars > 1) {
          ctx.fillStyle = '#fde047'
          ctx.font = '12px system-ui, sans-serif'
          ctx.textAlign = 'center'
          ctx.textBaseline = 'middle'
          ctx.fillText('★'.repeat(r.stars), x, y - R - 8)
        }
        ctx.restore()
      }
      // ball with a short trail
      const bx = L(0)
      const by = L(1)
      trail.push({ x: bx, y: by })
      if (trail.length > 8) trail.shift()
      trail.forEach((p, i) => {
        ctx.fillStyle = `rgba(255,255,255,${(i / trail.length) * 0.3})`
        ctx.beginPath()
        ctx.arc(p.x, p.y, 4, 0, Math.PI * 2)
        ctx.fill()
      })
      ctx.fillStyle = '#fff'
      ctx.strokeStyle = '#111'
      ctx.lineWidth = 1.5
      ctx.beginPath()
      ctx.arc(bx, by, 6, 0, Math.PI * 2)
      ctx.fill()
      ctx.stroke()
      // floating labels
      const now = performance.now()
      for (let i = floaters.length - 1; i >= 0; i--) {
        const f = floaters[i]
        const age = (now - f.born) / 1100
        if (age >= 1) {
          floaters.splice(i, 1)
          continue
        }
        ctx.globalAlpha = 1 - age
        ctx.font = 'bold 15px system-ui, sans-serif'
        ctx.textAlign = 'center'
        ctx.lineWidth = 3
        ctx.strokeStyle = 'rgba(0,0,0,.7)'
        ctx.strokeText(f.text, f.x, f.y - age * 22)
        ctx.fillStyle = f.color
        ctx.fillText(f.text, f.x, f.y - age * 22)
        ctx.globalAlpha = 1
      }
    }

    const loop = (now: number) => {
      // The first animation timestamp can be slightly older than `last`; a
      // negative step used to produce frame -1 and kill the loop (frozen pitch).
      const dt = Math.max(0, Math.min(0.1, (now - last) / 1000))
      last = now
      if (clock) pos = clock()
      else {
        pos += dt * 10 * speedRef.current
        if (skipRef.current) pos = frames.length
      }
      if (frames.length === 0) {
        drawPitch(ctx)
        raf = requestAnimationFrame(loop)
        return
      }
      const i = Math.max(0, Math.min(frames.length - 1, Math.floor(pos)))
      const tick = startTick + i + 1
      while (evI < events.length && events[evI].t <= tick) {
        handle(events[evI], frames[i])
        evI++
      }
      setMinute(tick > fullTicks ? 91 : Math.min(90, Math.floor((tick / fullTicks) * 90)))
      const next = frames[Math.min(frames.length - 1, i + 1)]
      draw(frames[i], next, Math.max(0, Math.min(1, pos - i)))
      if (clock) {
        // On the shared clock the replay never stops by itself: more frames can
        // still arrive after the final whistle (a penalty shootout).
        if (!finished && pos >= (totalRef.current ?? Infinity) - 1) {
          finished = true
          doneRef.current()
        }
      } else if (pos >= frames.length - 1 && !finished) {
        finished = true
        window.setTimeout(() => doneRef.current(), skipRef.current ? 0 : 900)
        return
      }
      raf = requestAnimationFrame(loop)
    }
    raf = requestAnimationFrame(loop)
    return () => {
      cancelAnimationFrame(raf)
      window.clearTimeout(bannerTimer)
    }
    // A new half replaces the component (keyed by half), so this runs once.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data])

  return (
    <div className="match">
      <div className="scoreboard">
        <span className="team t0">{data.names[0]}</span>
        <span className="score">
          {score[0]} : {score[1]}
          {pens && (
            <small className="pk">
              pens {pens[0]} – {pens[1]}
            </small>
          )}
        </span>
        <span className="team t1">{data.names[1]}</span>
        <span className="clock">{minute > 90 ? 'penalties' : `${minute}' · ${minute < 45 ? 'first half' : 'second half'}`}</span>
      </div>
      <div className="match-body">
        <div className="canvas-wrap">
          <canvas ref={canvasRef} width={W} height={H} />
          {banner && <div className="banner">{banner}</div>}
        </div>
        <aside className="feed">
          <h4>Commentary</h4>
          {feed.length === 0 && <p className="muted">Kick-off…</p>}
          {feed.map((f) => (
            <p key={f.id} className={`t${f.team}`}>
              {f.text}
            </p>
          ))}
        </aside>
      </div>
      {clock ? (
        <div className="match-controls">
          <span className="live-tag">● LIVE · both managers see the same moment</span>
        </div>
      ) : (
        <div className="match-controls">
          {[1, 2, 4].map((s) => (
            <button key={s} className={speed === s ? 'on' : ''} onClick={() => setSpeed(s)}>
              {s}×
            </button>
          ))}
          <button onClick={() => (skipRef.current = true)}>Skip ⏭</button>
        </div>
      )}
    </div>
  )
}
