import type { CSSProperties, ReactNode } from 'react'
import { Skeleton } from '@mantine/core'
import { Sparkline } from './Sparkline'
import classes from './ui.module.css'

export interface StatStripItem {
  /** Stable key; defaults to `label`. */
  key?: string
  /** Plain string: it is also the sparkline's accessible name. */
  label: string
  value: ReactNode
  hint?: ReactNode
  /** Small decorative icon right of the label; ignored when `action` is set. */
  icon?: ReactNode
  /** A small control right of the label (e.g. an ActionIcon size="sm"). */
  action?: ReactNode
  /** Left accent bar by meaning. */
  tone?: 'default' | 'accent' | 'danger' | 'success'
  /** Raw CSS colour for the accent bar; overrides `tone`. */
  accent?: string
  spark?: number[]
  sparkFormat?: (v: number) => string
}

const TONE_COLOR: Record<NonNullable<StatStripItem['tone']>, string | undefined> = {
  default: undefined,
  accent: 'var(--vh-ember)',
  danger: 'var(--vh-blood)',
  success: 'var(--vh-moss)',
}

/**
 * One row of hairline-separated KPI cells (label 12px soft, value 20px/600
 * tabular, hint 12px faint, optional sparkline). Cells auto-fit at
 * `minCellWidth` unless `cols` fixes the count. Rendered as a description
 * list so each label/value pair is announced together.
 */
export function StatStrip({
  items, cols, minCellWidth = 140, loading = false, className,
}: {
  items: StatStripItem[]; cols?: number; minCellWidth?: number; loading?: boolean; className?: string
}) {
  const style = { '--strip-min': `${minCellWidth}px`, ...(cols ? { '--strip-cols': cols } : {}) } as CSSProperties
  return (
    <dl className={[classes.statStrip, className].filter(Boolean).join(' ')} data-cols={cols ? '' : undefined} style={style}>
      {items.map((it) => {
        const accent = it.accent ?? (it.tone ? TONE_COLOR[it.tone] : undefined)
        return (
          <div
            key={it.key ?? it.label}
            className={classes.statCell}
            data-accent={accent ? '' : undefined}
            style={accent ? ({ '--cell-accent': accent } as CSSProperties) : undefined}
          >
            <dt className={classes.statCellLabel}>
              <span>{it.label}</span>
              {it.action ?? (it.icon && <span className={classes.statCellIcon} aria-hidden>{it.icon}</span>)}
            </dt>
            {loading ? (
              <dd className={classes.statCellValue}><Skeleton height={20} width="60%" /></dd>
            ) : (
              <>
                <dd className={classes.statCellValue} data-long={typeof it.value === 'string' && it.value.length > 12 ? '' : undefined}>
                  {it.value}
                </dd>
                {it.hint && <dd className={classes.statCellHint}>{it.hint}</dd>}
                {it.spark && it.spark.length >= 2 && (
                  <dd className={classes.statCellSpark}>
                    <Sparkline values={it.spark} format={it.sparkFormat} width="100%" height={20} label={it.label} />
                  </dd>
                )}
              </>
            )}
          </div>
        )
      })}
    </dl>
  )
}
