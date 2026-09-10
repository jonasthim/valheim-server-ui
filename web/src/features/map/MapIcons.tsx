// Map marker glyphs drawn in the game's style: an ivory shape with a dark
// outline and a soft shadow, as the in-game map draws its icons. Plain SVG
// (no icon library) so every marker is one small inline element.
import type { JSX } from 'react'
import classes from './map.module.css'

export type MapIconName =
  | 'player'
  | 'portal'
  | 'ship'
  | 'cart'
  | 'tombstone'
  | 'bed'
  | 'temple'
  | 'boss'
  | 'trader'
  | 'fire'
  | 'house'
  | 'mine'
  | 'cave'
  | 'death'
  | 'hildir'
  | 'other'

const IVORY = '#efe6cc'
const SHADE = '#c9bb95'
const INK = '#2a1f14'

/** Each glyph is drawn in a 24×24 box. */
const GLYPHS: Record<MapIconName, JSX.Element> = {
  player: (
    <>
      <circle cx="12" cy="12" r="9.5" fill={IVORY} />
      <path d="M6.5 10.5 C7 6.5 10 5 12 5 C14 5 17 6.5 17.5 10.5 C15.5 9 13.5 8.6 12 8.6 C10.5 8.6 8.5 9 6.5 10.5 Z" fill={SHADE} />
      <path d="M5.5 9 C4.5 6.5 5 4.5 6.5 3.5 C6.8 5.5 7.5 6.5 8.5 7.4 Z M18.5 9 C19.5 6.5 19 4.5 17.5 3.5 C17.2 5.5 16.5 6.5 15.5 7.4 Z" fill={INK} />
      <circle cx="9.6" cy="12.6" r="1.1" fill={INK} />
      <circle cx="14.4" cy="12.6" r="1.1" fill={INK} />
      <path d="M9.5 16 C10.5 17.2 13.5 17.2 14.5 16" fill="none" />
    </>
  ),
  portal: (
    <>
      <path d="M5 21 V10.5 A7 7 0 0 1 19 10.5 V21 Z" fill={IVORY} />
      <path d="M8.2 21 V11.2 A3.8 3.8 0 0 1 15.8 11.2 V21 Z" fill={INK} />
      <path d="M5 21 H19" />
    </>
  ),
  ship: (
    <>
      <path d="M3 14.5 H21 L18 19.5 H6 Z" fill={IVORY} />
      <path d="M12 4 V14.5" />
      <path d="M12.6 5 L18.5 12 H12.6 Z" fill={SHADE} />
      <path d="M3 14.5 C4 13 5 12.8 6 12.8" />
    </>
  ),
  cart: (
    <>
      <path d="M4 8.5 H15.5 V16 H4 Z" fill={IVORY} />
      <path d="M15.5 11.5 L21 9" />
      <circle cx="7.5" cy="18.3" r="2.2" fill={SHADE} />
      <circle cx="13" cy="18.3" r="2.2" fill={SHADE} />
    </>
  ),
  tombstone: (
    <>
      <path d="M7 21 V10 A5 5 0 0 1 17 10 V21 Z" fill={IVORY} />
      <path d="M12 9 V16 M9.5 11.5 H14.5" />
      <path d="M5 21 H19" />
    </>
  ),
  bed: (
    <>
      <path d="M3 17 V8 H6 V13 H19 A2 2 0 0 1 21 15 V17 Z" fill={IVORY} />
      <path d="M7 13 V11 A1.5 1.5 0 0 1 8.5 9.5 H11.5 A1.5 1.5 0 0 1 13 11 V13" fill={SHADE} />
      <path d="M3 17 V19.5 M21 17 V19.5" />
    </>
  ),
  temple: (
    <>
      <path d="M4.5 20 V11 A1.8 1.8 0 0 1 8.1 11 V20 Z" fill={IVORY} />
      <path d="M10.2 20 V7 A1.8 1.8 0 0 1 13.8 7 V20 Z" fill={IVORY} />
      <path d="M15.9 20 V11 A1.8 1.8 0 0 1 19.5 11 V20 Z" fill={IVORY} />
      <path d="M3 20 H21" />
    </>
  ),
  boss: (
    <>
      <path d="M3.5 5.5 C4.5 9.5 6.5 11 8.5 12 C7.5 9.5 6.5 7.5 6 4.5 Z M20.5 5.5 C19.5 9.5 17.5 11 15.5 12 C16.5 9.5 17.5 7.5 18 4.5 Z" fill={IVORY} />
      <path d="M12 5 C15.9 5 18.5 7.6 18.5 11 C18.5 13.7 16.8 15.2 16.3 17 L15.5 20 H8.5 L7.7 17 C7.2 15.2 5.5 13.7 5.5 11 C5.5 7.6 8.1 5 12 5 Z" fill={IVORY} />
      <circle cx="9.4" cy="11.8" r="1.7" fill={INK} />
      <circle cx="14.6" cy="11.8" r="1.7" fill={INK} />
      <path d="M12 13.5 L11 15.5 H13 Z" fill={INK} />
      <path d="M10 17.5 V20 M12 17.5 V20 M14 17.5 V20" />
    </>
  ),
  trader: (
    <>
      <path d="M9 6.5 H15 L17.5 10 C19.5 13 19.5 18 16.5 20 H7.5 C4.5 18 4.5 13 6.5 10 Z" fill={IVORY} />
      <path d="M9.5 6.5 L8.5 4 H15.5 L14.5 6.5" fill={SHADE} />
      <path d="M10 14.5 H14 M12 12.5 V16.5" />
    </>
  ),
  fire: (
    <>
      <path d="M12 3 C13 7 17 8 17 13 A5 5 0 0 1 7 13 C7 10 9 9 9 6 C10 8 11 9 12 9 C12 7 11 5 12 3 Z" fill={IVORY} />
      <path d="M12 11 C13 13 14.5 13.5 14.5 15.5 A2.5 2.5 0 0 1 9.5 15.5 C9.5 14 10.5 13.5 11 12 Z" fill={SHADE} />
    </>
  ),
  house: (
    <>
      <path d="M4 12 L12 4.5 L20 12 V20 H4 Z" fill={IVORY} />
      <path d="M10 20 V14 H14 V20 Z" fill={INK} />
      <path d="M2.5 12.8 L12 4 L21.5 12.8" fill="none" />
    </>
  ),
  mine: (
    <>
      <path d="M4 9 C9 4.5 15 4.5 20 9 C15 7.8 9 7.8 4 9 Z" fill={IVORY} />
      <path d="M12 7.5 L8.5 20.5" strokeWidth="2.6" />
    </>
  ),
  cave: (
    <>
      <path d="M4 20 V12 A8 8 0 0 1 20 12 V20 Z" fill={IVORY} />
      <path d="M8 20 V13.5 A4 4 0 0 1 16 13.5 V20 Z" fill={INK} />
    </>
  ),
  death: (
    <>
      <path d="M12 4.5 C16 4.5 18.5 7 18.5 10.5 C18.5 13 17 14.5 16.5 16.5 L15.8 19.5 H8.2 L7.5 16.5 C7 14.5 5.5 13 5.5 10.5 C5.5 7 8 4.5 12 4.5 Z" fill={IVORY} />
      <circle cx="9.5" cy="11.2" r="1.7" fill={INK} />
      <circle cx="14.5" cy="11.2" r="1.7" fill={INK} />
      <path d="M12 13 L11 15 H13 Z" fill={INK} />
      <path d="M10 17 V19.5 M12 17 V19.5 M14 17 V19.5" />
    </>
  ),
  hildir: (
    <>
      <path d="M12 4 L21 19.5 H3 Z" fill={IVORY} />
      <path d="M12 10 L15 19.5 H9 Z" fill={INK} />
      <path d="M12 4 V2" />
    </>
  ),
  other: (
    <>
      <circle cx="12" cy="12" r="7.5" fill={IVORY} />
      <circle cx="12" cy="12" r="2.6" fill={INK} />
    </>
  ),
}

export function MapIcon({ name, size, checked }: { name: MapIconName; size: number; checked?: boolean }) {
  return (
    <svg
      className={classes.icon}
      width={size}
      height={size}
      viewBox="0 0 24 24"
      aria-hidden
      stroke={INK}
      strokeWidth="1.4"
      strokeLinejoin="round"
      strokeLinecap="round"
    >
      {GLYPHS[name] ?? GLYPHS.other}
      {checked && <path d="M5 5 L19 19 M19 5 L5 19" stroke="#c8352b" strokeWidth="2.6" opacity="0.85" />}
    </svg>
  )
}

