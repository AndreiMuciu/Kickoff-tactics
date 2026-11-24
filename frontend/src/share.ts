// Sharing a result: a square card drawn on a canvas (1080 × 1080, good for
// chats and stories) and a link back to the game. Phones get the system share
// sheet with the picture; elsewhere the picture is saved and the text copied.

export interface Card {
  kicker: string // "Daily challenge · 24 Nov" or "Season"
  title: string // "Champions", "3rd place"
  line?: string // a second headline: the European run
  stats: [string, string][] // label, value
  players: string[] // the best players of the club, with their stars
  footer: string // "Can you beat it?"
}

const W = 1080
const LIME = '#b6f23c'

function loadImage(src: string): Promise<HTMLImageElement | null> {
  return new Promise((ok) => {
    const img = new Image()
    img.onload = () => ok(img)
    img.onerror = () => ok(null)
    img.src = src
  })
}

function fit(ctx: CanvasRenderingContext2D, text: string, max: number, size: number, weight = 800): void {
  let s = size
  do ctx.font = `${weight} ${s}px system-ui, -apple-system, 'Segoe UI', sans-serif`
  while (ctx.measureText(text).width > max && (s -= 4) > 20)
}

export async function drawCard(c: Card): Promise<Blob> {
  const cv = document.createElement('canvas')
  cv.width = cv.height = W
  const ctx = cv.getContext('2d')!
  const bg = ctx.createRadialGradient(W / 2, 0, 80, W / 2, 200, 1100)
  bg.addColorStop(0, '#17264a')
  bg.addColorStop(1, '#0b1220')
  ctx.fillStyle = bg
  ctx.fillRect(0, 0, W, W)

  const logo = await loadImage('/emblem.jpg')
  if (logo) {
    ctx.save()
    ctx.beginPath()
    ctx.arc(150, 150, 96, 0, Math.PI * 2)
    ctx.clip()
    ctx.drawImage(logo, 54, 54, 192, 192)
    ctx.restore()
  }
  ctx.textBaseline = 'alphabetic'
  ctx.fillStyle = '#e6edf7'
  fit(ctx, 'KICKOFF TACTICS', 720, 56)
  ctx.fillText('KICKOFF TACTICS', 280, 140)
  ctx.fillStyle = '#8ea0bd'
  fit(ctx, c.kicker, 720, 36, 600)
  ctx.fillText(c.kicker, 280, 196)

  ctx.textAlign = 'center'
  ctx.fillStyle = LIME
  fit(ctx, c.title, W - 120, 112)
  ctx.fillText(c.title, W / 2, 400)
  let y = 400
  if (c.line) {
    ctx.fillStyle = '#fde047'
    fit(ctx, c.line, W - 140, 48, 700)
    ctx.fillText(c.line, W / 2, (y += 76))
  }

  // The numbers, in boxes side by side.
  y += 70
  const n = c.stats.length
  const bw = Math.min(280, (W - 120 - (n - 1) * 24) / n)
  let x = (W - (n * bw + (n - 1) * 24)) / 2
  for (const [label, value] of c.stats) {
    ctx.fillStyle = '#131c2e'
    ctx.strokeStyle = '#2a3a5c'
    ctx.lineWidth = 3
    ctx.beginPath()
    ctx.roundRect(x, y, bw, 170, 24)
    ctx.fill()
    ctx.stroke()
    ctx.fillStyle = '#e6edf7'
    fit(ctx, value, bw - 30, 68)
    ctx.fillText(value, x + bw / 2, y + 92)
    ctx.fillStyle = '#8ea0bd'
    fit(ctx, label, bw - 30, 30, 600)
    ctx.fillText(label, x + bw / 2, y + 140)
    x += bw + 24
  }
  y += 170

  if (c.players.length) {
    ctx.fillStyle = '#8ea0bd'
    fit(ctx, 'KEY PLAYERS', W - 120, 30, 700)
    ctx.fillText('KEY PLAYERS', W / 2, (y += 80))
    ctx.fillStyle = '#e6edf7'
    const names = c.players.join('  ·  ')
    fit(ctx, names, W - 120, 40, 600)
    ctx.fillText(names, W / 2, (y += 56))
  }

  ctx.fillStyle = LIME
  ctx.beginPath()
  ctx.roundRect(140, W - 170, W - 280, 100, 50)
  ctx.fill()
  ctx.fillStyle = '#0c1a08'
  const foot = `${c.footer}  ${location.host}`
  fit(ctx, foot, W - 340, 40)
  ctx.fillText(foot, W / 2, W - 106)

  return new Promise((ok, fail) => cv.toBlob((b) => (b ? ok(b) : fail(new Error('canvas'))), 'image/png'))
}

// share sends the card with its text and link: the share sheet where the
// browser can share files, else the picture is downloaded and the text copied.
export async function shareCard(card: Card, text: string, link: string): Promise<'shared' | 'saved' | 'cancelled'> {
  const blob = await drawCard(card)
  const file = new File([blob], 'kickoff-tactics.png', { type: 'image/png' })
  if (navigator.canShare?.({ files: [file] })) {
    try {
      await navigator.share({ files: [file], text: `${text} ${link}` })
      return 'shared'
    } catch (e) {
      if (e instanceof DOMException && e.name === 'AbortError') return 'cancelled'
    }
  }
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = 'kickoff-tactics.png'
  a.click()
  window.setTimeout(() => URL.revokeObjectURL(a.href), 5000)
  await copyText(`${text} ${link}`)
  return 'saved'
}

// shareLink shares just a text and a link (no picture): the share sheet on
// phones, the clipboard elsewhere. Returns whether it ended on the clipboard.
export async function shareLink(text: string, link: string): Promise<'shared' | 'copied' | 'cancelled' | 'failed'> {
  if (navigator.share && matchMedia('(pointer: coarse)').matches) {
    try {
      await navigator.share({ text, url: link })
      return 'shared'
    } catch (e) {
      if (e instanceof DOMException && e.name === 'AbortError') return 'cancelled'
    }
  }
  return (await copyText(`${text} ${link}`)) ? 'copied' : 'failed'
}

async function copyText(t: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(t)
    return true
  } catch {
    return false
  }
}

// The links a shared result carries: today's challenge against you, or an invitation to be friends.
export const challengeLink = (name: string) => `${location.origin}/?c=${encodeURIComponent(name)}`
export const friendLink = (name: string) => `${location.origin}/?add=${encodeURIComponent(name)}`
