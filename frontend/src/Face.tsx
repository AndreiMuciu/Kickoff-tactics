import { useState } from 'react'
import type { PlayerDef } from './api'
import { cropRect, initials } from './photos'

// Face fills its parent with the player's portrait, cropped onto the face.
// `aspect` is the parent's width / height; `tight` is how many face widths fit across.
export function Face({ def, photo, aspect, tight }: { def: PlayerDef; photo?: string; aspect: number; tight: number }) {
  const [size, setSize] = useState<{ w: number; h: number } | null>(null)
  const [broken, setBroken] = useState(false)
  if (!photo || broken) return <span className="face face-empty">{initials(def.name)}</span>
  let style: React.CSSProperties = { opacity: 0 }
  if (size) {
    const c = cropRect(def.id, size.w, size.h, aspect, tight)
    style = { width: `${(size.w / c.w) * 100}%`, left: `${(-c.x / c.w) * 100}%`, top: `${(-c.y / c.h) * 100}%` }
  }
  return (
    <span className="face">
      <img
        src={photo}
        alt=""
        referrerPolicy="no-referrer"
        draggable={false}
        style={style}
        onLoad={(e) => setSize({ w: e.currentTarget.naturalWidth, h: e.currentTarget.naturalHeight })}
        onError={() => setBroken(true)}
      />
    </span>
  )
}
