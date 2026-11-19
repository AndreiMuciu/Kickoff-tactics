// Player portraits come from Wikipedia's public API: one batched request for
// the whole catalogue, restricted to freely licensed images, cached for the
// browser session. A player without a free image falls back to initials.
import type { PlayerDef } from './api'

const KEY = 'autofootball-photos-v4'
const ENDPOINT = 'https://en.wikipedia.org/w/api.php'

export type Photos = Record<string, string>

// Where the face is in each portrait: [centre x, centre y, face width], all as
// fractions of the image. Measured by hand against the current lead images, so
// cards can be cropped onto the face instead of the shirt.
const FOCUS: Record<string, [number, number, number]> = {
  mount: [0.46, 0.27, 0.33],
  ferran: [0.51, 0.3, 0.37],
  zubimendi: [0.5, 0.27, 0.34],
  olmo: [0.48, 0.3, 0.37],
  pedri: [0.49, 0.27, 0.36],
  saka: [0.49, 0.27, 0.43],
  rodri: [0.48, 0.25, 0.34],
  kdb: [0.475, 0.3, 0.38],
  messi: [0.5, 0.25, 0.37],
  maguire: [0.535, 0.27, 0.32],
  dragusin: [0.37, 0.3, 0.28],
  bastoni: [0.54, 0.25, 0.355],
  casemiro: [0.5, 0.27, 0.33],
  saliba: [0.49, 0.25, 0.37],
  dias: [0.52, 0.31, 0.4],
  vandijk: [0.42, 0.395, 0.55],
  chivu: [0.51, 0.31, 0.43],
  maldini: [0.52, 0.47, 0.45],
  nunez: [0.45, 0.19, 0.24],
  gallagher: [0.44, 0.385, 0.45],
  mctominay: [0.5, 0.27, 0.37],
  szoboszlai: [0.545, 0.3, 0.24],
  rice: [0.51, 0.24, 0.34],
  lautaro: [0.47, 0.31, 0.41],
  salah: [0.5, 0.35, 0.52],
  bellingham: [0.5, 0.3, 0.41],
  cruyff: [0.55, 0.375, 0.48],
  antony: [0.615, 0.13, 0.155],
  man: [0.67, 0.31, 0.32],
  leao: [0.51, 0.26, 0.37],
  felix: [0.54, 0.29, 0.37],
  musiala: [0.52, 0.3, 0.38],
  yamal: [0.5, 0.31, 0.37],
  vini: [0.5, 0.27, 0.37],
  mutu: [0.51, 0.395, 0.38],
  ghagi: [0.495, 0.395, 0.55],
  weghorst: [0.5, 0.34, 0.3],
  richarlison: [0.515, 0.35, 0.46],
  lukaku: [0.495, 0.29, 0.4],
  taa: [0.515, 0.365, 0.52],
  kane: [0.5, 0.29, 0.37],
  bruno: [0.48, 0.31, 0.43],
  haaland: [0.49, 0.25, 0.37],
  mbappe: [0.515, 0.235, 0.33],
  ronaldo: [0.5, 0.27, 0.355],
  pickford: [0.445, 0.185, 0.25],
  onana: [0.575, 0.395, 0.43],
  emi: [0.485, 0.195, 0.36],
  alisson: [0.45, 0.27, 0.43],
  neuer: [0.48, 0.22, 0.33],
  courtois: [0.49, 0.235, 0.32],
  donnarumma: [0.52, 0.32, 0.43],
  buffon: [0.45, 0.39, 0.34],
  cucurella: [0.51, 0.29, 0.32],
  bernardo: [0.49, 0.24, 0.31],
  wirtz: [0.53, 0.35, 0.35],
  modric: [0.505, 0.235, 0.37],
  zidane: [0.5, 0.42, 0.63],
  dier: [0.51, 0.365, 0.38],
  kante: [0.51, 0.27, 0.37],
  rudiger: [0.455, 0.24, 0.285],
  ramos: [0.495, 0.365, 0.44],
  beckenbauer: [0.55, 0.365, 0.44],
  werner: [0.5, 0.4, 0.44],
  luisdiaz: [0.41, 0.195, 0.28],
  valverde: [0.5, 0.46, 0.42],
  son: [0.48, 0.385, 0.49],
  matthaus: [0.48, 0.33, 0.42],
  sancho: [0.66, 0.195, 0.27],
  garnacho: [0.44, 0.175, 0.27],
  dembele: [0.57, 0.205, 0.3],
  neymar: [0.53, 0.35, 0.43],
  maradona: [0.39, 0.185, 0.33],
  burn: [0.48, 0.215, 0.26],
  giroud: [0.5, 0.42, 0.53],
  lewandowski: [0.49, 0.33, 0.4],
  ibra: [0.51, 0.32, 0.455],
  pele: [0.37, 0.3, 0.385],
  raya: [0.49, 0.27, 0.36],
  ederson: [0.505, 0.29, 0.4],
}
const DEFAULT_FOCUS: [number, number, number] = [0.5, 0.25, 0.35]

