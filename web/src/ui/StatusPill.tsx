import type { CSSProperties } from 'react'
import classes from './ui.module.css'

const TONE: Record<string, string> = {
  green: 'var(--vh-moss)',
  moss: 'var(--vh-moss)',
  red: 'var(--vh-blood)',
  blood: 'var(--vh-blood)',
  blue: 'var(--vh-frost)',
  frost: 'var(--vh-frost)',
  yellow: 'var(--vh-straw)',
  straw: 'var(--vh-straw)',
  orange: 'var(--vh-ember)',
  ember: 'var(--vh-ember)',
  violet: 'var(--vh-spirit)',
  spirit: 'var(--vh-spirit)',
  gray: 'var(--vh-text-soft)',
}

function toneColor(color: string) {
  return TONE[color] ?? `var(--mantine-color-${color}-5)`
}

/** A small glowing dot; `color` is a Mantine colour name (green, red, gray…). */
export function StatusDot({ color, pulse = false }: { color: string; pulse?: boolean }) {
  return (
    <span
      className={classes.statusDot}
      data-pulse={pulse ? '' : undefined}
      style={{ '--dot-color': toneColor(color) } as CSSProperties}
      aria-hidden
    />
  )
}

/**
 * Status pill = dot + label, tinted from the same colour. Drop-in for the
 * `Badge variant="light"` state badges; the label text stays as before so
 * tests and screen readers see the same words.
 */
export function StatusPill({ color, children, pulse = false }: { color: string; children: React.ReactNode; pulse?: boolean }) {
  return (
    <span className={classes.statusPill} style={{ '--dot-color': toneColor(color) } as CSSProperties}>
      <StatusDot color={color} pulse={pulse} />
      {children}
    </span>
  )
}