// cropRect returns the source rectangle (in image pixels) that puts the face
// in a frame of the given aspect (width / height). `tight` is how many face
// widths the frame spans: 2 is a close portrait, 3 shows the shoulders.
export function cropRect(id: string, w: number, h: number, aspect: number, tight: number) {
  const [fx, fy, fw] = FOCUS[id] ?? DEFAULT_FOCUS
  let cw = Math.min(w, fw * tight * w)
  let ch = cw / aspect
  if (ch > h) {
    ch = h
    cw = ch * aspect
  }
  const x = Math.max(0, Math.min(w - cw, fx * w - cw / 2))
  // Faces sit slightly above the middle of the frame, leaving room for the name below.
  const y = Math.max(0, Math.min(h - ch, fy * h - ch * 0.42))
  return { x, y, w: cw, h: ch }
}

function isWikimedia(src: string) {
  try {
    const u = new URL(src)
    return u.protocol === 'https:' && u.hostname.endsWith('.wikimedia.org')
  } catch {
    return false
  }
}

interface WikiResponse {
  query?: {
    normalized?: { from: string; to: string }[]
    redirects?: { from: string; to: string }[]
    pages?: Record<string, { title: string; thumbnail?: { source: string } }>
  }
}

let inflight: Promise<Photos> | null = null

// One shared request per page load, with a single retry if Wikipedia is busy.
export function loadPhotos(players: PlayerDef[]): Promise<Photos> {
  inflight ??= (async () => {
    const want = players.filter((p) => p.wiki).length
    let out = await fetchPhotos(players)
    if (Object.keys(out).length < want) {
      // Some lookups failed (Wikipedia busy): one more try for the missing ones.
      await new Promise((r) => setTimeout(r, 2500))
      out = await fetchPhotos(players)
    }
    return out
  })()
  return inflight
}

async function fetchPhotos(players: PlayerDef[]): Promise<Photos> {
  // The cache maps player id to image URL ('' = looked up, no free image).
  // Only players missing from it are requested, so adding players to the pool
  // never leaves them without a portrait.
  let out: Photos = {}
  try {
    out = JSON.parse(sessionStorage.getItem(KEY) ?? '{}') as Photos
  } catch {
    /* storage unavailable: just fetch */
  }
  const withWiki = players.filter((p) => p.wiki && !(p.id in out))
  if (withWiki.length === 0) return out
  // Wikipedia accepts at most 50 titles per request, so ask in chunks.
  for (let i = 0; i < withWiki.length; i += 40) {
    const chunk = withWiki.slice(i, i + 40)
    const params = new URLSearchParams({
      action: 'query',
      format: 'json',
      origin: '*',
      redirects: '1',
      prop: 'pageimages',
      piprop: 'thumbnail',
      pithumbsize: '500',
      pilicense: 'free',
      titles: chunk.map((p) => p.wiki).join('|'),
    })
    try {
      const res = await fetch(`${ENDPOINT}?${params}`, { credentials: 'omit', referrerPolicy: 'no-referrer' })
      if (!res.ok) continue
      const data = (await res.json()) as WikiResponse
      const follow = new Map<string, string>()
      for (const n of data.query?.normalized ?? []) follow.set(n.from, n.to)
      for (const r of data.query?.redirects ?? []) follow.set(r.from, r.to)
      const byTitle = new Map<string, string>()
      for (const page of Object.values(data.query?.pages ?? {})) {
        const src = page.thumbnail?.source
        // Only accept images served by Wikimedia itself.
        if (src && isWikimedia(src)) byTitle.set(page.title, src)
      }
      for (const p of chunk) {
        let t = p.wiki!
        for (let k = 0; k < 3 && follow.has(t); k++) t = follow.get(t)!
        out[p.id] = byTitle.get(t) ?? ''
      }
    } catch {
      /* offline or blocked: initials are shown instead */
    }
  }
  try {
    sessionStorage.setItem(KEY, JSON.stringify(out))
  } catch {
    /* ignore */
  }
  return out
}

export function initials(name: string) {
  return name
    .split(' ')
    .map((w) => w[0])
    .join('')
    .slice(0, 2)
    .toUpperCase()
}

export function shortName(name: string) {
  const parts = name.split(' ')
  return parts.length > 1 ? parts.slice(1).join(' ') : name
}
